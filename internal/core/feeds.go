package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// feed is one core subscription telling one output watch, shared by every stream asking for that same watch. The
// watcher opens a stream beside the one before it whenever a pane comes or goes or a rule changes; joining the feeds
// that stand, the new stream is not told every matching row still on a screen as if the core had just seen it, and a
// watch that did not change keeps telling its rows as they appear. Only a new watch gets a feed of its own.
type feed struct {
	key   OutputWatch // the watch as subscribed: the adapter's feeds are kept by it
	pane  string
	match herdr.OutputMatch
	sub   *Subscription

	mu      sync.Mutex
	streams map[*eventStream]struct{}
	ended   bool
}

// opensAtOnce is how many feeds a stream subscribes side by side.
const opensAtOnce = 8

// joinWithin is how long a stream's subscribing of its new watches may go without a single answer, as one request is
// bounded: a core that answers keeps the join going however many rounds of opensAtOnce its watches take. A test
// shortens it.
var joinWithin = callWithin

// add joins the stream to a feed that still tells; false once the feed ended.
func (f *feed) add(s *eventStream) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ended {
		return false
	}
	f.streams[s] = struct{}{}
	return true
}

// remove takes the stream out; true when no stream is left.
func (f *feed) remove(s *eventStream) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.streams, s)
	return len(f.streams) == 0
}

// end ends every stream the feed told, with why.
func (f *feed) end(err error) {
	f.mu.Lock()
	f.ended = true
	streams := f.streams
	f.streams = map[*eventStream]struct{}{}
	f.mu.Unlock()
	for s := range streams {
		s.end(err)
	}
}

func (f *feed) push(ev herdr.Event) {
	f.mu.Lock()
	streams := make([]*eventStream, 0, len(f.streams))
	for s := range f.streams {
		streams = append(streams, s)
	}
	f.mu.Unlock()
	for _, s := range streams {
		s.push(ev)
	}
}

// join hands the stream its watches: a feed already telling one is joined, the others subscribed side by side. On a
// refusal nothing new stands and the stream leaves the feeds it joined; a regex the core refused is the refusal told
// first, so the watcher drops the person's watches rather than asking again with them.
func (a *Adapter) join(s *eventStream, watches []OutputWatch) error {
	a.feedMu.Lock()
	defer a.feedMu.Unlock()
	var missing []OutputWatch
	for _, w := range watches {
		if f := a.feeds[w]; f != nil && f.add(s) {
			s.feeds = append(s.feeds, f)
		} else {
			missing = append(missing, w)
		}
	}
	ctx, cancel := context.WithCancel(a.life) // cut short by Close, a refusal, or joinWithin with no answer
	defer cancel()
	subs := make([]*Subscription, len(missing))
	errs := make([]error, len(missing))
	answered := make(chan int, len(missing))
	slots := make(chan struct{}, opensAtOnce)
	for i, w := range missing {
		go func() {
			defer func() { answered <- i }()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				errs[i] = ctx.Err()
				return
			}
			defer func() { <-slots }()
			subs[i], errs[i] = a.client.Subscribe(ctx, SubscribeParams{Labels: map[string]string{outputOnly: "1"}, Output: []OutputWatch{w}})
		}()
	}
	var refused error
	idle := time.NewTimer(joinWithin)
	defer idle.Stop()
	for range missing {
		var i int
		select {
		case i = <-answered:
			idle.Reset(joinWithin)
		case <-idle.C:
			refused = firstRefusal(refused, fmt.Errorf("remuda-core went %s without answering an output watch", joinWithin))
			cancel()
			i = <-answered
		}
		if errs[i] != nil {
			refused = firstRefusal(refused, errs[i])
			cancel() // the join is lost: the rest end at once
		}
	}
	if refused != nil {
		for _, sub := range subs {
			if sub != nil {
				sub.Close()
			}
		}
		a.leaveLocked(s)
		return a.refusal(refused, "pane_not_found")
	}
	for i, w := range missing {
		f := &feed{key: w, pane: w.Terminal, match: herdr.OutputMatch{Type: w.Match.Type, Value: w.Match.Value}, sub: subs[i], streams: map[*eventStream]struct{}{s: {}}}
		a.feeds[f.key] = f
		s.feeds = append(s.feeds, f)
		go a.tell(f)
	}
	return nil
}

// firstRefusal is the refusal a join tells: a regex the core refused before any other, our own cut short last.
func firstRefusal(told, err error) error {
	rank := func(err error) int {
		switch {
		case err == nil:
			return -1
		case IsKind(err, "invalid_regex"):
			return 2
		case errors.Is(err, context.Canceled):
			return 0
		}
		return 1
	}
	if rank(err) > rank(told) {
		return err
	}
	return told
}

// leave takes the stream out of its feeds, closing a feed no stream is left on.
func (a *Adapter) leave(s *eventStream) {
	a.feedMu.Lock()
	defer a.feedMu.Unlock()
	a.leaveLocked(s)
}

func (a *Adapter) leaveLocked(s *eventStream) {
	for _, f := range s.feeds {
		if f.remove(s) {
			if a.feeds[f.key] == f {
				delete(a.feeds, f.key)
			}
			f.sub.Close()
		}
	}
	s.feeds = nil
}

// tell hands the feed's streams every row its watches matched, as herdr's pane.output_matched, until the core's
// subscription ends: a `missed` there ends them with events_lost, so the watcher subscribes again.
func (a *Adapter) tell(f *feed) {
	toldLost := false
	for {
		notice, err := f.sub.Next(context.Background())
		if err == nil && notice.Kind == NoticeMissed {
			err = &herdr.Error{Code: herdr.ErrEventsLost, Message: "remuda-core dropped output rows: subscribe again"}
		}
		if err != nil {
			a.feedMu.Lock()
			if a.feeds[f.key] == f {
				delete(a.feeds, f.key)
			}
			a.feedMu.Unlock()
			f.end(err)
			f.sub.Close()
			return
		}
		if notice.Kind != NoticeEvent || notice.Event.Type != "output" {
			continue
		}
		var row struct {
			Line  string `json:"line"`
			Pos   uint64 `json:"pos"`
			Lost  int    `json:"lost"`
			Fresh *bool  `json:"fresh"`
		}
		if json.Unmarshal(notice.Event.Data, &row) != nil {
			continue
		}
		if row.Lost > 0 && !toldLost {
			log.Printf("remuda-core dropped %d rows of an output watch on %s before this one", row.Lost, f.pane)
			toldLost = true
		}
		f.push(herdr.Event{Kind: herdr.EventOutputMatched, PaneID: f.pane, MatchedLine: row.Line, Pos: row.Pos, Text: row.Line, Standing: row.Fresh, Watch: &f.match})
	}
}
