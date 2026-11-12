package watch

import (
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
)

// cpuTime is the CPU this process used so far.
func cpuTime(t *testing.T) time.Duration {
	t.Helper()
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		t.Fatal(err)
	}
	return time.Duration(usage.Utime.Nano() + usage.Stime.Nano())
}

func TestAStreamHerdrKeepsRefusingIsTriedAtItsPace(t *testing.T) {
	quickPace(t)
	f := newFakeHerdr(t, "w1:p1")
	f.refuse = func([]herdr.Subscription) string { return "pane_not_found" }
	watching(t, f, nil)
	waitFor(t, "the first try", func() bool { return f.subscribes() >= 1 })
	from := f.subscribes()
	time.Sleep(500 * time.Millisecond)
	if n := f.subscribes() - from; n > 25 {
		t.Fatalf("a refused stream is tried a retry apart, a snapshot apart at most: %d tries in 500 ms at a 50 ms retry", n)
	}
}

func TestAHerdrWithoutStreamsCostsNothingWhileItWaits(t *testing.T) {
	quickPace(t)
	f := newFakeHerdr(t, "w1:p1")
	f.refuse = func([]herdr.Subscription) string { return "invalid_request" } // a herdr that knows no events.subscribe
	var mu sync.Mutex
	var refusals []error
	watching(t, f, func(w *Watcher) {
		w.Outputs = func(string, *herdr.Snapshot) []Output {
			return []Output{{Key: "rule:1", Pane: "w1:p1", Match: herdr.OutputMatch{Type: "substring", Value: "DEPLOYED"}, Lines: 40, Person: true}}
		}
		w.Refused = func(machine string, err error) {
			mu.Lock()
			refusals = append(refusals, err)
			mu.Unlock()
		}
	})
	waitFor(t, "the first try", func() bool { return f.subscribes() >= 1 })
	time.Sleep(100 * time.Millisecond)
	before := cpuTime(t)
	time.Sleep(500 * time.Millisecond)
	if used := cpuTime(t) - before; used > 200*time.Millisecond {
		t.Fatalf("waiting out a herdr without a stream spins: %s of CPU in 500 ms", used)
	}
	if n := f.subscribes(); n != 1 {
		t.Fatalf("a herdr that knows no stream is asked once, not again without the person's watches: %d", n)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(refusals) != 0 {
		t.Fatalf("a herdr without streams is no refusal of the person's triggers: %v", refusals)
	}
}

func TestAStreamHerdrEndedIsClosedHere(t *testing.T) {
	quickPace(t)
	f := newFakeHerdr(t, "w1:p1")
	watching(t, f, nil)
	f.subscribed(t)
	waitFor(t, "the stream", func() bool { return f.open() == 1 })
	f.push(`{"id":"remuda-9","error":{"code":"events_lost","message":"the subscriber fell behind"}}`)
	f.subscribed(t)
	waitFor(t, "the stream that lost events to close here", func() bool { return f.answered() == 2 && f.open() == 1 })
}

// closings counts the connections a client dialled and closed.
type closings struct {
	dialled, closed atomic.Int32
}

type countedConn struct {
	net.Conn
	by   *closings
	once sync.Once
}

func (c *countedConn) Close() error {
	c.once.Do(func() { c.by.closed.Add(1) })
	return c.Conn.Close()
}

func TestEveryStreamThatEndsLeavesNoConnection(t *testing.T) {
	quickPace(t)
	f := newFakeHerdr(t, "w1:p1")
	var count closings
	watching(t, f, func(w *Watcher) {
		mc, _ := w.machines.Get(machines.LocalID)
		client, _, _ := mc.Ready()
		client.Dial = func(timeout time.Duration) (net.Conn, error) {
			conn, err := net.DialTimeout("unix", f.sock, timeout)
			if err != nil {
				return nil, err
			}
			count.dialled.Add(1)
			return &countedConn{Conn: conn, by: &count}, nil
		}
	})
	f.subscribed(t)
	waitFor(t, "the stream", func() bool { return f.open() == 1 })
	for i := 0; i < 3; i++ {
		f.closeStreams()
		f.subscribed(t)
		waitFor(t, "the stream again", func() bool { return f.answered() == i+2 && f.open() == 1 })
	}
	waitFor(t, "every connection but the standing stream closed", func() bool { return count.dialled.Load()-count.closed.Load() == 1 })
}

func TestASwapHerdrRefusedIsTriedAgain(t *testing.T) {
	quickPace(t)
	f := newFakeHerdr(t, "w1:p1")
	var refused atomic.Int32
	f.refuse = func(subs []herdr.Subscription) string {
		if strings.Contains(statusPanes(subs), "w1:p2") && refused.Add(1) == 1 {
			return "pane_not_found" // a moment's race: the pane stood in the snapshot, herdr not done making it
		}
		return ""
	}
	watching(t, f, nil)
	f.subscribed(t)
	waitFor(t, "the stream", func() bool { return f.open() == 1 })
	f.setPanes("w1:p1", "w1:p2")
	f.push(`{"data":{"type":"pane_created","pane":{"pane_id":"w1:p2"}},"event":"pane_created"}`)
	f.subscribed(t) // refused
	if got := statusPanes(f.subscribed(t)); got != "w1:p1,w1:p2" {
		t.Fatalf("the swap is tried again: %s", got)
	}
	waitFor(t, "the new stream alone", func() bool { return f.answered() == 2 && f.open() == 1 })
}

func TestANewStreamIsFollowedByASnapshot(t *testing.T) {
	quickPace(t)
	for _, bridged := range []bool{false, true} {
		f := newFakeHerdr(t, "w1:p1")
		watching(t, f, func(w *Watcher) {
			if bridged {
				mc, _ := w.machines.Get(machines.LocalID)
				client, _, _ := mc.Ready()
				client.Dial = func(timeout time.Duration) (net.Conn, error) { return net.DialTimeout("unix", f.sock, timeout) }
			}
		})
		f.subscribed(t)
		waitFor(t, "the stream", func() bool { return f.open() == 1 })
		f.setPanes("w1:p1", "w1:p2")
		f.push(`{"data":{"type":"pane_created","pane":{"pane_id":"w1:p2"}},"event":"pane_created"}`)
		f.subscribed(t)
		waitFor(t, "the new stream", func() bool { return f.answered() == 2 })
		swapped := f.lastAnswer()
		waitFor(t, "a snapshot after the new stream stood", func() bool { return f.polls(swapped) >= 1 }) // what the new pane did before it is in it
	}
}

func TestAnEditedWatchIsNotCarried(t *testing.T) {
	quickPace(t)
	f := newFakeHerdr(t, "w1:p1")
	var pattern atomic.Value
	pattern.Store("DEPLOYED")
	var mu sync.Mutex
	var heard []events.OutputMatched
	watcher, _, bus := watching(t, f, func(w *Watcher) {
		w.Outputs = func(string, *herdr.Snapshot) []Output {
			return []Output{{Key: "rule:1", Pane: "w1:p1", Match: herdr.OutputMatch{Type: "substring", Value: pattern.Load().(string)}, Lines: 40, Person: true}}
		}
	})
	events.Subscribe(bus, func(e events.OutputMatched) {
		mu.Lock()
		heard = append(heard, e)
		mu.Unlock()
	})
	f.subscribed(t)
	waitFor(t, "the stream", func() bool { return f.open() == 1 })
	pattern.Store("SHIPPED")
	watcher.Resubscribe()
	f.subscribed(t)
	waitFor(t, "the stream on the edited rule", func() bool { return f.answered() == 2 && f.open() == 1 })
	f.push(`{"data":{"matched_line":"SHIPPED to staging","pane_id":"w1:p1","read":{"text":"SHIPPED to staging"}},"event":"pane.output_matched"}`)
	waitFor(t, "the match", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(heard) == 1
	})
	mu.Lock()
	defer mu.Unlock()
	if e := heard[0]; e.Carried {
		t.Fatalf("a rule looking for something else is a new watch, whatever its key: %+v", e)
	}
}
