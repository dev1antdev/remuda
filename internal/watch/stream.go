package watch

// herdr's event stream, one per machine: a connection holding the global kinds (a pane, tab or workspace made, closed,
// moved, focused), every pane's agent status and the output watches asked for, opened again whenever that set moves —
// herdr fixes a connection's subscriptions for its life. Any event asks the loop for a snapshot (pace.go); an output
// match goes on the bus as it is. A stream that ends, or herdr saying it lost events, is a snapshot now and a new stream.

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
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

// Retries: a stream that could not open or broke, at most this far apart; a herdr that knows no stream, this often.
var (
	streamRetry      = time.Second
	streamBackoff    = 30 * time.Second
	unsupportedRetry = 5 * time.Minute
	freshFor         = 2 * time.Second // a match this soon after a stream opened may be one that already stood
)

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

// liveStream is one open stream and what it holds.
type liveStream struct {
	stream  *herdr.Stream
	set     *streamSet
	started time.Time
	carried map[string]bool // "pane|key" of the output watches the stream before it held too
	ended   chan error
}

func (ls *liveStream) close() { ls.stream.Close() }

// stream keeps the machine's event stream open until stop, one at a time — two for a moment while a socket's is
// swapped for the next, never for the `nc -U` bridge, whose every connection is a session of the ssh master.
func (w *Watcher) stream(mc *machines.Machine, st *machineState, stop <-chan struct{}) {
	var cur *liveStream
	defer func() {
		if cur != nil {
			cur.close()
		}
		st.streaming.Store(false)
	}()
	backoff := streamRetry
	pause := func(d time.Duration) bool {
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-stop:
			return false
		case <-st.changed:
			signal(st.changed)
			return true
		case <-timer.C:
			return true
		}
	}
	for {
		select {
		case <-stop:
			return
		default:
		}
		if cur == nil {
			next, retry := w.connect(mc, st, nil)
			if next == nil {
				if !pause(retry) {
					return
				}
				continue
			}
			cur = next
			st.streaming.Store(true)
			signal(st.kick) // a stream opened: what moved before it is in the next snapshot
			continue
		}
		select {
		case <-stop:
			return
		case err := <-cur.ended:
			if time.Since(cur.started) > time.Minute {
				backoff = streamRetry // it stood a while: no flapping to be patient with
			}
			cur = nil
			st.streaming.Store(false)
			signal(st.kick)
			if herdr.IsCode(err, herdr.ErrEventsLost) {
				log.Printf("%s: herdr lost events of the stream; a snapshot now, a new stream", mc.ID())
				backoff = streamRetry
				continue
			}
			if !pause(backoff) {
				return
			}
			backoff = min(backoff*2, streamBackoff)
		case <-st.changed:
			w.mu.Lock()
			wanted := st.wanted
			w.mu.Unlock()
			if wanted == nil || wanted.key == cur.set.key {
				continue
			}
			client, _, err := mc.Ready()
			if err != nil {
				continue // the stream on the old connection ends with it
			}
			if client.Bridged() {
				cur.close()
				prev := cur
				cur = nil
				if next, _ := w.connect(mc, st, prev); next != nil {
					cur = next
				} else {
					st.streaming.Store(false)
					signal(st.kick)
				}
				continue
			}
			if next, _ := w.connect(mc, st, cur); next != nil {
				cur.close()
				cur = next
			}
		}
	}
}

// connect opens a stream on the wanted set, answering how long to wait before the next try when it could not.
func (w *Watcher) connect(mc *machines.Machine, st *machineState, prev *liveStream) (*liveStream, time.Duration) {
	w.mu.Lock()
	set := st.wanted
	wait := time.Until(st.noStreamUntil)
	w.mu.Unlock()
	if set == nil {
		return nil, streamRetry // no snapshot yet: the first one tells the set
	}
	if wait > 0 {
		return nil, wait
	}
	client, _, err := mc.Ready()
	if err != nil || client == nil {
		return nil, 2 * streamRetry
	}
	stream, err := client.Subscribe(set.subs)
	if err != nil && set.person != "" && refusesWatches(err) {
		log.Printf("%s: herdr refused the triggers' watches: %v", mc.ID(), err)
		w.mu.Lock()
		st.refused = set.person
		set = buildSet(st.snap, set.all, st.refused)
		st.wanted = set
		w.mu.Unlock()
		if w.Refused != nil {
			w.Refused(mc.ID(), err)
		}
		stream, err = client.Subscribe(set.subs)
	} else if err == nil && set.person != "" && w.Refused != nil {
		w.Refused(mc.ID(), nil)
	}
	if err != nil {
		var he *herdr.Error
		switch {
		case herdr.IsCode(err, "pane_not_found"):
			signal(st.kick) // the pane set moved under the subscription: the next snapshot tells the new one
			return nil, streamRetry
		case errors.As(err, &he):
			log.Printf("%s: herdr keeps no event stream for remuda (%v); polling at the usual pace", mc.ID(), err)
			w.mu.Lock()
			st.noStreamUntil = time.Now().Add(unsupportedRetry)
			w.mu.Unlock()
			return nil, unsupportedRetry
		}
		return nil, 2 * streamRetry
	}
	ls := &liveStream{stream: stream, set: set, started: time.Now(), carried: map[string]bool{}, ended: make(chan error, 1)}
	if prev != nil {
		for pane, outputs := range prev.set.outputs {
			for _, o := range outputs {
				ls.carried[pane+"|"+o.Key] = true
			}
		}
	}
	go func() {
		for {
			event, err := stream.Next()
			if err != nil {
				if !errors.Is(err, net.ErrClosed) {
					log.Printf("%s: herdr's event stream: %v", mc.ID(), err)
				}
				ls.ended <- err
				return
			}
			w.heard(mc.ID(), st, ls, event)
		}
	}()
	return ls, 0
}

// refusesWatches is herdr refusing a set for its output watches rather than for a pane that went.
func refusesWatches(err error) bool {
	var he *herdr.Error
	return errors.As(err, &he) && he.Code != "pane_not_found"
}

// heard is one event of a stream: an output match goes on the bus for every watch its row matches, anything else
// asks the loop for a snapshot.
func (w *Watcher) heard(machineID string, st *machineState, ls *liveStream, event herdr.Event) {
	switch event.Kind {
	case herdr.EventOutputMatched:
		now := time.Now()
		for _, o := range ls.set.outputs[event.PaneID] {
			if !w.matches(o.Match, event.MatchedLine) {
				continue
			}
			events.Publish(w.bus, events.OutputMatched{
				Pane: events.PaneRef{Machine: machineID, Pane: event.PaneID}, Watch: o.Key, Line: event.MatchedLine, Text: event.Text,
				Fresh: now.Sub(ls.started) < freshFor, Carried: ls.carried[event.PaneID+"|"+o.Key], At: now,
			})
		}
	case lazyKind, "pane_updated":
		signal(st.nudge)
	default:
		signal(st.kick)
	}
}

// matches says whether a row matches a watch, as herdr would: a substring as it is, a regex a line at a time.
func (w *Watcher) matches(m herdr.OutputMatch, line string) bool {
	if m.Type != "regex" {
		return strings.Contains(line, m.Value)
	}
	if re, ok := w.matchers.Load(m.Value); ok {
		return re.(*regexp.Regexp) != nil && re.(*regexp.Regexp).MatchString(line)
	}
	re, err := regexp.Compile(m.Value)
	if err != nil {
		re = nil
	}
	w.matchers.Store(m.Value, re)
	return re != nil && re.MatchString(line)
}
