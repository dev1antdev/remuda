package handover

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/recap"
	"github.com/dev1antdev/remuda/internal/transcript"
)

func tool(name, input string, failed bool) transcript.Part {
	return transcript.Part{Type: "tool", ToolName: name, Input: json.RawMessage(input), Took: 1, Output: "out", IsError: failed}
}

func text(s string) transcript.Part { return transcript.Part{Type: "text", Text: s} }

func sample() []transcript.Turn {
	return []transcript.Turn{
		{Role: "user", Ts: "2026-10-24T08:00:00Z", Parts: []transcript.Part{text("look at the build")}},
		{Role: "assistant", Ts: "2026-10-24T08:00:05Z", Took: 5, Tokens: 100, Cost: 0.01, Parts: []transcript.Part{tool("Read", `{"file_path":"/old.go"}`, false), text("The build is fine.")}},
		{Role: "user", Ts: "2026-10-24T10:00:00Z", Parts: []transcript.Part{text("fix the tests")}},
		{Role: "assistant", Ts: "2026-10-24T10:00:03Z", Took: 92, Tokens: 38000, Cost: 0.42, Parts: []transcript.Part{
			text("Let me run them."),
			tool("Bash", `{"command":"go test ./...","description":"Run the tests"}`, true),
			tool("Edit", `{"file_path":"/a/b.go","old_string":"x","new_string":"y"}`, false),
			text("## Fixed\n\nThe tests pass now."),
		}},
		{Role: "user", Ts: "2026-10-24T10:05:00Z", Parts: []transcript.Part{text("now the docs")}},
		{Role: "assistant", Ts: "2026-10-24T10:05:03Z", Took: 10, Tokens: 2000, Cost: 0.02, Parts: []transcript.Part{tool("Edit", `{"file_path":"/README.md","old_string":"a","new_string":"b"}`, false), text("Done.")}},
	}
}

func TestTextCarriesTheRecapAndTheLastExchanges(t *testing.T) {
	turns := sample()
	got := Text("Claude Code", recap.Of(turns, time.Time{}), turns, 2)
	for _, want := range []string{
		"# Handover from Claude Code",
		"Claude Code worked on it with me in this workspace until now",
		"## What was done so far",
		"- 3 prompts · 3 answers · 1 min 47 s · 40k tokens · $0.45",
		"- Edited: `/a/b.go`, `/README.md`",
		"- Last answer: Done.",
		"## The last 2 exchanges in full",
		"### Me\n\nfix the tests\n",
		"### Claude Code\n\nLet me run them.\n- Bash: go test ./... (failed)\n- Edit: /a/b.go\n## Fixed\n\nThe tests pass now.\n",
		"### Me\n\nnow the docs\n",
		"- Edit: /README.md\nDone.\n",
		"\n---\n\nCarry on from here. Ask me if anything is unclear.\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("text lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "look at the build") || strings.Contains(got, "The build is fine.") {
		t.Errorf("the first exchange is not among the last two:\n%s", got)
	}
	if strings.Contains(got, `"old_string"`) || strings.Contains(got, "\nout\n") {
		t.Errorf("a call's input or output leaked in whole:\n%s", got)
	}
}

func TestTextWithoutExchanges(t *testing.T) {
	turns := sample()
	got := Text("Codex", recap.Of(turns, time.Time{}), turns, 0)
	if strings.Contains(got, "## The last") || strings.Contains(got, "exchanges in full") || strings.Contains(got, "### Me") {
		t.Errorf("zero exchanges asked, some given:\n%s", got)
	}
	if !strings.Contains(got, "## What was done so far") || !strings.Contains(got, "Carry on from here") {
		t.Errorf("the recap and the tail stand:\n%s", got)
	}
	if one := Text("Codex", recap.Of(turns, time.Time{}), turns, 1); !strings.Contains(one, "## The last 1 exchange in full") {
		t.Errorf("one exchange:\n%s", one)
	}
	if many := Text("Codex", recap.Of(turns, time.Time{}), turns, 99); !strings.Contains(many, "## The last 3 exchanges in full") {
		t.Errorf("more than there are:\n%s", many)
	}
}

