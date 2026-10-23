package push

import (
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
)

// sentLog records what the scheduler sent, safely across the timer goroutines.
type sentLog struct {
	mu   sync.Mutex
	list []Payload
}

func (l *sentLog) get() []Payload {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Payload(nil), l.list...)
}

func newTestService(t *testing.T) (*Service, *sentLog) {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	log := &sentLog{}
	s.send = func(sub *Subscription, p Payload) error {
		log.mu.Lock()
		log.list = append(log.list, p)
		log.mu.Unlock()
		return nil
	}
	sub := Subscription{Endpoint: "https://push.example/dev", Prefs: Prefs{Input: true, Done: "long"}}
	sub.Keys.P256dh, sub.Keys.Auth = "p", "a"
	s.Subscribe(sub, "")
	return s, log
}

func shrinkDelays(t *testing.T) {
	t.Helper()
	oi, od, of, ol := inputDelay, doneDelay, doneDelayFast, longTurn
	inputDelay, doneDelay, doneDelayFast, longTurn = 30*time.Millisecond, 60*time.Millisecond, 20*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { inputDelay, doneDelay, doneDelayFast, longTurn = oi, od, of, ol })
}

func TestBlockedAlertFiresAfterDelay(t *testing.T) {
	shrinkDelays(t)
	s, sent := newTestService(t)
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "p1"}, Workspace: "api", Agent: "claude", From: "working", To: "blocked"})
	time.Sleep(80 * time.Millisecond)
	if len(sent.get()) != 1 || sent.get()[0].Kind != "input" || sent.get()[0].Title != "api needs you" {
		t.Fatalf("want one input alert, got %+v", sent.get())
	}
}

func TestAnsweredAtTheDeskCancels(t *testing.T) {
	shrinkDelays(t)
	s, sent := newTestService(t)
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "p1"}, From: "working", To: "blocked"})
	time.Sleep(10 * time.Millisecond)
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "p1"}, From: "blocked", To: "working"})
	time.Sleep(80 * time.Millisecond)
	if len(sent.get()) != 0 {
		t.Fatalf("alert should have been called off, got %+v", sent.get())
	}
}

func TestDoneOnlyAfterLongTurn(t *testing.T) {
	shrinkDelays(t)
	s, sent := newTestService(t)
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "p1"}, From: "working", To: "done", Worked: 10 * time.Millisecond})
	time.Sleep(100 * time.Millisecond)
	if len(sent.get()) != 0 {
		t.Fatalf("short turn must not alert, got %+v", sent.get())
	}
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "p2"}, Workspace: "web", From: "working", To: "done", Worked: time.Second})
	time.Sleep(100 * time.Millisecond)
	if len(sent.get()) != 1 || sent.get()[0].Kind != "done" {
		t.Fatalf("long turn should alert once, got %+v", sent.get())
	}
}

func TestDoneNotFromWorkingIgnored(t *testing.T) {
	shrinkDelays(t)
	s, sent := newTestService(t)
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "p1"}, From: "idle", To: "done", Worked: time.Hour})
	time.Sleep(100 * time.Millisecond)
	if len(sent.get()) != 0 {
		t.Fatalf("idle→done is not a finish, got %+v", sent.get())
	}
}

func TestPrefsRespected(t *testing.T) {
	shrinkDelays(t)
	s, sent := newTestService(t)
	sub := Subscription{Endpoint: "https://push.example/dev", Prefs: Prefs{Input: false, Done: "always"}}
	sub.Keys.P256dh, sub.Keys.Auth = "p", "a"
	s.Subscribe(sub, "")
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "p1"}, From: "working", To: "blocked"})
	time.Sleep(60 * time.Millisecond)
	if len(sent.get()) != 0 {
		t.Fatalf("input alerts are off for this device, got %+v", sent.get())
	}
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "p2"}, From: "working", To: "done", Worked: time.Millisecond})
	time.Sleep(50 * time.Millisecond)
	if len(sent.get()) != 1 || sent.get()[0].Kind != "done" {
		t.Fatalf("done:always alerts even short turns, got %+v", sent.get())
	}
}

func TestSubscriptionsPersist(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	sub := Subscription{Endpoint: "https://push.example/x"}
	sub.Keys.P256dh, sub.Keys.Auth = "p", "a"
	s.Subscribe(sub, "")
	s2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s2.PublicKey() != s.PublicKey() {
		t.Fatal("VAPID key must survive a restart (rotating orphans every device)")
	}
	if got := s2.Subscriptions(); len(got) != 1 || got[0].Prefs.Done != "long" {
		t.Fatalf("subscription not persisted with default prefs: %+v", got)
	}
}

// TestSiblingTimerStaysCancellable: the fast device group's timer firing must leave the slow one cancellable by the next status change.
func TestSiblingTimerStaysCancellable(t *testing.T) {
	shrinkDelays(t)
	s, sent := newTestService(t)
	sub := Subscription{Endpoint: "https://push.example/dev2", Prefs: Prefs{Done: "always"}}
	sub.Keys.P256dh, sub.Keys.Auth = "p", "a"
	s.Subscribe(sub, "")

	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "p1"}, From: "working", To: "done", Worked: time.Second})
	time.Sleep(35 * time.Millisecond)
	if got := sent.get(); len(got) != 1 {
		t.Fatalf("want the fast alert only, got %+v", got)
	}
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "p1"}, From: "done", To: "working"})
	time.Sleep(80 * time.Millisecond)
	if got := sent.get(); len(got) != 1 {
		t.Fatalf("the 'long' alert must have been called off, got %+v", got)
	}
}

func TestFeedHearsAlertsWithoutSubscriptions(t *testing.T) {
	inputDelay, doneDelayFast = 10*time.Millisecond, 10*time.Millisecond
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var heard []string
	var mu sync.Mutex
	s.OnAlert = func(p Payload) {
		mu.Lock()
		heard = append(heard, p.Kind+":"+p.Title)
		mu.Unlock()
	}
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "p1"}, Workspace: "api", Agent: "claude", From: "working", To: "blocked"})
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "p2"}, Workspace: "web", From: "working", To: "done", Worked: time.Millisecond})
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "p3"}, Workspace: "quick", From: "working", To: "blocked"})
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "p3"}, From: "blocked", To: "working"}) // answered at the desk
	s.Notify(Payload{Kind: "stuck", Title: "api is stuck"})
	time.Sleep(60 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	sort.Strings(heard)
	want := []string{"done:web finished", "input:api needs you", "stuck:api is stuck"}
	if strings.Join(heard, "|") != strings.Join(want, "|") {
		t.Errorf("heard %v, want %v", heard, want)
	}
}
