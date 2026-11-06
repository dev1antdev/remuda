package recap

import (
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/transcript"
)

func at(s string) time.Time {
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return v
}

// day is a morning's conversation: a long gap, a turn that straddles since, a question answered after ten minutes.
func day() []transcript.Turn {
	return []transcript.Turn{
		{Role: "user", Ts: "2026-10-24T07:50:00Z", IDs: []string{"u1"}, Parts: []transcript.Part{text("early prompt")}},
		{Role: "assistant", Ts: "2026-10-24T07:50:05Z", IDs: []string{"a1"}, Took: 1200, Parts: []transcript.Part{text("done at 08:10")}},
		{Role: "user", Ts: "2026-10-24T10:00:00Z", IDs: []string{"u2"}, Parts: []transcript.Part{text("fix the tests")}},
		{Role: "assistant", Ts: "2026-10-24T10:00:03Z", IDs: []string{"a2"}, Took: 1800, Parts: []transcript.Part{
			tool("Bash", `{"command":"go test ./..."}`, 60, "FAIL", true),
			{Type: "tool", ToolName: "AskUserQuestion", At: "2026-10-24T10:10:00Z", Took: 600, Output: "picked A"},
			text("Fixed."),
		}},
		{Role: "user", Ts: "2026-10-24T10:31:00Z", IDs: []string{"u3"}, Parts: []transcript.Part{text("thanks, one more")}},
		{Role: "assistant", Ts: "2026-10-24T10:31:02Z", IDs: []string{"a3"}, Took: 300, Parts: []transcript.Part{
			{Type: "tool", ToolName: "AskUserQuestion", At: "2026-10-24T10:36:00Z", Output: ""},
		}},
	}
}

func words(spans []Span) string {
	var out []string
	for _, s := range spans {
		line := s.State + " " + s.From.Format("15:04") + "-" + s.To.Format("15:04")
		if s.Turn != "" {
			line += " " + s.Turn
		}
		out = append(out, line)
	}
	return strings.Join(out, " | ")
}

func TestSpansLayTheDayOut(t *testing.T) {
	since, now := at("2026-10-24T08:00:00Z"), at("2026-10-24T11:00:00Z")
	got := words(Spans(day(), since, now))
	want := strings.Join([]string{
		"working 08:00-08:10 u1", // a turn straddling since is clipped to it
		"idle 08:10-10:00",       // a gap over two minutes reads as idle
		"working 10:00-10:10 u2", // the question splits the turn's work
		"waiting 10:10-10:20 u2", // for as long as the answer took
		"working 10:20-10:30 u2",
		"working 10:31-10:36 u3", // the gap under two minutes stays unfilled
		"waiting 10:36-11:00 u3", // a question still open waits until now
	}, " | ")
	if got != want {
		t.Errorf("spans:\n got %s\nwant %s", got, want)
	}
}

func TestSpansOverEverythingAndNothing(t *testing.T) {
	all := Spans(day(), time.Time{}, at("2026-10-24T11:00:00Z"))
	if len(all) == 0 || all[0].State != "working" || all[0].From != at("2026-10-24T07:50:00Z") || all[len(all)-1].State != "waiting" {
		t.Errorf("unclipped: %s", words(all))
	}
	if open := Spans(day(), time.Time{}, time.Time{}); open[len(open)-1].State != "working" {
		t.Errorf("without a now an open question has no end to wait for: %s", words(open))
	}
	if idle := Spans(nil, at("2026-10-24T08:00:00Z"), at("2026-10-24T09:00:00Z")); words(idle) != "idle 08:00-09:00" {
		t.Errorf("an empty period is one idle span: %s", words(idle))
	}
	if none := Spans(nil, time.Time{}, time.Time{}); none == nil || len(none) != 0 {
		t.Errorf("no turns and no period: %v", none)
	}
	later := Spans(day(), at("2026-10-24T12:00:00Z"), at("2026-10-24T13:00:00Z"))
	if words(later) != "waiting 12:00-13:00 u3" {
		t.Errorf("a period after the conversation still waits on the open question: %s", words(later))
	}
}

func TestSpansWithoutAPromptUseTheCalls(t *testing.T) {
	turns := []transcript.Turn{
		{Role: "assistant", Ts: "2026-10-24T09:00:00Z", IDs: []string{"a1"}, Parts: []transcript.Part{
			{Type: "tool", ToolName: "Read", At: "2026-10-24T09:00:10Z", Took: 50},
			text("looked"),
		}},
		{Role: "assistant", Ts: "2026-10-24T09:30:00Z", Parts: []transcript.Part{text("nothing timed")}},
	}
	got := words(Spans(turns, time.Time{}, time.Time{}))
	if got != "working 09:00-09:01 a1" {
		t.Errorf("a turn after a compaction spans its own calls, a turn with no end is skipped: %s", got)
	}
}