func TestTextStaysUnderTheCap(t *testing.T) {
	big := strings.Repeat("word ", 7000) // 35 KB an answer
	var turns []transcript.Turn
	for i := 0; i < 4; i++ {
		turns = append(turns, transcript.Turn{Role: "user", Parts: []transcript.Part{text("prompt " + string(rune('A'+i)))}})
		turns = append(turns, transcript.Turn{Role: "assistant", Parts: []transcript.Part{text(big + "answer " + string(rune('A'+i)))}})
	}
	got := Text("Claude Code", recap.Of(turns, time.Time{}), turns, 3)
	if len(got) > MaxText {
		t.Fatalf("text is %d bytes, over the cap", len(got))
	}
	if !strings.Contains(got, "answer D") || strings.Contains(got, "answer C") {
		t.Errorf("the newest exchange is kept and the older ones dropped: has D %v, has C %v", strings.Contains(got, "answer D"), strings.Contains(got, "answer C"))
	}
	if !strings.Contains(got, "## The last 1 exchange in full") {
		t.Errorf("the heading counts what is left:\n%.300s", got)
	}
	huge := []transcript.Turn{
		{Role: "user", Parts: []transcript.Part{text("go")}},
		{Role: "assistant", Parts: []transcript.Part{text(strings.Repeat("x", 2*MaxText))}},
	}
	summary := recap.Of(huge, time.Time{})
	summary.Edited = []string{strings.Repeat("/very/long/path", 5000)}
	got = Text("Claude Code", summary, huge, 1)
	if len(got) > MaxText || strings.Contains(got, "cut here") || strings.Contains(got, "Edited:") || strings.Contains(got, "### Me") {
		t.Errorf("a text too big with no exchange loses the lists: %d bytes, cut %v, lists %v", len(got), strings.Contains(got, "cut here"), strings.Contains(got, "Edited:"))
	}
	got = Text(strings.Repeat("Я", MaxText), summary, huge, 1)
	if len(got) > MaxText || !strings.Contains(got, "cut here") || strings.ContainsRune(got, '\uFFFD') {
		t.Errorf("a text too big even bare is cut at a rune: %d bytes, cut %v", len(got), strings.Contains(got, "cut here"))
	}
}

func TestExchangesPairPromptsWithAnswers(t *testing.T) {
	turns := []transcript.Turn{
		{Role: "assistant", Parts: []transcript.Part{text("hello before any prompt")}},
		{Role: "user", Parts: []transcript.Part{text("one")}},
		{Role: "assistant", Parts: []transcript.Part{tool("Grep", `{"pattern":"x"}`, false)}},
		{Role: "assistant", Parts: []transcript.Part{text("found")}},
		{Role: "user", Parts: []transcript.Part{{Type: "image"}}},
	}
	got := Exchanges(turns)
	if len(got) != 3 || got[0].Prompt != "" || got[0].Lines[0] != "hello before any prompt" {
		t.Fatalf("exchanges = %+v", got)
	}
	if got[1].Prompt != "one" || strings.Join(got[1].Lines, "|") != "- Grep: x|found" {
		t.Errorf("two assistant turns join one exchange: %+v", got[1])
	}
	if got[2].Prompt != "" || len(got[2].Lines) != 0 {
		t.Errorf("a prompt without text or answer: %+v", got[2])
	}
	if text := Text("X", recap.Recap{}, turns, 1); !strings.Contains(text, "### Me\n\n(no text)\n") || !strings.Contains(text, "### X\n\n(no answer)\n") {
		t.Errorf("empty sides are said so:\n%s", text)
	}
}

func TestCutKeepsRunesWhole(t *testing.T) {
	got := cut(strings.Repeat("ж", 100), 101)
	if !strings.HasSuffix(got, "can carry)\n") || !strings.HasPrefix(got, "ж") {
		t.Errorf("cut = %q", got)
	}
	for _, r := range got {
		if r == '\uFFFD' {
			t.Fatal("a rune was split")
		}
	}
}
