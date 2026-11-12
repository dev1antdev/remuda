package watch

import (
	"bytes"
	"errors"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
)

// lockedLog is the log's output while a test runs.
type lockedLog struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lockedLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lockedLog) count(words string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Count(l.buf.String(), words)
}

func capturedLog(t *testing.T) *lockedLog {
	t.Helper()
	out := &lockedLog{}
	before := log.Writer()
	log.SetOutput(out)
	t.Cleanup(func() { log.SetOutput(before) })
	return out
}

func TestAHerdrWithoutStreamsIsAskedAgainQuietly(t *testing.T) {
	quickPace(t)
	saved := unsupportedRetry
	unsupportedRetry = 60 * time.Millisecond
	t.Cleanup(func() { unsupportedRetry = saved })
	said := capturedLog(t)
	f := newFakeHerdr(t, "w1:p1")
	f.refuse = func([]herdr.Subscription) string { return "invalid_request" }
	watching(t, f, nil)
	waitFor(t, "a few tries", func() bool { return f.subscribes() >= 4 })
	if n := said.count("keeps no event stream"); n != 1 {
		t.Fatalf("a herdr without streams is told once, however often it is asked again: %d times", n)
	}
}

// refusingWrites is a connection whose events.subscribe never reaches herdr, as a bridge whose ssh session fails.
type refusingWrites struct {
	net.Conn
	tries *atomic.Int32
}

func (c refusingWrites) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("events.subscribe")) {
		c.tries.Add(1)
		return 0, errors.New("broken pipe")
	}
	return c.Conn.Write(p)
}

func TestAStreamThatCannotBeReachedIsTriedLessAndLessOften(t *testing.T) {
	quickPace(t)
	f := newFakeHerdr(t, "w1:p1")
	var tries atomic.Int32
	watching(t, f, func(w *Watcher) {
		mc, _ := w.machines.Get(machines.LocalID)
		client, _, _ := mc.Ready()
		client.Dial = func(timeout time.Duration) (net.Conn, error) {
			conn, err := net.DialTimeout("unix", f.sock, timeout)
			if err != nil {
				return nil, err
			}
			return refusingWrites{Conn: conn, tries: &tries}, nil
		}
	})
	waitFor(t, "the first try", func() bool { return tries.Load() >= 1 })
	time.Sleep(1500 * time.Millisecond)
	if n := tries.Load(); n > 8 {
		t.Fatalf("a stream that cannot be reached backs off from 50 ms: %d tries in 1.5 s", n)
	}
}

func TestTheRefusedWatchesStayToldWhileTheyAreLeftOut(t *testing.T) {
	quickPace(t)
	f := newFakeHerdr(t, "w1:p1")
	f.refuse = func(subs []herdr.Subscription) string {
		if strings.Contains(fingerprint(subs), `"value":"bad"`) {
			return "invalid_regex"
		}
		return ""
	}
	var mu sync.Mutex
	var refusals []error
	watching(t, f, func(w *Watcher) {
		w.Outputs = func(string, *herdr.Snapshot) []Output {
			return []Output{{Key: "rule:1", Pane: "w1:p1", Match: herdr.OutputMatch{Type: "regex", Value: "bad"}, Lines: 40, Person: true}}
		}
		w.Refused = func(machine string, err error) {
			mu.Lock()
			refusals = append(refusals, err)
			mu.Unlock()
		}
	})
	f.subscribed(t)
	f.subscribed(t)
	waitFor(t, "the stream without the refused watch", func() bool { return f.open() == 1 })
	f.closeStreams() // herdr restarted: the next stream leaves the watch out as before
	f.subscribed(t)
	waitFor(t, "the stream again", func() bool { return f.answered() == 2 && f.open() == 1 })
	time.Sleep(100 * time.Millisecond) // the watcher's word once herdr answered
	mu.Lock()
	defer mu.Unlock()
	if len(refusals) != 1 || refusals[0] == nil {
		t.Fatalf("the refusal stands while the watch is left out: %v", refusals)
	}
}

func TestAStreamAfterOneThatEndedCarriesItsWatches(t *testing.T) {
	quickPace(t)
	f := newFakeHerdr(t, "w1:p1")
	var mu sync.Mutex
	var heard []events.OutputMatched
	_, _, bus := watching(t, f, func(w *Watcher) {
		w.Outputs = func(string, *herdr.Snapshot) []Output {
			return []Output{{Key: "tests", Pane: "w1:p1", Match: herdr.OutputMatch{Type: "substring", Value: "FAIL"}, Lines: 40}}
		}
	})
	events.Subscribe(bus, func(e events.OutputMatched) {
		mu.Lock()
		heard = append(heard, e)
		mu.Unlock()
	})
	f.subscribed(t)
	waitFor(t, "the stream", func() bool { return f.open() == 1 })
	f.closeStreams() // herdr restarted, the stream ended
	f.subscribed(t)
	waitFor(t, "the stream again", func() bool { return f.answered() == 2 && f.open() == 1 })
	f.push(`{"data":{"matched_line":"--- FAIL: TestLogin","pane_id":"w1:p1","read":{"text":""}},"event":"pane.output_matched"}`)
	waitFor(t, "the match", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(heard) == 1
	})
	mu.Lock()
	defer mu.Unlock()
	if e := heard[0]; !e.Carried {
		t.Fatalf("the watch the ended stream held is carried, so a row that came while none stood is news: %+v", e)
	}
}
