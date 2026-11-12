package watch

// What one stream holds: the global kinds, every pane's agent status, the output watches asked for; worked out for each
// snapshot, and a new stream opened when it moved.

import (
	"encoding/json"
	"sort"

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

// noteWanted works out the subscriptions for the snapshot just applied and tells the stream when they moved.
func (w *Watcher) noteWanted(machineID string, snap *herdr.Snapshot) {
	var outputs []Output
	if w.Outputs != nil {
		outputs = w.Outputs(machineID, snap)
	}
	w.mu.Lock()
	st := w.stateFor(machineID)
	set := buildSet(snap, outputs, st.refused)
	moved := st.wanted == nil || st.wanted.key != set.key
	if moved {
		st.wanted = set
	}
	w.mu.Unlock()
	if moved {
		signal(st.changed)
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
		w.noteWanted(id, snap)
	}
}
