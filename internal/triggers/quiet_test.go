package triggers

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestAMutedFolderAddsNoActivityRowButItsTaskMoves(t *testing.T) {
	h := &heard{}
	s := h.service(t)
	var muted atomic.Bool
	muted.Store(true)
	s.Muted = func(machine, pane string) bool { return muted.Load() }
	if _, err := s.Replace([]Rule{
		{ID: "f", Name: "Migrated", Match: "substring", Pattern: "migrated", Action: ActionFeed, Enabled: true},
		{ID: "t", Name: "Ready for QA", Match: "substring", Pattern: "ready for qa", Action: ActionTask, Column: "Testing", Enabled: true},
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	s.Observe(snapshot(true, now, shell("w1:p1")))
	s.OnMatch(match("w1:p1", "rule:f", "12 tables migrated", false, false, now))
	s.OnMatch(match("w1:p1", "rule:t", "ready for qa", false, false, now))
	eventually(t, "the move", func() bool { _, _, m := h.counts(); return m == 1 })
	time.Sleep(30 * time.Millisecond)
	if _, f, _ := h.counts(); f != 0 {
		t.Fatalf("a muted folder's panes add no row to the activity: %d", f)
	}
	muted.Store(false)
	s.OnMatch(match("w1:p1", "rule:f", "3 tables migrated", false, false, now.Add(2*time.Minute)))
	eventually(t, "the row once the mute is over", func() bool { _, f, _ := h.counts(); return f == 1 })
}

func TestADevServerSayingItAgainIsTheOfferThatStands(t *testing.T) {
	h := &heard{listens: 1}
	var changed atomic.Int32
	s := h.service(t)
	s.Changed = func() { changed.Add(1) }
	now := time.Now()
	s.Observe(snapshot(true, now, shell("w1:p1")))
	s.OnMatch(match("w1:p1", WatchServe, "Server listening on port 3000", false, false, now))
	eventually(t, "the offer", func() bool { return changed.Load() == 1 })
	first := s.OfferOf("gog", "w1:p1")
	for i := 1; i <= 3; i++ { // nodemon restarting the server on every save
		s.OnMatch(match("w1:p1", WatchServe, "Server listening on port 3000", false, false, now.Add(time.Duration(i)*10*time.Second)))
	}
	time.Sleep(50 * time.Millisecond)
	if again := s.OfferOf("gog", "w1:p1"); changed.Load() != 1 || !again.At.Equal(first.At) || len(h.ports()) != 1 {
		t.Fatalf("one offer while it stands, the ports looked at once: %d changes, %v then %v, asked %v", changed.Load(), first.At, again.At, h.ports())
	}
	s.OnMatch(match("w1:p1", WatchServe, "Server listening on port 3001", false, false, now.Add(time.Minute)))
	eventually(t, "a server on another port is offered", func() bool { return changed.Load() == 2 })
	if o := s.OfferOf("gog", "w1:p1"); o.Port != 3001 {
		t.Fatalf("the new offer: %+v", o)
	}
}
