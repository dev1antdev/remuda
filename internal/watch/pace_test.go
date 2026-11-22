package watch

import (
	"testing"
	"time"
)

// TestAnInMemorySnapshotDoesNotWaitThePace: a client that holds the machine in memory — remuda-core's adapter — takes
// the snapshot an event asks for as the burst's rest is over, and straight again after one; a client that answers a
// snapshot as a request keeps herdr's pace, the debounce and the gap. A burst of events is one snapshot either way.
// The paces are the ones that ship, so the numbers this logs are the delivery's.
func TestAnInMemorySnapshotDoesNotWaitThePace(t *testing.T) {
	status := `{"data":{"agent":"claude","agent_status":"blocked","pane_id":"w1:p1","workspace_id":"w1"},"event":"pane.agent_status_changed"}`
	layout := `{"data":{"type":"layout_updated"},"event":"layout_updated"}`
	for _, request := range []bool{true, false} {
		name := "memory"
		if request {
			name = "request"
		}
		t.Run(name, func(t *testing.T) {
			f := newFakeHerdr(t, "w1:p1")
			f.memory = !request
			watching(t, f, nil)
			f.subscribed(t)
			waitFor(t, "the stream to stand", func() bool { return f.open() == 1 })
			time.Sleep(localGap + eventDebounce) // the snapshot a new stream asks for, and any the pace held back

			asked := time.Now()
			f.push(status)
			waitFor(t, "a snapshot after an event", func() bool { return f.polls(asked) == 1 })
			took := f.polledAt(asked).Sub(asked) // the fake's own clock: a late test goroutine says nothing of the pace
			t.Logf("an event's snapshot: %s", took)
			if request && took < eventDebounce {
				t.Fatalf("a request's snapshot waits for the rest of the burst: %s", took)
			}
			if !request && took >= eventDebounce {
				t.Fatalf("an in-memory snapshot waited %s, the debounce a request's pace would add", took)
			}

			right := time.Now()
			f.push(status)
			waitFor(t, "a snapshot right after one", func() bool { return f.polls(right) == 1 })
			after := f.polledAt(right).Sub(right)
			t.Logf("an event right after a snapshot: %s", after)
			if request && after < localGap-eventDebounce {
				t.Fatalf("a request's snapshot keeps the gap: %s", after)
			}
			if !request && after >= eventDebounce {
				t.Fatalf("an in-memory snapshot keeps a gap: %s", after)
			}

			burst := time.Now()
			for range 6 {
				f.push(layout)
			}
			waitFor(t, "a snapshot after a burst", func() bool { return f.polls(burst) >= 1 })
			time.Sleep(2 * eventDebounce)
			// one snapshot for the burst, two if the reader was scheduled away in the middle of it — never one per event
			if n := f.polls(burst); n > 2 {
				t.Fatalf("a burst of events is one snapshot: %d", n)
			}
		})
	}
}
