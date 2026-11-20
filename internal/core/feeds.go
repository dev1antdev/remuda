package core

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"sync"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// feed is one core subscription telling one output watch, shared by every stream asking for that same watch. The
// watcher opens a stream beside the one before it whenever a pane comes or goes or a rule changes; joining the feeds
// that stand, the new stream is not told every matching row still on a screen as if the core had just seen it, and a
// watch that did not change keeps telling its rows as they appear. Only a new watch gets a feed of its own.
type feed struct {
	key  string
	pane string
	sub  *Subscription

	mu      sync.Mutex
	streams map[*eventStream]struct{}
	ended   bool
}

// opensAtOnce is how many feeds a stream subscribes side by side.
const opensAtOnce = 8

// feedKey names a watch by its pane and what it looks for.
func feedKey(w OutputWatch) string {
	return w.Terminal + "\x00" + w.Match.Type + "\x00" + w.Match.Value + "\x00" + strconv.Itoa(w.Lines)
}

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
		if f := a.feeds[feedKey(w)]; f != nil && f.add(s) {
			s.feeds = append(s.feeds, f)
		} else {
			missing = append(missing, w)
		}
	}
	subs := make([]*Subscription, len(missing))
	errs := make([]error, len(missing))
	var wg sync.WaitGroup
	slots := make(chan struct{}, opensAtOnce)
	for i, w := range missing {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			ctx, cancel := context.WithTimeout(context.Background(), callWithin)
			defer cancel()
			subs[i], errs[i] = a.client.Subscribe(ctx, SubscribeParams{Labels: map[string]string{outputOnly: "1"}, Output: []OutputWatch{w}})
		}()
	}
	wg.Wait()
	var refused error
	for _, err := range errs {
		if err != nil && (refused == nil || IsKind(err, "invalid_regex") && !IsKind(refused, "invalid_regex")) {
			refused = err
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
		f := &feed{key: feedKey(w), pane: w.Terminal, sub: subs[i], streams: map[*eventStream]struct{}{s: {}}}
		a.feeds[f.key] = f
		s.feeds = append(s.feeds, f)
		go a.tell(f)
	}
	return nil
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
		f.push(herdr.Event{Kind: herdr.EventOutputMatched, PaneID: f.pane, MatchedLine: row.Line, Text: row.Line, Standing: row.Fresh})
	}
}
