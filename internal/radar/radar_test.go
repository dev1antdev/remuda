package radar

import (
	"testing"
	"time"

	"github.com/dev1antdev/herdr-ui/internal/herdr"
)

func snap(status string, focused bool) *herdr.Snapshot {
	return &herdr.Snapshot{Panes: []herdr.PaneInfo{{PaneID: "p1", Agent: "claude", AgentStatus: status, Focused: focused}}}
}

func display(t *Tracker, s *herdr.Snapshot) Display {
	return t.Display("m", &s.Panes[0])
}

func TestDoneIsHeldUntilSeen(t *testing.T) {
	tr := New(t.TempDir(), nil)
	now := time.Now()
	tr.Observe("m", snap(herdr.StatusWorking, false), now)
	tr.Observe("m", snap(herdr.StatusWorking, false), now.Add(time.Second))
	// herdr lets go of done within seconds; the tick must not
	tr.Observe("m", snap(herdr.StatusDone, false), now.Add(5*time.Second))
	for i := 0; i < 5; i++ {
		s := snap(herdr.StatusIdle, false)
		tr.Observe("m", s, now.Add(time.Duration(10+i)*time.Second))
		if d := display(tr, s); d.State != herdr.StatusDone {
			t.Fatalf("tick %d: %s, want done held", i, d.State)
		}
	}
	tr.Seen("m", "p1")
	s := snap(herdr.StatusIdle, false)
	tr.Observe("m", s, now.Add(20*time.Second))
	if d := display(tr, s); d.State != herdr.StatusIdle || d.LastActive != now.Add(time.Second).UnixMilli() {
		t.Fatalf("after seen: %+v", d)
	}
}

func TestHerdrFocusCountsAsSeen(t *testing.T) {
	tr := New(t.TempDir(), nil)
	now := time.Now()
	tr.Observe("m", snap(herdr.StatusWorking, false), now)
	tr.Observe("m", snap(herdr.StatusIdle, false), now.Add(5*time.Second))
	s := snap(herdr.StatusIdle, true)
	tr.Observe("m", s, now.Add(6*time.Second))
	if d := display(tr, s); d.State != herdr.StatusIdle {
		t.Fatalf("focused pane still %s", d.State)
	}
}

func TestIdleBlinkWithinGraceIsStillWorking(t *testing.T) {
	tr := New(t.TempDir(), nil)
	now := time.Now()
	tr.Observe("m", snap(herdr.StatusWorking, false), now)
	s := snap(herdr.StatusIdle, false)
	tr.Observe("m", s, now.Add(time.Second))
	if d := display(tr, s); d.State != herdr.StatusWorking {
		t.Fatalf("one second after working: %s, want working (grace)", d.State)
	}
	tr.Observe("m", s, now.Add(4*time.Second))
	if d := display(tr, s); d.State != herdr.StatusDone {
		t.Fatalf("after the grace: %s, want done", d.State)
	}
}

func TestQuestionIsHeldUntilItWorksAgain(t *testing.T) {
	tr := New(t.TempDir(), nil)
	now := time.Now()
	tr.Observe("m", snap(herdr.StatusWorking, false), now)
	tr.Observe("m", snap(herdr.StatusBlocked, false), now.Add(5*time.Second))
	// herdr may read the dialog as idle once the menu scrolls away
	s := snap(herdr.StatusIdle, false)
	tr.Observe("m", s, now.Add(10*time.Second))
	if d := display(tr, s); d.State != herdr.StatusBlocked {
		t.Fatalf("question let go: %s", d.State)
	}
	w := snap(herdr.StatusWorking, false)
	tr.Observe("m", w, now.Add(15*time.Second))
	if d := display(tr, w); d.State != herdr.StatusWorking {
		t.Fatalf("working again: %s", d.State)
	}
}

func TestQuestionWithNoWorkBeforeItIsNotHeld(t *testing.T) {
	tr := New(t.TempDir(), nil)
	now := time.Now()
	tr.Observe("m", snap(herdr.StatusBlocked, false), now)
	s := snap(herdr.StatusIdle, false)
	tr.Observe("m", s, now.Add(5*time.Second))
	if d := display(tr, s); d.State != herdr.StatusIdle {
		t.Fatalf("a dialog that was simply closed: %s, want idle", d.State)
	}
}

func TestRecoveryAndPersistence(t *testing.T) {
	dir := t.TempDir()
	stamp := time.Now().Add(-3 * time.Hour).Truncate(time.Millisecond)
	tr := New(dir, func(string, *herdr.PaneInfo) time.Time { return stamp })
	s := snap(herdr.StatusIdle, false)
	tr.Observe("m", s, time.Now())
	deadline := time.Now().Add(2 * time.Second)
	for display(tr, s).LastActive == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if d := display(tr, s); d.LastActive != stamp.UnixMilli() {
		t.Fatalf("recovered %d, want %d", d.LastActive, stamp.UnixMilli())
	}
	tr.Flush()
	// a restart keeps the stamp and does not ask again
	tr2 := New(dir, func(string, *herdr.PaneInfo) time.Time { t.Fatal("recovery asked twice"); return time.Time{} })
	tr2.Observe("m", s, time.Now())
	if d := display(tr2, s); d.LastActive != stamp.UnixMilli() {
		t.Fatalf("after restart: %+v", d)
	}
}

func TestGonePanesAreForgotten(t *testing.T) {
	tr := New(t.TempDir(), nil)
	tr.Observe("m", snap(herdr.StatusWorking, false), time.Now())
	tr.Observe("m", &herdr.Snapshot{}, time.Now())
	if _, ok := tr.panes["m|p1"]; ok {
		t.Fatal("closed pane kept")
	}
}
