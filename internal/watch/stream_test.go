package watch

import (
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/herdr/herdrfake"
	"github.com/dev1antdev/remuda/internal/machines"
)

// quickPace shrinks the pace so a test sees in milliseconds what takes seconds.
func quickPace(t *testing.T) {
	t.Helper()
	saved := []time.Duration{localEvery, localStreamingEvery, eventDebounce, memoryDebounce, localGap, lazyGap, streamRetry, freshFor}
	localEvery, localStreamingEvery, eventDebounce, memoryDebounce, localGap, lazyGap, streamRetry, freshFor =
		40*time.Millisecond, 10*time.Second, 20*time.Millisecond, 5*time.Millisecond, 40*time.Millisecond, 300*time.Millisecond, 50*time.Millisecond, 200*time.Millisecond
	t.Cleanup(func() {
		localEvery, localStreamingEvery, eventDebounce, memoryDebounce, localGap, lazyGap, streamRetry, freshFor = saved[0], saved[1], saved[2], saved[3], saved[4], saved[5], saved[6], saved[7]
	})
}

// watching runs the watcher on a fake machine's API until the test ends.
func watching(t *testing.T, f *fakeHerdr, w func(*Watcher)) (*Watcher, *machines.Machine, *events.Bus) {
	t.Helper()
	fleet := machines.NewManagerWith(t.TempDir(), nil, false, f.api(), &herdrfake.Terminals{})
	mc, err := fleet.Get(machines.LocalID)
	if err != nil {
		t.Fatal(err)
	}
	bus := events.NewBus()
	watcher := New(fleet, bus)
	if w != nil {
		w(watcher)
	}
	done := make(chan struct{})
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		watcher.run(mc, done)
	}()
	t.Cleanup(func() {
		close(done)
		select {
		case <-ended:
		case <-time.After(5 * time.Second):
			t.Error("the watcher did not stop")
		}
	})
	return watcher, mc, bus
}

