package herdr

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeStream is a unix socket that answers one events.subscribe the way herdr does and hands the test the connection.
type fakeStream struct {
	sock  string
	conns chan net.Conn
	asked chan []Subscription
}

func newFakeStream(t *testing.T, answer string) *fakeStream {
	t.Helper()
	dir, err := os.MkdirTemp("", "hev") // short: a unix socket path has ~100 bytes
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	f := &fakeStream{sock: filepath.Join(dir, "herdr.sock"), conns: make(chan net.Conn, 4), asked: make(chan []Subscription, 4)}
	ln, err := net.Listen("unix", f.sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			line, err := bufio.NewReader(conn).ReadBytes('\n')
			if err != nil {
				conn.Close()
				continue
			}
			var req struct {
				Method string `json:"method"`
				Params struct {
					Subscriptions []Subscription `json:"subscriptions"`
				} `json:"params"`
			}
			json.Unmarshal(line, &req)
			if req.Method != "events.subscribe" {
				conn.Close()
				continue
			}
			f.asked <- req.Params.Subscriptions
			conn.Write([]byte(answer + "\n"))
			f.conns <- conn
		}
	}()
	return f
}

const started = `{"id":"remuda-1","result":{"type":"subscription_started"}}`

func TestSubscribeReadsHerdrsEventsUntilTheStreamEnds(t *testing.T) {
	f := newFakeStream(t, started)
	var transport []error
	client := NewClient(f.sock)
	client.OnTransportError = func(err error) { transport = append(transport, err) }
	want := []Subscription{{Type: "pane.created"}, {Type: EventAgentStatus, PaneID: "w1:p1"},
		{Type: EventOutputMatched, PaneID: "w1:p1", Source: "recent", Lines: 40, Match: &OutputMatch{Type: "regex", Value: `^--- FAIL: `}}}
	stream, err := client.Subscribe(want)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if asked := <-f.asked; len(asked) != 3 || asked[2].Match == nil || asked[2].Match.Value != `^--- FAIL: ` || asked[1].PaneID != "w1:p1" {
		t.Fatalf("subscriptions sent: %+v", asked)
	}
	conn := <-f.conns
	// herdr's own frames, a global kind with an underscore and the per-pane ones with a dot
	conn.Write([]byte(`{"data":{"pane":{"agent_status":"unknown","pane_id":"w1:p2","tab_id":"w1:t2","workspace_id":"w1"},"type":"pane_created"},"event":"pane_created"}` + "\n\n"))
	conn.Write([]byte(`{"data":{"agent":"claude","agent_status":"blocked","pane_id":"w1:p1","workspace_id":"w1"},"event":"pane.agent_status_changed"}` + "\n"))
	conn.Write([]byte(`{"data":{"matched_line":"--- FAIL: TestX (0.00s)","pane_id":"w1:p1","read":{"format":"text","pane_id":"w1:p1","revision":0,"source":"recent_unwrapped","tab_id":"w1:t1","text":"ok\n--- FAIL: TestX (0.00s)\nFAIL","truncated":false,"workspace_id":"w1"}},"event":"pane.output_matched"}` + "\n"))
	conn.Write([]byte(`{"id":"remuda-1","error":{"code":"events_lost","message":"the subscriber fell behind"}}` + "\n"))
	conn.Close()

	got, err := stream.Next()
	if err != nil || got.Kind != "pane_created" || got.PaneID != "w1:p2" {
		t.Fatalf("pane created: %+v %v", got, err)
	}
	got, err = stream.Next()
	if err != nil || got.Kind != EventAgentStatus || got.PaneID != "w1:p1" || got.AgentStatus != StatusBlocked {
		t.Fatalf("status: %+v %v", got, err)
	}
	got, err = stream.Next()
	if err != nil || got.Kind != EventOutputMatched || got.MatchedLine != "--- FAIL: TestX (0.00s)" || !strings.Contains(got.Text, "FAIL") {
		t.Fatalf("match: %+v %v", got, err)
	}
	if _, err = stream.Next(); !IsCode(err, ErrEventsLost) {
		t.Fatalf("events lost is herdr's own error: %v", err)
	}
	if _, err = stream.Next(); err == nil || IsCode(err, ErrEventsLost) {
		t.Fatalf("the stream ended: %v", err)
	}
	if len(transport) != 1 {
		t.Fatalf("a stream that ended under remuda is a transport error, once: %v", transport)
	}
}

func TestSubscribeRefusedIsHerdrsError(t *testing.T) {
	f := newFakeStream(t, `{"id":"remuda-1","error":{"code":"invalid_regex","message":"regex parse error"}}`)
	client := NewClient(f.sock)
	client.OnTransportError = func(err error) { t.Errorf("a refusal is no transport error: %v", err) }
	if _, err := client.Subscribe([]Subscription{{Type: "pane.created"}}); !IsCode(err, "invalid_regex") {
		t.Fatalf("refusal: %v", err)
	}
}

func TestStreamClosedHereEndsNextQuietly(t *testing.T) {
	f := newFakeStream(t, started)
	client := NewClient(f.sock)
	client.OnTransportError = func(err error) { t.Errorf("a stream closed here is no transport error: %v", err) }
	stream, err := client.Subscribe(nil)
	if err != nil {
		t.Fatal(err)
	}
	<-f.conns
	done := make(chan error, 1)
	go func() {
		_, err := stream.Next()
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	stream.Close()
	select {
	case err := <-done:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("Next after Close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Next still waits after Close")
	}
}

func TestSubscribeWithNobodyThereIsATransportError(t *testing.T) {
	var transport int
	client := NewClient(filepath.Join(t.TempDir(), "none.sock"))
	client.OnTransportError = func(error) { transport++ }
	if _, err := client.Subscribe(nil); err == nil || transport != 1 {
		t.Fatalf("no server: %v, %d transport errors", err, transport)
	}
}
