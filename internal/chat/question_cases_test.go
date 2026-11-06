package chat

import (
	"strings"
	"testing"
)

func TestKeysToWalksFromTheCursor(t *testing.T) {
	four := func(picked int) []Option {
		options := make([]Option, 4)
		for i := range options {
			options[i] = Option{N: i + 1, Label: string(rune('A' + i)), Picked: i == picked}
		}
		return options
	}
	cases := []struct {
		picked, index int
		want          string
	}{
		{0, 0, "enter"},
		{0, 3, "down down down enter"},
		{2, 0, "up up enter"},
		{2, 3, "down enter"},
		{3, 3, "enter"},
		{3, 1, "up up enter"},
	}
	for _, c := range cases {
		if got := strings.Join(KeysTo(four(c.picked), c.index), " "); got != c.want {
			t.Errorf("from %d to %d: %q, want %q", c.picked+1, c.index+1, got, c.want)
		}
	}
}

// a framed prompt with the cursor on a choice that wraps, below the first
const wrappedCursor = rule + `
 Bash command

   npm run build

 Do you want to proceed?
   1. Yes
 ❯ 2. Yes, and don't ask again for npm run build commands in
   /Users/me/project
   3. No, and tell Claude what to do differently (esc)

 Esc to cancel · Tab to amend
`

// an AskUserQuestion with several questions: a tab header, the question, choices each described on the next row
const severalQuestions = `● I need two decisions.

 ←  ☐ Storage  ☐ Format  ✔ Submit  →

 Which storage should the feed use?

   1. A JSON file
      Simple, 300 entries, rewritten whole
   2. Memory
      Gone on a restart
 ❯ 3. SQLite
      The tasks database is there already
   4. Type something.

   5. Chat about this

 Enter to select · ↑/↓ to navigate · Tab to switch questions · Esc to cancel
`

func TestParseQuestionCases(t *testing.T) {
	q, ok := ParseQuestion(wrappedCursor)
	if !ok || len(q.Options) != 3 || !q.Options[1].Picked || q.Options[1].Label != "Yes, and don't ask again for npm run build commands in /Users/me/project" {
		t.Fatalf("a wrapped cursor row: %+v %v", q, ok)
	}
	if keys, err := Answer(wrappedCursor, 0, "Yes", dialogOf(wrappedCursor)); err != nil || strings.Join(keys, " ") != "up enter" {
		t.Fatalf("a choice above the cursor: %v %v", keys, err)
	}
	if keys, err := Answer(wrappedCursor, 2, "No, and tell Claude what to do differently (esc)", dialogOf(wrappedCursor)); err != nil || strings.Join(keys, " ") != "down enter" {
		t.Fatalf("a choice below the cursor: %v %v", keys, err)
	}

	q, ok = ParseQuestion(severalQuestions)
	if !ok || q.Text != "Which storage should the feed use?" || len(q.Options) != 4 {
		t.Fatalf("several questions: %+v %v", q, ok)
	}
	if q.Options[1].Label != "Memory Gone on a restart" || !q.Options[2].Picked || q.Options[3].Label != "Type something." {
		t.Fatalf("several questions' choices: %+v", q.Options)
	}
	if keys, err := Answer(severalQuestions, 3, "Type something.", dialogOf(severalQuestions)); err != nil || strings.Join(keys, " ") != "down enter" {
		t.Fatalf("the typed answer: %v %v", keys, err)
	}
	changed := strings.Replace(severalQuestions, "2. Memory", "2. Redis", 1)
	if _, err := Answer(changed, 1, "Memory Gone on a restart", dialogOf(changed)); err == nil || !strings.Contains(err.Error(), "now reads") {
		t.Fatalf("a choice that changed since it was offered: %v", err)
	}
}

func TestParseModeUnderClaudeCodesDialogs(t *testing.T) {
	for name, screen := range map[string]string{"bash": framedBash, "edit": framedEdit, "trust": framedTrust, "question": severalQuestions} {
		if mode, ok := ParseMode(screen); ok {
			t.Errorf("%s: a dialog covers the footer, yet read %q", name, mode)
		}
	}
	narrow := strings.Replace(idleScreen, "FOOTER", "⏸ plan mode on (shi", 1)
	if mode, ok := ParseMode(narrow); !ok || mode != "plan" {
		t.Errorf("a narrow footer: %q %v", mode, ok)
	}
	statusLines := strings.Replace(idleScreen, "FOOTER", "⏵⏵ bypass permissions on (shift+tab to cycle)", 1) + strings.Repeat("status row\n", 6)
	if mode, ok := ParseMode(statusLines); !ok || mode != "bypass" {
		t.Errorf("a footer over a tall status line: %q %v", mode, ok)
	}
}
