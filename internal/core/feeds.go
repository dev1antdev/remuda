package core

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// feed is one core subscription telling one pane's output watches, shared by every stream asking for those same
// watches. The watcher opens a stream beside the one before it whenever a pane comes or goes; joining the feed that
// stands, the new stream is not told every matching row still on the pane's screen as if the core had just seen it.
// A pane whose watches changed gets a feed of its own, opened while the old one still tells, so a row printed then
// reaches the old stream as it appears.
type feed struct {
	key  string
	pane string
	sub  *Subscription

	mu      sync.Mutex
	streams map[*eventStream]struct{}
	ended   bool
}

// feedKey names a pane's watches as asked, in order.
func feedKey(pane string, watches []OutputWatch) string {
	var b strings.Builder
	b.WriteString(pane)
	for _, w := range watches {
		b.WriteString("\x00" + w.Match.Type + "\x00" + w.Match.Value + "\x00" + strconv.Itoa(w.Lines))
	}
	return b.String()
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

// join hands the stream the panes' watches: a feed already telling a pane's is joined, the others opened. On a
// refusal the stream leaves every feed it joined or opened.
func (a *Adapter) join(s *eventStream, panes []string, watches map[string][]OutputWatch) error {
	a.feedMu.Lock()
	defer a.feedMu.Unlock()
	for _, pane := range panes {
		key := feedKey(pane, watches[pane])
		f := a.feeds[key]
		if f == nil || !f.add(s) {
			ctx, cancel := context.WithTimeout(context.Background(), callWithin)
			sub, err := a.client.Subscribe(ctx, SubscribeParams{Labels: map[string]string{outputOnly: "1"}, Output: watches[pane]})
			cancel()
			if err != nil {
				a.leaveLocked(s)
				return a.refusal(err, "pane_not_found")
			}
			f = &feed{key: key, pane: pane, sub: sub, streams: map[*eventStream]struct{}{s: {}}}
			a.feeds[key] = f
			go a.tell(f)
		}
		s.feeds = append(s.feeds, f)
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
