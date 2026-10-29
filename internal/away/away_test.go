package away

import (
	"testing"
	"time"
)

type recorder struct {
	notices []Notice
}

func (r *recorder) take(n Notice) { r.notices = append(r.notices, n) }

func newTracker(rec *recorder, start time.Time) (*Tracker, *time.Time) {
	now := start
	t := New(2*time.Minute, rec.take)
	t.Now = func() time.Time { return now }
	return t, &now
}

func TestAnAwayIsReportedAfterTheThresholdAndABackAfterIt(t *testing.T) {
	rec := &recorder{}
	start := time.Date(2026, 10, 29, 14, 0, 0, 0, time.UTC)
	tr, now := newTracker(rec, start)
	tr.Observe("gog", "GOG", "connected", "")
	*now = start.Add(time.Minute)
	tr.Observe("gog", "GOG", "error", "ssh: connection refused")
	tr.Sweep(start.Add(2 * time.Minute)) // a minute down: nothing yet
	if len(rec.notices) != 0 {
		t.Fatalf("reported too early: %+v", rec.notices)
	}
	tr.Observe("gog", "GOG", "error", "connection lost: timeout") // still down, the reason moved on
	tr.Sweep(start.Add(3*time.Minute + time.Second))
	if len(rec.notices) != 1 || !rec.notices[0].Away || rec.notices[0].Reason != "connection lost: timeout" || rec.notices[0].Label != "GOG" {
		t.Fatalf("away: %+v", rec.notices)
	}
	tr.Sweep(start.Add(10 * time.Minute)) // reported once
	if len(rec.notices) != 1 {
		t.Fatalf("reported twice: %+v", rec.notices)
	}
	*now = start.Add(13 * time.Minute)
	tr.Observe("gog", "GOG", "connected", "")
	if len(rec.notices) != 2 || rec.notices[1].Away || rec.notices[1].For != 12*time.Minute {
		t.Fatalf("back: %+v", rec.notices)
	}
}

func TestABlipAndTheFirstStateSayNothing(t *testing.T) {
	rec := &recorder{}
	start := time.Date(2026, 10, 29, 14, 0, 0, 0, time.UTC)
	tr, now := newTracker(rec, start)
	tr.Observe("roofr", "Roofr", "error", "ssh: no route") // down at remuda's own start: seeds only
	tr.Sweep(start.Add(time.Hour))
	*now = start.Add(time.Hour)
	tr.Observe("roofr", "Roofr", "connected", "") // up after an unreported down: no back either
	*now = start.Add(2 * time.Hour)
	tr.Observe("roofr", "Roofr", "error", "connection lost")
	*now = start.Add(2*time.Hour + 30*time.Second)
	tr.Observe("roofr", "Roofr", "connected", "") // a blip: back within the threshold
	tr.Sweep(start.Add(3 * time.Hour))
	if len(rec.notices) != 0 {
		t.Fatalf("something was said: %+v", rec.notices)
	}
}

func TestAForgottenMachineIsNotReported(t *testing.T) {
	rec := &recorder{}
	start := time.Date(2026, 10, 29, 14, 0, 0, 0, time.UTC)
	tr, now := newTracker(rec, start)
	tr.Observe("box", "Box", "connected", "")
	*now = start.Add(time.Minute)
	tr.Observe("box", "Box", "error", "gone")
	tr.Forget("box")
	tr.Sweep(start.Add(time.Hour))
	if len(rec.notices) != 0 {
		t.Fatalf("a removed machine was reported: %+v", rec.notices)
	}
}
