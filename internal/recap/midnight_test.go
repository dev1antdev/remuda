package recap

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/transcript"
)

func TestSpansAcrossALocalMidnight(t *testing.T) {
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Skip("no zone database")
	}
	// 23:50 to 00:20 in Kyiv, a question asked at 00:10 and still open
	turns := []transcript.Turn{
		{Role: "user", Ts: "2026-10-24T20:50:00Z", IDs: []string{"u1"}, Parts: []transcript.Part{text("one last thing")}},
		{Role: "assistant", Ts: "2026-10-24T20:50:02Z", IDs: []string{"a1"}, Took: 1200, Parts: []transcript.Part{
			{Type: "tool", ToolName: "AskUserQuestion", At: "2026-10-24T21:10:00Z"},
		}},
	}
	midnight := time.Date(2026, 10, 25, 0, 0, 0, 0, kyiv)
	now := midnight.Add(time.Hour)
	got := words(inUTC(Spans(turns, midnight, now)))
	if got != "working 21:00-21:10 u1 | waiting 21:10-22:00 u1" {
		t.Errorf("since midnight: %s", got)
	}
	if all := words(inUTC(Spans(turns, time.Time{}, now))); all != "working 20:50-21:10 u1 | waiting 21:10-22:00 u1" {
		t.Errorf("the whole of it: %s", all)
	}
}

// inUTC puts every span's edges in one zone: a clipped edge keeps the zone of the period it was clipped to.
func inUTC(spans []Span) []Span {
	for i := range spans {
		spans[i].From, spans[i].To = spans[i].From.UTC(), spans[i].To.UTC()
	}
	return spans
}

func TestOfAConversationWithNoPrompt(t *testing.T) {
	turns := []transcript.Turn{
		{Role: "assistant", Ts: "2026-10-24T09:00:00Z", Took: 0, Tokens: 500, Parts: []transcript.Part{
			tool("Read", `{"file_path":"/a.go"}`, 2, "x", false), text("Carrying on after the compaction."),
		}},
		{Role: "assistant", Ts: "", Parts: []transcript.Part{text("A turn with no time counts.")}},
	}
	r := Of(turns, time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC))
	if r.Prompts != 0 || r.Answers != 2 || r.Calls != 1 || r.Tokens != 500 || r.First != "2026-10-24T09:00:00Z" || r.Last != "2026-10-24T09:00:00Z" {
		t.Fatalf("recap %+v", r)
	}
	if r.Said != "A turn with no time counts." || r.Quiet() || r.Asking {
		t.Fatalf("said %q quiet %v asking %v", r.Said, r.Quiet(), r.Asking)
	}
	if r := Of(turns, time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)); r.Answers != 1 || r.Calls != 0 {
		t.Fatalf("a later since keeps only the turn with no time: %+v", r)
	}
}
