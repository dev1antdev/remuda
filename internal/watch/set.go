package watch

// What one stream holds: the global kinds, every pane's agent status, the output watches asked for; worked out for each
// snapshot, and a new stream opened when it moved.

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// Output is a watch herdr keeps on a pane's output: a match, the rows it reads, and the key its matches come back with.
type Output struct {
	Key    string
	Pane   string
	Match  herdr.OutputMatch
	Lines  int
	Person bool // one of the person's own rules: left out of the stream when herdr refuses them
}

// urgentKinds are the global kinds that ask for a snapshot at once; lazyKind, every change of a pane's title or
// folder (an agent's spinner moves its title several times a second), one within the lazy gap.
var urgentKinds = []string{
	"workspace.created", "workspace.closed", "workspace.renamed", "workspace.moved", "workspace.reordered", "workspace.focused",
	"worktree.created", "worktree.opened", "worktree.removed",
	"tab.created", "tab.closed", "tab.renamed", "tab.moved", "tab.focused",
	"pane.created", "pane.closed", "pane.focused", "pane.moved", "pane.exited", "pane.agent_detected", "layout.updated",
}

const lazyKind = "pane.updated"

// streamSet is what one stream holds.
type streamSet struct {
	subs    []herdr.Subscription
	all     []Output            // the watches asked for, as asked
	outputs map[string][]Output // by pane, the ones held
	key     string              // the whole set as one string, to tell a move
	person  string              // the person's own watches as one string, "" for none
	holds   bool                // the person's watches are in it: herdr has not refused them as they stand
}

// buildSet is the subscriptions for a snapshot: the global kinds, each pane's agent status, the output watches but
// the person's when they are the ones herdr refused.
func buildSet(snap *herdr.Snapshot, outputs []Output, refused string) *streamSet {
	set := &streamSet{all: outputs, outputs: map[string][]Output{}}
	for _, kind := range urgentKinds {
		set.subs = append(set.subs, herdr.Subscription{Type: kind})
	}
	set.subs = append(set.subs, herdr.Subscription{Type: lazyKind})
	panes := make([]string, 0, len(snap.Panes))
	known := map[string]bool{}
	for i := range snap.Panes {
		panes = append(panes, snap.Panes[i].PaneID)
		known[snap.Panes[i].PaneID] = true
	}
	sort.Strings(panes)
	for _, pane := range panes {
		set.subs = append(set.subs, herdr.Subscription{Type: herdr.EventAgentStatus, PaneID: pane})
	}
	var person []Output
	for _, o := range outputs {
		if o.Person {
			person = append(person, o)
		}
	}
	set.person = fingerprint(person)
	set.holds = set.person != "" && set.person != refused
	for _, o := range outputs {
		if !known[o.Pane] || o.Person && set.person == refused {
			continue
		}
		match := o.Match
		set.subs = append(set.subs, herdr.Subscription{Type: herdr.EventOutputMatched, PaneID: o.Pane, Source: "recent", Lines: o.Lines, Match: &match})
		set.outputs[o.Pane] = append(set.outputs[o.Pane], o)
	}
	set.key = fingerprint(set.subs)
	return set
}

// fingerprint is a value as one string; "" for nothing.
func fingerprint[T any](values []T) string {
	if len(values) == 0 {
		return ""
	}
	b, _ := json.Marshal(values)
	return string(b)
}

// noteWanted works out the subscriptions for the snapshot just applied and tells the stream when they moved. A move
// the snapshot asked for — a pane that appeared or went, the watches that follow the panes — is kept pending for the
// rest of its burst (`eventDebounce`), and only then becomes the set the stream subscribes to: a pane that appears and
// goes within it swaps no stream at all, and a stream opening meanwhile holds the set that stands. A later snapshot
// replaces the set still settling without moving that rest, so a status event cannot push a watch out. A move the
// person asked for (`now`: a rule edited, a refusal taken again) is promoted and told at once, and so is the first
// set.
func (w *Watcher) noteWanted(machineID string, snap *herdr.Snapshot, now bool) {
	var outputs []Output
	if w.Outputs != nil {
		outputs = w.Outputs(machineID, snap)
	}
	w.mu.Lock()
	st := w.stateFor(machineID)
	set := buildSet(snap, outputs, st.refused)
	switch {
	case st.wanted == nil || now:
		if st.wanted != nil && st.wanted.key == set.key && st.pending == nil {
			w.mu.Unlock()
			return
		}
		st.promote(set)
		w.mu.Unlock()
		signal(st.changed)
	case st.wanted.key == set.key:
		if st.pending != nil { // back where the stream stands: the settling move is dropped
			st.dropSettling()
		}
		w.mu.Unlock()
	default:
		st.pending = set
		if st.settling != nil {
			w.mu.Unlock() // a set is already settling: this one takes its place, the rest it was booked for stands
			return
		}
		st.gen++
		gen := st.gen
		st.settling = time.AfterFunc(eventDebounce, func() { w.settle(st, gen) })
		w.mu.Unlock()
	}
}

// settle promotes the set the churn's rest left, so the stream swaps to it — once: a move booked a later rest is not
// the one that fired.
func (w *Watcher) settle(st *machineState, gen uint64) {
	w.mu.Lock()
	if st.gen != gen || st.pending == nil {
		w.mu.Unlock()
		return
	}
	st.promote(st.pending)
	w.mu.Unlock()
	signal(st.changed)
}

// promote makes set the one the stream subscribes to, drops any move still settling, and answers whether it is news.
// The caller holds the lock.
func (st *machineState) promote(set *streamSet) {
	st.wanted, st.pending = set, nil
	st.gen++
	if st.settling != nil {
		st.settling.Stop()
		st.settling = nil
	}
}

// dropSettling drops the move still settling, leaving the set the stream subscribes to as it is. The caller holds
// the lock.
func (st *machineState) dropSettling() {
	st.pending = nil
	st.gen++
	if st.settling != nil {
		st.settling.Stop()
		st.settling = nil
	}
}

// Resubscribe works the subscriptions out again on every machine: the watches asked for moved.
func (w *Watcher) Resubscribe() {
	w.mu.Lock()
	snaps := map[string]*herdr.Snapshot{}
	for id, st := range w.states {
		if st.snap != nil {
			snaps[id] = st.snap
		}
	}
	w.mu.Unlock()
	for id, snap := range snaps {
		w.noteWanted(id, snap, true)
	}
}
