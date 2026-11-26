package restart

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/coreapi"
	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/state"
)

func settledPane(paneID, session string) coreapi.PaneInfo {
	return coreapi.PaneInfo{PaneID: paneID, Agent: "claude", AgentStatus: "idle", AgentSession: &coreapi.AgentSession{Kind: "id", Value: session}}
}

// armed is the booking's timer, if one runs; stopped with the answer.
func armed(r *Restarter, key string) *time.Timer {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.pending[key]
	if b == nil {
		return nil
	}
	return b.timer
}

func TestABookedRestartSurvivesARestartOfRemuda(t *testing.T) {
	dir := t.TempDir()
	first := New(nil)
	first.Open(dir)
	first.book("studio", "w1:p1", "claude", "a1b2c3")
	if n := first.Pending("studio"); n != 1 {
		t.Fatalf("booked: %d", n)
	}
	if _, err := os.Stat(filepath.Join(dir, bookingsFile)); err != nil {
		t.Fatalf("the booking was not written: %v", err)
	}

	second := New(nil)
	second.Open(dir)
	if n := second.Pending("studio"); n != 1 {
		t.Fatalf("after a restart: %d", n)
	}
	second.OnSnapshot(events.Snapshot{Machine: "studio", Snap: &coreapi.Snapshot{Panes: []coreapi.PaneInfo{settledPane("w1:p1", "a1b2c3")}}})
	if timer := armed(second, "studio|w1:p1"); timer == nil {
		t.Fatal("a settled chat of a loaded booking was not armed")
	} else {
		timer.Stop()
	}

	// another session in the pane is not the chat the booking was made for
	third := New(nil)
	third.Open(dir)
	third.OnSnapshot(events.Snapshot{Machine: "studio", Snap: &coreapi.Snapshot{Panes: []coreapi.PaneInfo{settledPane("w1:p1", "z9y8x7")}}})
	if n := third.Pending("studio"); n != 0 {
		t.Fatalf("a booking for another session stayed: %d", n)
	}
	fourth := New(nil)
	fourth.Open(dir)
	if n := fourth.Pending("studio"); n != 0 {
		t.Fatalf("a dropped booking came back: %d", n)
	}
}

func TestABookingOlderThanItsWindowIsNotLoaded(t *testing.T) {
	dir := t.TempDir()
	if err := state.SaveJSON(dir, bookingsFile, map[string]savedBooking{
		"studio|w1:p1": {Kind: "claude", Session: "a1b2c3", At: time.Now().Add(-bookingTTL - time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	r := New(nil)
	r.Open(dir)
	if n := r.Pending("studio"); n != 0 {
		t.Fatalf("an old booking was loaded: %d", n)
	}
}

func TestAClosedPaneDropsItsBooking(t *testing.T) {
	dir := t.TempDir()
	r := New(nil)
	r.Open(dir)
	r.book("studio", "w1:p1", "claude", "a1b2c3")
	r.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "studio", Pane: "w1:p1"}, Kind: "claude", To: events.Gone})
	if n := r.Pending("studio"); n != 0 {
		t.Fatalf("a closed pane kept its booking: %d", n)
	}
	again := New(nil)
	again.Open(dir)
	if n := again.Pending("studio"); n != 0 {
		t.Fatalf("the dropped booking came back after a restart: %d", n)
	}
}