func TestTheStreamSlowsThePollAndAnEventSnapshotsAtOnce(t *testing.T) {
	quickPace(t)
	f := newFakeHerdr(t, "w1:p1")
	watching(t, f, nil)

	subs := f.subscribed(t)
	if statusPanes(subs) != "w1:p1" || !strings.Contains(fingerprint(subs), `"pane.created"`) || strings.Contains(fingerprint(subs), herdr.EventOutputMatched) {
		t.Fatalf("the first stream: %s", fingerprint(subs))
	}
	waitFor(t, "the stream to stand", func() bool { return f.open() == 1 })
	time.Sleep(100 * time.Millisecond) // the snapshot a new stream asks for
	quiet := time.Now()
	time.Sleep(400 * time.Millisecond)
	if n := f.polls(quiet); n > 0 {
		t.Fatalf("behind the stream the poll slows down: %d snapshots in 400 ms at a 40 ms pace", n)
	}

	asked := time.Now()
	f.push(`{"data":{"agent":"claude","agent_status":"blocked","pane_id":"w1:p1","workspace_id":"w1"},"event":"pane.agent_status_changed"}`)
	waitFor(t, "a snapshot after an event", func() bool { return f.polls(asked) == 1 })
	if took := time.Since(asked); took > 300*time.Millisecond {
		t.Fatalf("an event's snapshot took %s", took)
	}

	burst := time.Now().Add(100 * time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	for i := 0; i < 6; i++ {
		f.push(`{"data":{"type":"layout_updated"},"event":"layout_updated"}`)
	}
	time.Sleep(200 * time.Millisecond)
	if n := f.polls(burst); n != 1 {
		t.Fatalf("a burst of events is one snapshot: %d", n)
	}

	kicked := time.Now()
	f.push(`{"data":{"type":"tab_created"},"event":"tab_created"}`)
	waitFor(t, "a snapshot after an event", func() bool { return f.polls(kicked) == 1 })
	nudged := time.Now()
	for i := 0; i < 10; i++ {
		f.push(`{"data":{"type":"pane_updated","pane":{"pane_id":"w1:p1"}},"event":"pane_updated"}`)
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if n := f.polls(nudged); n != 0 {
		t.Fatalf("a title moving right after a snapshot waits for the lazy gap: %d snapshots at once", n)
	}
	waitFor(t, "the lazy snapshot", func() bool { return f.polls(nudged) == 1 })
	if took := time.Since(kicked); took < lazyGap {
		t.Fatalf("the lazy snapshot came %s after the one before", took)
	}

	f.setPanes("w1:p1", "w1:p2")
	f.push(`{"data":{"type":"pane_created","pane":{"pane_id":"w1:p2"}},"event":"pane_created"}`)
	if got := statusPanes(f.subscribed(t)); got != "w1:p1,w1:p2" {
		t.Fatalf("a new pane's status joins the stream: %s", got)
	}
	waitFor(t, "the old stream to close once the new one stands", func() bool { return f.open() == 1 })
	f.mu.Lock()
	most := f.most
	f.mu.Unlock()
	if most > 2 {
		t.Fatalf("a stream is swapped two at a time at most: %d", most)
	}
}

// TestAPaneThatComesAndGoesSwapsNoStream: the subscriptions a snapshot asks for wait for the churn's rest, so a pane
// that appears and goes within it — a one-off command, a scene's pane — costs no stream, and the watches of the panes
// that stand are never pushed past a cap for it.
func TestAPaneThatComesAndGoesSwapsNoStream(t *testing.T) {
	quickPace(t)
	f := newFakeHerdr(t, "w1:p1")
	f.memory = true // the snapshot is the mirror's: the rest a move waits for, not the poll, is what slows it
	watching(t, f, nil)
	f.subscribed(t)
	waitFor(t, "the stream to stand", func() bool { return f.open() == 1 })
	time.Sleep(eventDebounce) // the snapshot a new stream asks for

	appeared := time.Now()
	f.setPanes("w1:p1", "w1:p2")
	f.push(`{"data":{"type":"pane_created","pane":{"pane_id":"w1:p2"}},"event":"pane_created"}`)
	waitFor(t, "the snapshot with the momentary pane", func() bool { return f.polls(appeared) == 1 })
	f.setPanes("w1:p1")
	f.push(`{"data":{"type":"pane_closed","pane":{"pane_id":"w1:p2"}},"event":"pane_closed"}`)
	time.Sleep(3 * localGap)

	if n := f.subscribes(); n != 1 {
		t.Fatalf("a pane that came and went within the rest swapped the stream: %d subscribes", n)
	}
}

// TestAStreamOpeningWhileASetSettlesHoldsTheOneThatStands: a set the snapshot moved waits for the rest, and
// a stream that opens meanwhile — an ended one, a swap that failed — subscribes to the set that stands, never to the
// one still settling: a momentary pane can cost no stream, nor place a watch of the panes that stand past a cap.
func TestAStreamOpeningWhileASetSettlesHoldsTheOneThatStands(t *testing.T) {
	quickPace(t)
	eventDebounce = 300 * time.Millisecond // the rest is long enough to open a stream inside it
	f := newFakeHerdr(t, "w1:p1")
	f.memory = true
	watching(t, f, nil)
	f.subscribed(t)
	waitFor(t, "the stream to stand", func() bool { return f.open() == 1 })
	time.Sleep(2 * memoryDebounce)

	appeared := time.Now()
	f.setPanes("w1:p1", "w1:p2")
	f.push(`{"data":{"type":"pane_created","pane":{"pane_id":"w1:p2"}},"event":"pane_created"}`)
	waitFor(t, "the snapshot with the momentary pane", func() bool { return f.polls(appeared) == 1 })

	f.closeStreams() // the next stream opens inside the rest, on the set that stands
	if got := statusPanes(f.subscribed(t)); got != "w1:p1" {
		t.Fatalf("a stream opened on the set still settling: %s", got)
	}
	time.Sleep(eventDebounce / 2) // a stream standing is no licence for the pane still settling
	if n := f.subscribes(); n != 2 {
		t.Fatalf("a reconnect opened a third stream inside the rest: %d subscribes", n)
	}
}

// TestARuleEditPromotesASettlingSetAtOnce: a rule edited is the person's move, not the snapshot's: a poll that read the
// new rule and booked it waits for no rest once Resubscribe says so.
func TestARuleEditPromotesASettlingSetAtOnce(t *testing.T) {
	quickPace(t)
	eventDebounce = 10 * time.Second // a rest no test would sit through: only the person's word promotes the set
	f := newFakeHerdr(t, "w1:p1")
	f.memory = true
	var rules atomic.Pointer[[]Output]
	watcher, _, _ := watching(t, f, func(w *Watcher) {
		w.Outputs = func(string, *herdr.Snapshot) []Output {
			if p := rules.Load(); p != nil {
				return *p
			}
			return nil
		}
	})
	f.subscribed(t)
	waitFor(t, "the stream to stand", func() bool { return f.open() == 1 })
	time.Sleep(2 * memoryDebounce)

	rules.Store(&[]Output{{Key: "deployed", Pane: "w1:p1", Match: herdr.OutputMatch{Type: "substring", Value: "DEPLOYED"}, Lines: 40, Person: true}})
	seen := time.Now()
	f.push(`{"data":{"type":"layout_updated"},"event":"layout_updated"}`)
	waitFor(t, "the snapshot with the new rule", func() bool { return f.polls(seen) == 1 })

	watcher.Resubscribe()
	subs := f.subscribed(t) // a rest of ten seconds: the stream that arrives is the person's word, not the timer's
	if got := fingerprint(subs); !strings.Contains(got, herdr.EventOutputMatched) || !strings.Contains(got, "DEPLOYED") {
		t.Fatalf("the rule's watch is not in the stream: %s", got)
	}
}

// TestASettlingSetIsNotPushedOutByTheSameSetAgain: an agent working asks for a snapshot every few milliseconds, and
// each one works out the same pending set — that must not re-arm the rest, or a new pane's watches would never open
// while the agents around it move.
func TestASettlingSetIsNotPushedOutByTheSameSetAgain(t *testing.T) {
	quickPace(t)
	f := newFakeHerdr(t, "w1:p1")
	f.memory = true
	watching(t, f, nil)
	f.subscribed(t)
	waitFor(t, "the stream to stand", func() bool { return f.open() == 1 })
	time.Sleep(eventDebounce) // the snapshot a new stream asks for

	f.setPanes("w1:p1", "w1:p2")
	f.push(`{"data":{"type":"pane_created","pane":{"pane_id":"w1:p2"}},"event":"pane_created"}`)
	waitFor(t, "the snapshot that books the new pane", func() bool { return f.open() == 1 })

	working, ended := make(chan struct{}), make(chan struct{})
	go func() { // an agent's statuses, a snapshot each, all of them the set already standing pending
		defer close(ended)
		for {
			select {
			case <-working:
				return
			case <-time.After(localGap / 4):
				f.push(`{"data":{"agent":"claude","agent_status":"working","pane_id":"w1:p1","workspace_id":"w1"},"event":"pane.agent_status_changed"}`)
			}
		}
	}()
	defer func() { close(working); <-ended }() // the goroutine stops before a cleanup restores the pace it reads
	if got := statusPanes(f.subscribed(t)); got != "w1:p1,w1:p2" {
		t.Fatalf("the new pane's watches never opened while the agents moved: %s", got)
	}
}

// TestASettlingSetTakesTheLatestTopology: panes that keep coming replace the set still settling without moving its
// rest, and the one stream that opens holds the topology as it stands then — never one per pane.
func TestASettlingSetTakesTheLatestTopology(t *testing.T) {
	quickPace(t)
	eventDebounce = 300 * time.Millisecond // both panes appear inside one rest
	f := newFakeHerdr(t, "w1:p1")
	f.memory = true
	watching(t, f, nil)
	f.subscribed(t)
	waitFor(t, "the stream to stand", func() bool { return f.open() == 1 })
	time.Sleep(eventDebounce) // the snapshot a new stream asks for

	f.setPanes("w1:p1", "w1:p2") // a set of its own, booked while p3 is still to come
	seen := time.Now()
	f.push(`{"data":{"type":"pane_created","pane":{"pane_id":"w1:p2"}},"event":"pane_created"}`)
	waitFor(t, "the snapshot with the first pane", func() bool { return f.polls(seen) == 1 })
	time.Sleep(localGap / 6)
	f.setPanes("w1:p1", "w1:p2", "w1:p3")
	seen = time.Now()
	f.push(`{"data":{"type":"pane_created","pane":{"pane_id":"w1:p3"}},"event":"pane_created"}`)
	waitFor(t, "the snapshot with the second", func() bool { return f.polls(seen) == 1 })
	if got := statusPanes(f.subscribed(t)); got != "w1:p1,w1:p2,w1:p3" {
		t.Fatalf("the settling set was not the latest topology: %s", got)
	}
	if n := f.subscribes(); n != 2 {
		t.Fatalf("a set per pane instead of one for the churn: %d subscribes", n)
	}
}

func TestAStreamThatEndsGoesBackToThePollAndOpensAgain(t *testing.T) {
	quickPace(t)
	streamRetry = 300 * time.Millisecond
	f := newFakeHerdr(t, "w1:p1")
	watching(t, f, nil)
	f.subscribed(t)
	waitFor(t, "the stream", func() bool { return f.open() == 1 })

	ended := time.Now()
	f.closeStreams()
	f.subscribed(t) // a new stream after the retry
	if n := f.polls(ended); n < 2 {
		t.Fatalf("without the stream the poll keeps today's pace: %d snapshots while it was down", n)
	}

	waitFor(t, "the stream again", func() bool { return f.answered() == 2 && f.open() == 1 })
	time.Sleep(100 * time.Millisecond)
	lost := time.Now()
	f.push(`{"id":"remuda-9","error":{"code":"events_lost","message":"the subscriber fell behind"}}`)
	f.subscribed(t)
	if since := time.Since(lost); since > 300*time.Millisecond {
		t.Fatalf("lost events open a new stream at once: %s", since)
	}
	waitFor(t, "a snapshot after lost events", func() bool { return f.polls(lost) >= 1 })
}

func TestOutputMatchesGoOnTheBusWithTheirWatch(t *testing.T) {
	quickPace(t)
	f := newFakeHerdr(t, "w1:p1")
	var mu sync.Mutex
	var heard []events.OutputMatched
	outputs := func(machine string, snap *herdr.Snapshot) []Output {
		var out []Output
		for _, pane := range snap.Panes {
			out = append(out,
				Output{Key: "serve", Pane: pane.PaneID, Match: herdr.OutputMatch{Type: "regex", Value: `Local:\s+https?://`}, Lines: 40},
				Output{Key: "rule:1", Pane: pane.PaneID, Match: herdr.OutputMatch{Type: "substring", Value: "DEPLOYED"}, Lines: 40, Person: true})
		}
		return out
	}
	_, _, bus := watching(t, f, func(w *Watcher) { w.Outputs = outputs })
	events.Subscribe(bus, func(e events.OutputMatched) {
		mu.Lock()
		heard = append(heard, e)
		mu.Unlock()
	})
	got := func() []events.OutputMatched {
		mu.Lock()
		defer mu.Unlock()
		return append([]events.OutputMatched(nil), heard...)
	}
	subs := f.subscribed(t)
	if !strings.Contains(fingerprint(subs), `"match":{"type":"substring","value":"DEPLOYED"}`) {
		t.Fatalf("the watches ride the stream: %s", fingerprint(subs))
	}
	waitFor(t, "the stream", func() bool { return f.open() == 1 })
	f.push(`{"data":{"matched_line":"  Local:   http://localhost:5173/","pane_id":"w1:p1","read":{"text":"VITE ready\n  Local:   http://localhost:5173/"}},"event":"pane.output_matched"}`)
	waitFor(t, "the match on the bus", func() bool { return len(got()) == 1 })
	if e := got()[0]; e.Watch != "serve" || e.Pane.Pane != "w1:p1" || !e.Fresh || e.Carried || !strings.Contains(e.Text, "VITE") {
		t.Fatalf("a match as the stream opened: %+v", e)
	}
	time.Sleep(250 * time.Millisecond)
	f.push(`{"data":{"matched_line":"DEPLOYED to staging","pane_id":"w1:p1","read":{"text":"DEPLOYED to staging"}},"event":"pane.output_matched"}`)
	waitFor(t, "the person's match", func() bool { return len(got()) == 2 })
	if e := got()[1]; e.Watch != "rule:1" || e.Fresh {
		t.Fatalf("a match while the stream stands: %+v", e)
	}

	f.setPanes("w1:p1", "w1:p2")
	f.push(`{"data":{"type":"pane_created","pane":{"pane_id":"w1:p2"}},"event":"pane_created"}`)
	f.subscribed(t)
	waitFor(t, "the swap", func() bool { return f.answered() == 2 && f.open() == 1 })
	f.push(`{"data":{"matched_line":"  Local:   http://localhost:5173/","pane_id":"w1:p1","read":{"text":""}},"event":"pane.output_matched"}`)
	f.push(`{"data":{"matched_line":"  Local:   http://localhost:3000/","pane_id":"w1:p2","read":{"text":""}},"event":"pane.output_matched"}`)
	waitFor(t, "the matches after the swap", func() bool { return len(got()) == 4 })
	if e := got()[2]; !e.Fresh || !e.Carried {
		t.Fatalf("the old pane's watch was held by the stream before: %+v", e)
	}
	if e := got()[3]; !e.Fresh || e.Carried {
		t.Fatalf("the new pane's watch is new: %+v", e)
	}
}

func TestHerdrRefusingThePersonsWatchesKeepsTheStream(t *testing.T) {
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
	outputs := func(machine string, snap *herdr.Snapshot) []Output {
		return []Output{
			{Key: "serve", Pane: "w1:p1", Match: herdr.OutputMatch{Type: "regex", Value: "Local:"}, Lines: 40},
			{Key: "rule:1", Pane: "w1:p1", Match: herdr.OutputMatch{Type: "regex", Value: "bad"}, Lines: 40, Person: true},
		}
	}
	watching(t, f, func(w *Watcher) {
		w.Outputs = outputs
		w.Refused = func(machine string, err error) {
			mu.Lock()
			refusals = append(refusals, err)
			mu.Unlock()
		}
	})
	if subs := fingerprint(f.subscribed(t)); !strings.Contains(subs, `"bad"`) {
		t.Fatalf("the first try carries the person's watch: %s", subs)
	}
	if subs := fingerprint(f.subscribed(t)); strings.Contains(subs, `"bad"`) || !strings.Contains(subs, `"Local:"`) {
		t.Fatalf("the second leaves it out and keeps the rest: %s", subs)
	}
	waitFor(t, "the stream without the refused watch", func() bool { return f.open() == 1 })
	mu.Lock()
	defer mu.Unlock()
	if len(refusals) != 1 || !herdr.IsCode(refusals[0], "invalid_regex") {
		t.Fatalf("the refusal is told once: %v", refusals)
	}
}

func TestTheWatcherLeavesNothingRunning(t *testing.T) {
	quickPace(t)
	before := runtime.NumGoroutine()
	f := newFakeHerdr(t, "w1:p1")
	fleet := machines.NewManagerWith(t.TempDir(), nil, false, f.api(), &herdrfake.Terminals{})
	mc, _ := fleet.Get(machines.LocalID)
	watcher := New(fleet, events.NewBus())
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		watcher.run(mc, done)
	}()
	f.subscribed(t)
	waitFor(t, "the stream", func() bool { return f.open() == 1 })
	for i := 0; i < 5; i++ { // streams that keep ending: each reconnect replaces the one before
		f.closeStreams()
		f.subscribed(t)
		waitFor(t, "the stream again", func() bool { return f.answered() == i+2 && f.open() == 1 })
	}
	close(done)
	<-stopped
	waitFor(t, "the stream to close with the watcher", func() bool { return f.open() == 0 })
	waitFor(t, "the goroutines to end", func() bool { return runtime.NumGoroutine() <= before }) // the reader of a stream ends with it
}
