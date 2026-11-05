package push

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
)

func TestAFinishSaysHowLongItsTurnWorked(t *testing.T) {
	shrinkDelays(t)
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var heard []Payload
	var mu sync.Mutex
	s.OnAlert = func(p Payload) {
		mu.Lock()
		heard = append(heard, p)
		mu.Unlock()
	}
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "p1"}, Workspace: "api", From: "working", To: "done", Worked: 95 * time.Second})
	time.Sleep(60 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(heard) != 1 || heard[0].Worked != 95 {
		t.Fatalf("heard %+v, want one finish of 95 s", heard)
	}
}

func TestADeviceKeepsItsLastDelivery(t *testing.T) {
	s, _ := newTestService(t)
	if _, last, known := s.Device("https://push.example/dev"); !known || last != nil {
		t.Fatalf("a device nothing was sent to: known %v, last %+v", known, last)
	}
	if _, _, known := s.Device("https://push.example/other"); known {
		t.Fatal("an endpoint this cockpit never saw counts as known")
	}
	if err := s.SendTest("https://push.example/dev"); err != nil {
		t.Fatal(err)
	}
	_, last, _ := s.Device("https://push.example/dev")
	if last == nil || last.Kind != "test" || last.Error != "" || last.At.IsZero() {
		t.Fatalf("after a test: last %+v", last)
	}
	s.send = func(*Subscription, Payload) error { return errors.New("push service answered 403") }
	s.Notify(Payload{Kind: "stuck", Title: "api is stuck"})
	_, last, _ = s.Device("https://push.example/dev")
	if last == nil || last.Kind != "stuck" || last.Error != "push service answered 403" {
		t.Fatalf("after a refused push: last %+v", last)
	}
}

func TestADroppedDeviceKeepsNoDelivery(t *testing.T) {
	s, _ := newTestService(t)
	gone := "https://push.example/dev"
	s.send = func(sub *Subscription, _ Payload) error {
		s.Unsubscribe(sub.Endpoint) // what Send does on the push service's 404 or 410
		return errors.New("subscription gone (410)")
	}
	_ = s.SendTest(gone)
	s.mu.Lock()
	_, kept := s.last[gone]
	s.mu.Unlock()
	if kept {
		t.Fatal("a delivery is kept for an endpoint the push service dropped")
	}
	s.Subscribe(Subscription{Endpoint: gone}, "")
	s.send = func(*Subscription, Payload) error { return nil }
	_ = s.SendTest(gone)
	s.Unsubscribe(gone)
	s.mu.Lock()
	_, kept = s.last[gone]
	s.mu.Unlock()
	if kept {
		t.Fatal("a delivery outlives its device's unsubscription")
	}
}
