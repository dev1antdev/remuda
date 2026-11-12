package watch

// herdr's event stream, one per machine: a connection holding the global kinds (a pane, tab or workspace made, closed,
// moved, focused), every pane's agent status and the output watches asked for, opened again whenever that set moves —
// herdr fixes a connection's subscriptions for its life. Any event asks the loop for a snapshot (pace.go); an output
// match goes on the bus as it is. A stream that ends, or herdr saying it lost events, is a snapshot now and a new stream.

import (
	"errors"
	"log"
	"net"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
)

// Retries: a stream that could not open or broke, at most this far apart; a herdr that knows no stream, this often.
var (
	streamRetry      = time.Second
	streamBackoff    = 30 * time.Second
	unsupportedRetry = 5 * time.Minute
	freshFor         = 2 * time.Second // a match this soon after a stream opened may be one that already stood
)

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
// swapped for the next, never for the `nc -U` bridge, whose every connection is a session of the ssh master. Every
// stream that opens is followed by a snapshot, so what moved before it stood is not missed; a stream that ended is
// closed here too, whatever ended it, and the next carries its watches: a row that stood through the gap is the same
// row, one that came in it is news.
func (w *Watcher) stream(mc *machines.Machine, st *machineState, stop <-chan struct{}) {
	var cur, last *liveStream // the stream standing; the one before it, while none stands
	defer func() {
		if cur != nil {
			cur.close()
		}
		st.streaming.Store(false)
	}()
	backoff := streamRetry
	pause := func(d time.Duration) bool { // a wait the set moving cuts short: the next try takes the new one
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-stop:
			return false
		case <-st.changed:
			return true
		case <-timer.C:
			return true
		}
	}
	var retry <-chan time.Time // a swap that could not open, tried again then
	for {
		select {
		case <-stop:
			return
		default:
		}
		if cur == nil {
			next, wait := w.connect(mc, st, last)
			if next == nil {
				if !pause(max(wait, backoff)) { // a bridged remote's every try is an ssh session: patience while it fails
					return
				}
				backoff = min(backoff*2, streamBackoff)
				continue
			}
			cur, last, retry = next, nil, nil
			st.streaming.Store(true)
			signal(st.kick)
			continue
		}
		select {
		case <-stop:
			return
		case err := <-cur.ended:
			cur.close()
			if time.Since(cur.started) > time.Minute {
				backoff = streamRetry // it stood a while: no flapping to be patient with
			}
			cur, last, retry = nil, cur, nil
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
		case <-retry:
			retry = nil
			signal(st.changed)
		case <-st.changed:
			next, wait := w.swap(mc, st, cur)
			if wait > 0 {
				retry = time.After(wait)
			}
			if next == cur {
				continue
			}
			if next == nil {
				last = cur
				st.streaming.Store(false) // the bridge's new stream did not open: the loop above tries at its pace
			}
			cur, retry = next, nil
			signal(st.kick)
		}
	}
}

// swap opens a stream on the wanted set in place of cur when the set moved: the new one first on a socket, cur closed
// first through the bridge. It answers the stream that stands now — cur when nothing moved or a socket's new stream
// could not open (then how long until it is tried again), else the new one, or nil when the bridge's did not open.
func (w *Watcher) swap(mc *machines.Machine, st *machineState, cur *liveStream) (*liveStream, time.Duration) {
	w.mu.Lock()
	wanted := st.wanted
	w.mu.Unlock()
	if wanted == nil || wanted.key == cur.set.key {
		return cur, 0
	}
	client, _, err := mc.Ready()
	if err != nil || client == nil {
		return cur, 2 * streamRetry
	}
	if client.Bridged() {
		cur.close()
		next, _ := w.connect(mc, st, cur)
		return next, 0
	}
	next, wait := w.connect(mc, st, cur)
	if next == nil {
		return cur, max(wait, streamRetry)
	}
	cur.close()
	return next, 0
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
	if err != nil && set.holds && refusesWatches(err) {
		if w.sayOnce(st, "refused", err) {
			log.Printf("%s: herdr refused the triggers' watches: %v", mc.ID(), err)
		}
		w.mu.Lock()
		st.refused = set.person
		set = buildSet(st.snap, set.all, st.refused)
		st.wanted = set
		w.mu.Unlock()
		if w.Refused != nil {
			w.Refused(mc.ID(), err)
		}
		stream, err = client.Subscribe(set.subs)
	} else if err == nil && set.holds && w.Refused != nil {
		w.Refused(mc.ID(), nil) // taken again: herdr reads the person's watches as they stand now
	}
	if err != nil {
		var he *herdr.Error
		switch {
		case herdr.IsCode(err, "pane_not_found"):
			signal(st.kick) // the pane set moved under the subscription: the next snapshot tells the new one
			return nil, streamRetry
		case errors.As(err, &he):
			if w.sayOnce(st, "unsupported", err) {
				log.Printf("%s: herdr keeps no event stream for remuda (%v); polling at the usual pace, asking again every %s", mc.ID(), err, unsupportedRetry)
			}
			w.mu.Lock()
			st.noStreamUntil = time.Now().Add(unsupportedRetry)
			w.mu.Unlock()
			return nil, unsupportedRetry
		}
		return nil, 2 * streamRetry
	}
	w.mu.Lock()
	st.said = "" // a refusal after this one stood is news again
	w.mu.Unlock()
	ls := &liveStream{stream: stream, set: set, started: time.Now(), carried: map[string]bool{}, ended: make(chan error, 1)}
	if prev != nil {
		for pane, outputs := range prev.set.outputs {
			for _, o := range outputs {
				ls.carried[carryKey(pane, o)] = true
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

// sayOnce says whether a refusal of the stream is news for the log: not the same words as the try before.
func (w *Watcher) sayOnce(st *machineState, what string, err error) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	words := what + ": " + err.Error()
	if st.said == words {
		return false
	}
	st.said = words
	return true
}

// refusesWatches is herdr refusing a set for a regex of its output watches; any other refusal is of the stream itself
// (one too old for it answers invalid_request) or of a pane that went.
func refusesWatches(err error) bool { return herdr.IsCode(err, "invalid_regex") }

// carryKey names a watch on a pane by what it looks for too: a rule edited to look for something else is a new watch.
func carryKey(pane string, o Output) string {
	return pane + "|" + o.Key + "|" + o.Match.Type + "|" + o.Match.Value
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
				Fresh: now.Sub(ls.started) < freshFor, Carried: ls.carried[carryKey(event.PaneID, o)], At: now,
			})
		}
	case lazyKind, "pane_updated":
		signal(st.nudge)
	default:
		signal(st.kick)
	}
}

// matches says whether a row matches a watch, as herdr would: a substring as it is, a regex in herdr's dialect.
func (w *Watcher) matches(m herdr.OutputMatch, line string) bool {
	if m.Type != "regex" {
		return strings.Contains(line, m.Value)
	}
	if re, ok := w.matchers.Load(m.Value); ok {
		return re.(*herdr.Regexp) != nil && re.(*herdr.Regexp).MatchString(line)
	}
	re, err := herdr.CompileRegex(m.Value)
	if err != nil {
		re = nil
	}
	w.matchers.Store(m.Value, re)
	return re != nil && re.MatchString(line)
}
