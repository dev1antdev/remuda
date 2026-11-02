package mcp

import (
	"testing"
	"time"
)

// TestDueDateIsTheLastSecondOfThatDayWhateverTheClockDid: a date means the end of that day where the person is, on
// the day the clock springs forward or falls back as on any other — 24 hours from midnight is the next day's first hour.
func TestDueDateIsTheLastSecondOfThatDayWhateverTheClockDid(t *testing.T) {
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Skip("no zone database")
	}
	for _, c := range []struct{ day, want string }{
		{"2026-11-01", "2026-11-01T21:59:59Z"}, // an ordinary day, UTC+2
		{"2026-03-29", "2026-03-29T20:59:59Z"}, // the clock springs forward at 03:00: 23 hours long, UTC+3 by its end
		{"2026-10-25", "2026-10-25T21:59:59Z"}, // the clock falls back at 04:00: 25 hours long, UTC+2 by its end
	} {
		got, err := dueIn(c.day, kyiv)
		if err != nil || got != c.want {
			t.Errorf("dueIn(%s) = %q %v, want %q", c.day, got, err, c.want)
		}
		if local, _ := time.Parse(time.RFC3339, got); local.In(kyiv).Format("2006-01-02 15:04:05") != c.day+" 23:59:59" {
			t.Errorf("dueIn(%s) reads %s there", c.day, local.In(kyiv))
		}
	}
	if got, err := dueIn("2026-03-29T10:00:00+03:00", kyiv); err != nil || got != "2026-03-29T07:00:00Z" {
		t.Errorf("a moment is kept: %q %v", got, err)
	}
	if _, err := dueIn("tomorrow", kyiv); err == nil {
		t.Error("a word is no date")
	}
}
