package chat

import (
	"strings"
	"testing"
)

// the footer lines as Claude Code 2.1.312 draws them, read off a pane while shift+tab cycled the mode
const idleScreen = `● Done.

────────────────────────────────────────────────────────────────────────────
❯ Try "refactor <filepath>"
────────────────────────────────────────────────────────────────────────────
  FOOTER
Model: Fable 5.1 | Context: [░░░░░░░░░░░░░░░░] 0/1.0M (0%) | Session: 29.0%
`

func TestParseModeReadsTheFooter(t *testing.T) {
	cases := map[string]string{
		"⏵⏵ accept edits on (shift+tab to cycle)":             "acceptEdits",
		"⏸ plan mode on (shift+tab to cycle)":                 "plan",
		"⏵⏵ auto mode on (shift+tab to cycle)":                "auto",
		"⏸ manual mode on":                                    "default",
		"⏵⏵ bypass permissions on (shift+tab to cycle)":       "bypass",
		"⏵⏵ accept edits on (shift+tab to":                    "acceptEdits", // a narrow pane cuts the tail
		"⏵⏵ auto mode on (shift+tab to cycle) · ← for agents": "auto",        // the footer as herdr renders it, a hint after
	}
	for footer, want := range cases {
		screen := strings.Replace(idleScreen, "FOOTER", footer, 1)
		got, ok := ParseMode(screen)
		if !ok || got != want {
			t.Errorf("%q: got %q, %v; want %q", footer, got, ok, want)
		}
	}
}

func TestParseModeReadsNothingWithoutAFooter(t *testing.T) {
	if mode, ok := ParseMode(bashPrompt); ok {
		t.Fatalf("a dialog covers the footer, yet read %q", mode)
	}
	quoted := "● The footer said ⏵⏵ plan mode on yesterday, and I kept working.\n\n❯ \n"
	if mode, ok := ParseMode(quoted); ok {
		t.Fatalf("a quote in the conversation read as %q", mode)
	}
	far := strings.Repeat("row\n", 20) + "⏸ plan mode on (shift+tab to cycle)\n" + strings.Repeat("row\n", 20)
	if mode, ok := ParseMode(far); ok {
		t.Fatalf("a line far above the bottom read as %q", mode)
	}
	if _, ok := ParseMode("$ \n"); ok {
		t.Fatal("a shell has no mode")
	}
}
