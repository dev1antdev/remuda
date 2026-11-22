package watch

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/herdr/herdrfake"
)

// fakeHerdr is a machine's herdr as the watcher asks it: a snapshot the test sets, and one event stream a Subscribe,
// every open stream hearing the events the test pushes — the way remuda-core answers a machine.
type fakeHerdr struct {
	mu          sync.Mutex
	snap        herdr.Snapshot
	memory      bool // the snapshot is the fake's own, as remuda-core's mirror holds one
	snapshots   []time.Time
	streams     []*watchedStream // open streams
	closes      int              // streams the watcher closed itself, not ones an error read ended
	most        int              // the most streams open at once
	started     int              // streams answered
	answers     []time.Time
	asks        int // Subscribe calls, refused or not
	asked       chan []herdr.Subscription
	refuse      func([]herdr.Subscription) string // a refusal's code for a subscription set, "" to take it
	unreachable error                             // Subscribe answers this, as a core that cannot be reached
}

func newFakeHerdr(t *testing.T, panes ...string) *fakeHerdr {
	t.Helper()
	f := &fakeHerdr{asked: make(chan []herdr.Subscription, 32)}
	f.setPanes(panes...)
	return f
}

// api is the herdr.API the watcher's machine speaks.
func (f *fakeHerdr) api() herdr.API {
	return &herdrfake.API{Memory: f.memory, Snapshot: f.snapshot, SubscribeF: f.subscribe}
}

func (f *fakeHerdr) setPanes(panes ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snap = herdr.Snapshot{Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1", Label: "api"}}}
	for _, id := range panes {
		f.snap.Panes = append(f.snap.Panes, herdr.PaneInfo{PaneID: id, WorkspaceID: "w1", Agent: "claude", AgentStatus: herdr.StatusIdle})
	}
}

func (f *fakeHerdr) snapshot() (*herdr.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snapshots = append(f.snapshots, time.Now())
	snap := f.snap
	return &snap, nil
}

func (f *fakeHerdr) subscribe(subs []herdr.Subscription) (herdr.EventStream, error) {
	select {
	case f.asked <- subs:
	default: // a test that reads none of them
	}
	f.mu.Lock()
	f.asks++
	refuse, unreachable := f.refuse, f.unreachable
	f.mu.Unlock()
	if unreachable != nil {
		return nil, unreachable
	}
	if refuse != nil {
		if code := refuse(subs); code != "" {
			return nil, &herdr.Error{Code: code, Message: "refused"}
		}
	}
	stream := &watchedStream{Stream: herdrfake.NewStream(256), f: f}
	f.mu.Lock()
	f.most = max(f.most, len(f.streams)+1)
	f.streams = append(f.streams, stream)
	f.started++
	f.answers = append(f.answers, time.Now())
	f.mu.Unlock()
	return stream, nil
}

// push hands one line to every open stream: an event as herdr sends it, or the error that ends the stream, as
// events_lost does.
func (f *fakeHerdr) push(line string) {
	event, err := parseEvent(line)
	f.mu.Lock()
	streams := append([]*watchedStream(nil), f.streams...)
	f.mu.Unlock()
	for _, s := range streams {
		if err != nil {
			s.End(err)
			continue
		}
		s.Push(event)
	}
}

// closed takes a stream out of the open ones, the first time its end is seen.
func (f *fakeHerdr) closed(s *watchedStream) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, o := range f.streams {
		if o == s {
			f.streams = append(f.streams[:i], f.streams[i+1:]...)
			return
		}
	}
}

func (f *fakeHerdr) closeStreams() {
	f.mu.Lock()
	streams := append([]*watchedStream(nil), f.streams...)
	f.mu.Unlock()
	for _, s := range streams {
		s.End(nil)
	}
}

// answered is how many streams were answered.
func (f *fakeHerdr) answered() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.started
}

// lastAnswer is when the newest stream was answered.
func (f *fakeHerdr) lastAnswer() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.answers) == 0 {
		return time.Time{}
	}
	return f.answers[len(f.answers)-1]
}

// subscribes is how many Subscribe calls came, refused or not.
func (f *fakeHerdr) subscribes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.asks
}

func (f *fakeHerdr) open() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.streams)
}

// polls is how many snapshots were asked for since from.
func (f *fakeHerdr) polls(from time.Time) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, at := range f.snapshots {
		if !at.Before(from) {
			n++
		}
	}
	return n
}

// polledAt is when the first snapshot at or after from was asked for, so a test measures the watcher's timing rather
// than how late its own goroutine was scheduled.
func (f *fakeHerdr) polledAt(from time.Time) time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, at := range f.snapshots {
		if !at.Before(from) {
			return at
		}
	}
	return time.Time{}
}

// subscribed waits for the next Subscribe.
func (f *fakeHerdr) subscribed(t *testing.T) []herdr.Subscription {
	t.Helper()
	select {
	case subs := <-f.asked:
		return subs
	case <-time.After(5 * time.Second):
		t.Fatal("no Subscribe came")
		return nil
	}
}

// watchedStream is one stream of the fake's: the events pushed into it, counted open until its reader is handed its
// end — by the fake, or by the watcher closing it.
type watchedStream struct {
	*herdrfake.Stream
	f    *fakeHerdr
	once sync.Once
}

func (s *watchedStream) Next() (herdr.Event, error) {
	event, err := s.Stream.Next()
	if err != nil {
		s.done()
	}
	return event, err
}

func (s *watchedStream) Close() error {
	s.f.mu.Lock()
	s.f.closes++
	s.f.mu.Unlock()
	s.done()
	return s.Stream.Close()
}

// closedHere is how many streams the watcher closed itself.
func (f *fakeHerdr) closedHere() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closes
}

func (s *watchedStream) done() { s.once.Do(func() { s.f.closed(s) }) }

// parseEvent reads one line as herdr sends it: an event of the stream, or the error that ends it.
func parseEvent(line string) (herdr.Event, error) {
	var said struct {
		Event string `json:"event"`
		Data  struct {
			Type        string `json:"type"`
			PaneID      string `json:"pane_id"`
			AgentStatus string `json:"agent_status"`
			MatchedLine string `json:"matched_line"`
			Read        struct {
				Text string `json:"text"`
			} `json:"read"`
			Standing *bool              `json:"standing"`
			Match    *herdr.OutputMatch `json:"match"`
			Pane     struct {
				PaneID string `json:"pane_id"`
			} `json:"pane"`
		} `json:"data"`
		Error *herdr.Error `json:"error"`
	}
	if err := json.Unmarshal([]byte(line), &said); err != nil {
		return herdr.Event{}, err
	}
	if said.Error != nil {
		return herdr.Event{}, said.Error
	}
	kind, pane := said.Event, said.Data.PaneID
	if kind == "" {
		kind = said.Data.Type
	}
	if pane == "" {
		pane = said.Data.Pane.PaneID
	}
	return herdr.Event{
		Kind: kind, PaneID: pane, AgentStatus: said.Data.AgentStatus, MatchedLine: said.Data.MatchedLine,
		Text: said.Data.Read.Text, Standing: said.Data.Standing, Watch: said.Data.Match,
	}, nil
}

// waitFor waits until cond holds, a few seconds at most.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("waited for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// statusPanes is the panes whose agent status a subscription set follows.
func statusPanes(subs []herdr.Subscription) string {
	var panes []string
	for _, s := range subs {
		if s.Type == herdr.EventAgentStatus {
			panes = append(panes, s.PaneID)
		}
	}
	return strings.Join(panes, ",")
}
