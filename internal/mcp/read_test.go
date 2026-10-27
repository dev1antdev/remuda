package mcp

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/tasksapi"
)

func TestAgentStateWords(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	ms := func(ago time.Duration) int64 { return now.Add(-ago).UnixMilli() }
	cases := []struct {
		radar *tasksapi.Radar
		want  string
	}{
		{nil, "unknown"},
		{&tasksapi.Radar{State: "working", Since: ms(12 * time.Minute)}, "working 12 min"},
		{&tasksapi.Radar{State: "working", Since: ms(20 * time.Second)}, "working just now"},
		{&tasksapi.Radar{State: "working"}, "working"},
		{&tasksapi.Radar{State: "blocked", Since: ms(90 * time.Minute)}, "waiting for the person 1 h 30 min"},
		{&tasksapi.Radar{State: "done", Since: ms(2 * time.Hour)}, "finished 2 h ago"},
		{&tasksapi.Radar{State: "done", Since: ms(5 * time.Second)}, "finished just now"},
		{&tasksapi.Radar{State: "done"}, "finished"},
		{&tasksapi.Radar{State: "idle", LastActive: ms(3 * 24 * time.Hour)}, "idle, last worked 3 d ago"},
		{&tasksapi.Radar{State: "idle", LastActive: ms(10 * time.Second)}, "idle, last worked just now"},
		{&tasksapi.Radar{State: "idle"}, "idle"},
		{&tasksapi.Radar{State: "working", Since: ms(time.Minute), Alert: "stuck for 20 min"}, "needs the person: stuck for 20 min"},
		{&tasksapi.Radar{State: "unknown"}, "unknown"},
	}
	for _, c := range cases {
		if got := agentState(c.radar, now); got != c.want {
			t.Errorf("agentState(%+v) = %q, want %q", c.radar, got, c.want)
		}
	}
}

func TestSinceOfReadsThePeriod(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 30, 0, 0, time.Local)
	midnight := time.Date(2026, 10, 27, 0, 0, 0, 0, time.Local)
	for word, want := range map[string]time.Time{"": {}, "midnight": midnight, "Today": midnight, "24h": now.Add(-24 * time.Hour), "7d": now.Add(-7 * 24 * time.Hour), "2026-10-20": time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local), "2026-10-20T08:00:00Z": time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)} {
		got, err := sinceOf(word, now)
		if err != nil || !got.Equal(want) {
			t.Errorf("sinceOf(%q) = %v, %v; want %v", word, got, err, want)
		}
	}
	if _, err := sinceOf("yesterday-ish", now); err == nil {
		t.Error("a word the tool does not know passed")
	}
}

func TestOneLineCutsAtTheFirstLineAndN(t *testing.T) {
	if got := oneLine("  The tests pass now.\nMore below.", 80); got != "The tests pass now." {
		t.Errorf("first line: %q", got)
	}
	if got := oneLine("Закоммитил, запушил и создал МР: https://example.com/x", 20); got != "Закоммитил, запушил…" {
		t.Errorf("cut at a rune count: %q", got)
	}
	if got := oneLine("", 80); got != "" {
		t.Errorf("empty: %q", got)
	}
}
