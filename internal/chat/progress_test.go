package chat

import (
	"strings"
	"testing"
)

// a working Claude Code pane as 2.1.323 draws it: the answer so far, the spinner, the plan's items, the input box, the footer
func workingScreen(spinner string) string {
	return strings.Join([]string{
		"⏺ I'll fold the three checkout steps into one page.",
		"",
		"⏺ Bash(npm test -- checkout)",
		"  ⎿  Running…",
		"",
		spinner,
		"  ⎿  ☒ Read the checkout steps",
		"     ☐ Fold them into one page",
		"",
		rule,
		"❯ ",
		rule,
		"  ⏵⏵ accept edits on (shift+tab to cycle)",
	}, "\n")
}

func TestTheSpinnerSaysHowFarTheTurnIs(t *testing.T) {
	cases := map[string]Progress{
		"✻ Refactoring… (2m 14s · ↓ 6.8k tokens · thinking)":                          {Verb: "Refactoring", Seconds: 134, Tokens: 6800},
		"✶ Cogitating… (45s · ↑ 840 tokens)":                                          {Verb: "Cogitating", Seconds: 45, Tokens: 840},
		"✳ Running the checkout tests… (1h 0m 5s · ↓ 1.2m tokens · ran tool for 12s)": {Verb: "Running the checkout tests", Seconds: 3605, Tokens: 1_200_000},
		"· Thinking…":                                            {Verb: "Thinking"},
		"✽ Compacting conversation… (18s)":                       {Verb: "Compacting conversation", Seconds: 18},
		"* Brewing… (2m 5s · ↓ 11.4k tokens · esc to interrupt)": {Verb: "Brewing", Seconds: 125, Tokens: 11400},
		"✢ Drawing… (8 min · esc to interrupt)":                  {Verb: "Drawing"},
		"✻ Pondering… (1d 2h 3m · ↓ 12.0k tokens)":               {Verb: "Pondering", Seconds: 93780, Tokens: 12000},
	}
	for spinner, want := range cases {
		got, ok := ParseProgress(workingScreen(spinner))
		if !ok || got != want {
			t.Errorf("%q: got %+v %v, want %+v", spinner, got, ok, want)
		}
	}
}

func TestNoSpinnerIsNoProgress(t *testing.T) {
	cases := map[string]string{
		"a finished turn":                "✻ Worked for 2m 14s",
		"the answer's own line":          "⏺ Reading the files…",
		"a list item in the answer":      "  - step one…",
		"words in brackets that run off": "✻ Refactoring… later (maybe)",
	}
	for name, row := range cases {
		if p, ok := ParseProgress(workingScreen(row)); ok {
			t.Errorf("%s read as %+v", name, p)
		}
	}
	if p, ok := ParseProgress("✻ Refactoring… (2m 14s · ↓ 6.8k tokens)\n❯ 1. Yes\n  2. No\n"); ok {
		t.Errorf("a screen with no input box read as %+v", p)
	}
	far := "✻ Refactoring… (2m 14s)\n" + strings.Repeat("text\n", progressRows+1) + rule + "\n❯ \n" + rule + "\n"
	if p, ok := ParseProgress(far); ok {
		t.Errorf("a spinner-like row far above the box read as %+v", p)
	}
}

func TestTheSpinnerAboveARoundedBox(t *testing.T) {
	screen := "✻ Refactoring… (14s · ↓ 2k tokens)\n\n╭────────────────────────╮\n│ > fix it               │\n╰────────────────────────╯\n  ? for shortcuts\n"
	got, ok := ParseProgress(screen)
	if !ok || got != (Progress{Verb: "Refactoring", Seconds: 14, Tokens: 2000}) {
		t.Errorf("got %+v %v", got, ok)
	}
}
