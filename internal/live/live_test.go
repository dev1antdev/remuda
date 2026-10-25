package live

import (
	"sync/atomic"
	"testing"
	"time"
)

func receive(t *testing.T, s *Subscription, within time.Duration) (Event, bool) {
	t.Helper()
	select {
	case ev, ok := <-s.Events:
		return ev, ok
	case <-time.After(within):
		return Event{}, false
	}
}

func TestRosterChangedAnnouncesOnceWhenTheFingerprintMoved(t *testing.T) {
	var calls, stamp atomic.Int32
	hub := New(Inputs{Fingerprint: func() string { calls.Add(1); return "fp" + string(rune('0'+stamp.Load())) }})
	hub.Debounce = 10 * time.Millisecond
	page := hub.Subscribe()
	defer page.Close()
	for i := 0; i < 10; i++ {
		hub.RosterChanged() // a burst of snapshots within the debounce
	}
	if ev, ok := receive(t, page, time.Second); !ok || ev.Topic != "roster" {
		t.Fatalf("want one roster event, got %+v %v", ev, ok)
	}
	if calls.Load() != 1 {
		t.Fatalf("the burst should cost one fingerprint, cost %d", calls.Load())
	}
	hub.RosterChanged() // the same fingerprint again: nothing to say
	if ev, ok := receive(t, page, 50*time.Millisecond); ok {
		t.Fatalf("an unchanged roster must not be announced, got %+v", ev)
	}
	stamp.Store(1)
	hub.RosterChanged()
	if _, ok := receive(t, page, time.Second); !ok {
		t.Fatal("a moved fingerprint must be announced")
	}
}

func TestTickAnnouncesAVersionChangeToThePanesWatchers(t *testing.T) {
	versions := map[string]string{"local|w1:p1": "1-1", "local|w2:p1": "7-7"}
	var stats atomic.Int32
	hub := New(Inputs{Version: func(machine, pane string) (string, bool) {
		stats.Add(1)
		v, ok := versions[Key(machine, pane)]
		return v, ok
	}})
	watcher := hub.Subscribe()
	defer watcher.Close()
	other := hub.Subscribe()
	defer other.Close()
	watcher.Watch([]string{"local|w1:p1", "bad", "|x", "local|"})
	if got := watcher.Watching(); len(got) != 1 || got[0] != "local|w1:p1" {
		t.Fatalf("Watch should keep only well-formed keys, kept %v", got)
	}
	now := time.Now()
	hub.Tick(now) // the first look only remembers
	if ev, ok := receive(t, watcher, 20*time.Millisecond); ok {
		t.Fatalf("the first version seen is no change, got %+v", ev)
	}
	if stats.Load() != 1 {
		t.Fatalf("only the watched pane is stat'ed, stat'ed %d", stats.Load())
	}
	versions["local|w1:p1"] = "2-2"
	hub.Tick(now.Add(500 * time.Millisecond)) // too soon for a local pane
	if stats.Load() != 1 {
		t.Fatalf("a pane is not stat'ed before its turn, stat'ed %d", stats.Load())
	}
	hub.Tick(now.Add(time.Second))
	ev, ok := receive(t, watcher, time.Second)
	if !ok || ev.Topic != "chat" || ev.Machine != "local" || ev.Pane != "w1:p1" || ev.Version != "2-2" {
		t.Fatalf("want the chat event with the new version, got %+v %v", ev, ok)
	}
	if ev, ok := receive(t, other, 20*time.Millisecond); ok {
		t.Fatalf("a page not watching the pane hears nothing, got %+v", ev)
	}
	hub.Tick(now.Add(2 * time.Second)) // unchanged: silence
	if ev, ok := receive(t, watcher, 20*time.Millisecond); ok {
		t.Fatalf("an unchanged version is not announced, got %+v", ev)
	}
}

func TestRemotePanesAreCheckedLessOften(t *testing.T) {
	var stats atomic.Int32
	hub := New(Inputs{
		Version: func(machine, pane string) (string, bool) { stats.Add(1); return "1-1", true },
		Remote:  func(machine string) bool { return machine == "gog" },
	})
	page := hub.Subscribe()
	defer page.Close()
	page.Watch([]string{"gog|w1:p1"})
	now := time.Now()
	hub.Tick(now)
	hub.Tick(now.Add(time.Second))
	hub.Tick(now.Add(2 * time.Second))
	if stats.Load() != 1 {
		t.Fatalf("a remote pane waits its three seconds, stat'ed %d", stats.Load())
	}
	hub.Tick(now.Add(3 * time.Second))
	if stats.Load() != 2 {
		t.Fatalf("then it is stat'ed again, stat'ed %d", stats.Load())
	}
}

func TestPaneChangedReachesWatchersAtOnce(t *testing.T) {
	hub := New(Inputs{})
	page := hub.Subscribe()
	defer page.Close()
	page.Watch([]string{"local|w1:p1"})
	hub.PaneChanged("local", "w9:p9")
	if ev, ok := receive(t, page, 20*time.Millisecond); ok {
		t.Fatalf("another pane's status is not this page's news, got %+v", ev)
	}
	hub.PaneChanged("local", "w1:p1")
	if ev, ok := receive(t, page, time.Second); !ok || ev.Topic != "chat" || ev.Pane != "w1:p1" {
		t.Fatalf("want the chat event, got %+v %v", ev, ok)
	}
}

func TestASlowPageDropsFramesAndBlocksNobody(t *testing.T) {
	hub := New(Inputs{})
	slow := hub.Subscribe()
	defer slow.Close()
	quick := hub.Subscribe()
	defer quick.Close()
	slow.Watch([]string{"local|w1:p1"})
	quick.Watch([]string{"local|w1:p1"})
	done := make(chan struct{})
	go func() {
		for i := 0; i < queue*3; i++ {
			hub.PaneChanged("local", "w1:p1")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("a full queue must not block the hub")
	}
	if n := len(slow.ch); n != queue {
		t.Fatalf("the slow page keeps a full queue and no more, has %d", n)
	}
	if _, ok := receive(t, quick, time.Second); !ok {
		t.Fatal("the quick page still hears")
	}
}

func TestCloseEndsEveryStream(t *testing.T) {
	hub := New(Inputs{})
	page := hub.Subscribe()
	hub.Close()
	if _, ok := <-page.Events; ok {
		t.Fatal("a closed hub closes the page's stream")
	}
	page.Close() // idempotent
	late := hub.Subscribe()
	if _, ok := <-late.Events; ok {
		t.Fatal("a subscription after Close is closed at once")
	}
	hub.RosterChanged() // nothing booked after Close
}
