package recap

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/transcript"
)

func TestAMessageSentWhileItWorkedCountsAsAPrompt(t *testing.T) {
	turns := []transcript.Turn{
		{Role: "user", Ts: "2026-11-13T09:00:00Z", Parts: []transcript.Part{text("rename the flag")}},
		{Role: "assistant", Ts: "2026-11-13T09:00:04Z", Took: 30, Parts: []transcript.Part{
			tool("Bash", `{"command":"grep -rn fast"}`, 1, "x", false),
			{Type: "queued", Text: "also the README", At: "2026-11-13T09:00:06Z"},
			text("Renamed it, the README too."),
		}},
	}
	r := Of(turns, time.Time{})
	if r.Prompts != 2 || r.Answers != 1 || r.Said != "Renamed it, the README too." {
		t.Errorf("%+v", r)
	}
	alone := []transcript.Turn{{Role: "user", Parts: []transcript.Part{{Type: "queued", Text: "and this"}}}}
	if got := Of(alone, time.Time{}).Prompts; got != 1 {
		t.Errorf("a turn holding only a message sent mid-turn counts it once: %d", got)
	}
}
