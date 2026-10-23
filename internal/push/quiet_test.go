package push

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
)

func at(hour, minute int) time.Time {
	return time.Date(2026, 10, 23, hour, minute, 0, 0, time.UTC)
}

func TestQuietCovers(t *testing.T) {
	night := Quiet{From: "22:00", To: "07:00"}
	day := Quiet{From: "09:30", To: "12:00"}
	cases := []struct {
		name string
		q    Quiet
		now  time.Time
		want bool
	}{
		{"night: before", night, at(21, 59), false},
		{"night: at the start", night, at(22, 0), true},
		{"night: past midnight", night, at(3, 15), true},
		{"night: just before the end", night, at(6, 59), true},
		{"night: at the end", night, at(7, 0), false},
		{"night: noon", night, at(12, 0), false},
		{"day: inside", day, at(10, 0), true},
		{"day: before", day, at(9, 29), false},
		{"day: at the end", day, at(12, 0), false},
		{"equal times: no window", Quiet{From: "08:00", To: "08:00"}, at(8, 0), false},
		{"unparsable: no window", Quiet{From: "late", To: "early"}, at(8, 0), false},
	}
	for _, c := range cases {
		if got := c.q.Covers(c.now); got != c.want {
			t.Errorf("%s: Covers(%s) = %v, want %v", c.name, c.now.Format("15:04"), got, c.want)
		}
	}
}

func TestQuietFollowsTheDeviceZone(t *testing.T) {
	kyiv := Quiet{From: "22:00", To: "07:00", TZ: "Europe/Kyiv"}
	// 20:30 UTC is 23:30 in Kyiv (UTC+3 in October): inside the window there, outside in UTC
	if !kyiv.Covers(at(20, 30)) {
		t.Error("20:30 UTC is 23:30 in Kyiv, inside the window")
	}
	if kyiv.Covers(at(5, 0)) {
		t.Error("05:00 UTC is 08:00 in Kyiv, past the window")
	}
	unknown := Quiet{From: "22:00", To: "07:00", TZ: "Mars/Olympus"}
	if !unknown.Covers(at(23, 0)) || unknown.Covers(at(12, 0)) {
		t.Error("an unknown zone reads as UTC")
	}
}

func TestNormalizeDropsABadWindow(t *testing.T) {
	cases := map[string]Quiet{
		"bad time":     {From: "22", To: "07:00"},
		"unknown zone": {From: "22:00", To: "07:00", TZ: "Mars/Olympus"},
	}
	for name, q := range cases {
		p := Prefs{Quiet: &q}
		p.Normalize()
		if p.Quiet != nil {
			t.Errorf("%s: a window that does not parse must be dropped, kept %+v", name, *p.Quiet)
		}
	}
	keep := Prefs{Quiet: &Quiet{From: "22:00", To: "07:00", TZ: "Europe/Kyiv"}}
	keep.Normalize()
	if keep.Quiet == nil {
		t.Error("a good window must survive Normalize")
	}
	utc := Prefs{Quiet: &Quiet{From: "22:00", To: "07:00"}}
	utc.Normalize()
	if utc.Quiet == nil {
		t.Error("an empty zone is UTC, not an error")
	}
}

// TestQuietHoursSkipTheSend: a device inside its window hears nothing, the feed still does, and the window only
// mutes that device.
func TestQuietHoursSkipTheSend(t *testing.T) {
	shrinkDelays(t)
	s, sent := newTestService(t)
	now := time.Now().UTC()
	allDay := &Quiet{From: now.Add(-time.Hour).Format(clock), To: now.Add(time.Hour).Format(clock), TZ: "UTC"}
	sub := Subscription{Endpoint: "https://push.example/dev", Prefs: Prefs{Input: true, Done: "always", Quiet: allDay}}
	sub.Keys.P256dh, sub.Keys.Auth = "p", "a"
	s.Subscribe(sub, "")
	awake := Subscription{Endpoint: "https://push.example/awake", Prefs: Prefs{Input: true, Done: "off"}}
	awake.Keys.P256dh, awake.Keys.Auth = "p", "a"
	s.Subscribe(awake, "")
	heard := &sentLog{}
	s.OnAlert = func(p Payload) {
		heard.mu.Lock()
		heard.list = append(heard.list, p)
		heard.mu.Unlock()
	}

	s.Notify(Payload{Kind: "stuck", Title: "api is stuck"})
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "p1"}, Workspace: "api", From: "working", To: "blocked"})
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "p2"}, Workspace: "web", From: "working", To: "done", Worked: time.Millisecond})
	time.Sleep(80 * time.Millisecond)

	got := sent.get()
	if len(got) != 2 {
		t.Fatalf("the awake device hears the note and the question, the quiet one nothing: got %d sends %+v", len(got), got)
	}
	for _, p := range got {
		if p.Kind == "done" {
			t.Fatalf("only the quiet device wanted finishes; it must not have been sent one: %+v", got)
		}
	}
	if n := len(heard.get()); n != 3 {
		t.Errorf("the feed hears every alert whoever sleeps, heard %d", n)
	}
}
