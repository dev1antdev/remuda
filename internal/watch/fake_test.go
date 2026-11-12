package watch

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// fakeHerdr is a unix socket answering session.snapshot from a snapshot the test sets, and events.subscribe with a
// stream the test writes events into, the way herdr does: one request a connection, a stream kept open.
type fakeHerdr struct {
	sock string

	mu        sync.Mutex
	snap      herdr.Snapshot
	snapshots []time.Time
	streams   []net.Conn // open streams
	most      int        // the most streams open at once, once the watcher had a moment to close the one before
	started   int        // streams answered
	asked     chan []herdr.Subscription
	refuse    func([]herdr.Subscription) string // a refusal's code for a subscription set, "" to take it
}

func newFakeHerdr(t *testing.T, panes ...string) *fakeHerdr {
	t.Helper()
	dir, err := os.MkdirTemp("", "hw") // short: a unix socket path has ~100 bytes
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	f := &fakeHerdr{sock: filepath.Join(dir, "herdr.sock"), asked: make(chan []herdr.Subscription, 32)}
	f.setPanes(panes...)
	ln, err := net.Listen("unix", f.sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ln.Close()
		f.closeStreams()
	})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(conn)
		}
	}()
	return f
}

func (f *fakeHerdr) setPanes(panes ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snap = herdr.Snapshot{Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1", Label: "api"}}}
	for _, id := range panes {
		f.snap.Panes = append(f.snap.Panes, herdr.PaneInfo{PaneID: id, WorkspaceID: "w1", Agent: "claude", AgentStatus: herdr.StatusIdle})
	}
}

func (f *fakeHerdr) serve(conn net.Conn) {
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		conn.Close()
		return
	}
	var req struct {
		ID     string `json:"id"`
		Method string `json:"method"`
		Params struct {
			Subscriptions []herdr.Subscription `json:"subscriptions"`
		} `json:"params"`
	}
	json.Unmarshal(line, &req)
	answer := func(v any) {
		b, _ := json.Marshal(v)
		conn.Write(append(b, '\n'))
	}
	switch req.Method {
	case "session.snapshot":
		f.mu.Lock()
		f.snapshots = append(f.snapshots, time.Now())
		snap := f.snap
		f.mu.Unlock()
		answer(map[string]any{"id": req.ID, "result": map[string]any{"type": "session_snapshot", "snapshot": snap}})
		conn.Close()
	case "events.subscribe":
		f.asked <- req.Params.Subscriptions
		f.mu.Lock()
		refuse := f.refuse
		f.mu.Unlock()
		if refuse != nil {
			if code := refuse(req.Params.Subscriptions); code != "" {
				answer(map[string]any{"id": req.ID, "error": map[string]string{"code": code, "message": "refused"}})
				conn.Close()
				return
			}
		}
		for settle := time.Now().Add(100 * time.Millisecond); f.open() > 0 && time.Now().Before(settle); {
			time.Sleep(5 * time.Millisecond) // a stream the watcher closed is gone here a moment later
		}
		f.mu.Lock()
		f.most = max(f.most, len(f.streams)+1)
		f.mu.Unlock()
		answer(map[string]any{"id": req.ID, "result": map[string]string{"type": "subscription_started"}})
		f.mu.Lock()
		f.streams = append(f.streams, conn)
		f.started++
		f.mu.Unlock()
		reader.ReadBytes('\n') // until the watcher closes it
		f.mu.Lock()
		for i, c := range f.streams {
			if c == conn {
				f.streams = append(f.streams[:i], f.streams[i+1:]...)
				break
			}
		}
		f.mu.Unlock()
		conn.Close()
	default:
		answer(map[string]any{"id": req.ID, "result": map[string]any{"type": "ok"}})
		conn.Close()
	}
}

// push writes one line to every open stream.
func (f *fakeHerdr) push(line string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.streams {
		c.Write([]byte(line + "\n"))
	}
}

func (f *fakeHerdr) closeStreams() {
	f.mu.Lock()
	streams := f.streams
	f.mu.Unlock()
	for _, c := range streams {
		c.Close()
	}
}

// answered is how many streams were answered.
func (f *fakeHerdr) answered() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.started
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

// subscribed waits for the next events.subscribe.
func (f *fakeHerdr) subscribed(t *testing.T) []herdr.Subscription {
	t.Helper()
	select {
	case subs := <-f.asked:
		return subs
	case <-time.After(5 * time.Second):
		t.Fatal("no events.subscribe came")
		return nil
	}
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
