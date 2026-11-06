package push

import (
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
)

func TestAMuteHoldsUntilItsTimeAndIsKeptOnDisk(t *testing.T) {
	dir := t.TempDir()
	clock := time.Date(2026, 10, 29, 18, 0, 0, 0, time.UTC)
	m := loadMutes(dir)
	m.now = func() time.Time { return clock }
	if err := m.Mute("local", "/w/api", clock.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if until, ok := m.Until("local", "/w/api"); !ok || !until.Equal(clock.Add(time.Hour)) {
		t.Fatalf("Until = %v %v", until, ok)
	}
	if _, ok := m.Until("local", "/w/other"); ok {
		t.Fatal("another folder reads muted")
	}
	if _, ok := m.Until("gog", "/w/api"); ok {
		t.Fatal("the same folder on another machine reads muted")
	}
	again := loadMutes(dir)
	again.now = func() time.Time { return clock }
	if _, ok := again.Until("local", "/w/api"); !ok {
		t.Fatal("the mute did not survive a reload")
	}
	clock = clock.Add(61 * time.Minute)
	if _, ok := again.Until("local", "/w/api"); ok {
		t.Fatal("an expired mute still holds")
	}
	if len(again.All()) != 0 {
		t.Fatalf("All after expiry = %+v", again.All())
	}
	if err := m.Mute("local", "/w/api", clock.Add(-time.Minute)); err == nil {
		t.Fatal("a time already past was accepted")
	}
	if err := m.Mute("", "/w/api", clock.Add(time.Hour)); err == nil {
		t.Fatal("no machine was accepted")
	}
}

func TestUnmuteEndsTheSilenceNow(t *testing.T) {
	m := loadMutes(t.TempDir())
	if err := m.Mute("local", "/w/api", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := m.Unmute("local", "/w/api"); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Until("local", "/w/api"); ok {
		t.Fatal("still muted after Unmute")
	}
	if err := m.Unmute("local", "/w/never"); err != nil {
		t.Fatalf("unmuting a folder that was not muted: %v", err)
	}
}

func TestAMutedFolderSendsNothingAndRecordsNothing(t *testing.T) {
	shrinkDelays(t)
	s, sent := newTestService(t)
	var mu sync.Mutex
	var recorded []string
	s.OnAlert = func(p Payload) {
		mu.Lock()
		recorded = append(recorded, p.Title)
		mu.Unlock()
	}
	s.Folder = func(machine, pane string) string {
		if pane == "p1" {
			return "/w/quiet"
		}
		return "/w/loud"
	}
	if err := s.Mutes.Mute("local", "/w/quiet", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "p1"}, Workspace: "quiet", Agent: "claude", From: "working", To: "blocked"})
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "p2"}, Workspace: "loud", Agent: "claude", From: "working", To: "blocked"})
	s.Notify(Payload{Machine: "local", Pane: "p1", Kind: "stuck", Title: "quiet looks stuck", Tag: "stuck"})
	s.Notify(Payload{Machine: "local", Pane: "p2", Kind: "stuck", Title: "loud looks stuck", Tag: "stuck"})
	time.Sleep(80 * time.Millisecond)
	for _, p := range sent.get() {
		if p.Pane == "p1" {
			t.Fatalf("the muted pane's alert was sent: %+v", p)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(sent.get()) != 2 || len(recorded) != 2 || recorded[0] != "loud looks stuck" || recorded[1] != "loud needs you" {
		t.Fatalf("sent %d, recorded %v: want the loud pane's two alone", len(sent.get()), recorded)
	}
}
