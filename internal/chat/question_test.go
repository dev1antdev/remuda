package chat

import (
	"strings"
	"testing"
)

const bashPrompt = `● I'll remove the build folder.

● Bash(rm -rf build)
╭──────────────────────────────────────────────────────────────────────────╮
│ Bash command                                                             │
│                                                                          │
│   rm -rf build                                                           │
│   Remove the build folder                                                │
│                                                                          │
│ Do you want to proceed?                                                  │
│ ❯ 1. Yes                                                                 │
│   2. Yes, and don't ask again for rm commands in                         │
│   /Users/me/project                                                      │
│   3. No, and tell Claude what to do differently (esc)                    │
│                                                                          │
╰──────────────────────────────────────────────────────────────────────────╯
`

const editPrompt = `╭──────────────────────────────────────────────────────────────────────────╮
│ Edit file                                                                │
│ ╭──────────────────────────────────────────────────────────────────────╮ │
│ │ src/app.ts                                                           │ │
│ │                                                                      │ │
│ │   1  const a = 1;                                                    │ │
│ │   2 -const b = 2;                                                    │ │
│ │   2 +const b = 3;                                                    │ │
│ ╰──────────────────────────────────────────────────────────────────────╯ │
│ Do you want to make this edit to app.ts?                                 │
│   1. Yes                                                                 │
│ ❯ 2. Yes, allow all edits during this session (shift+tab)                │
│   3. No, and tell Claude what to do differently (esc)                    │
╰──────────────────────────────────────────────────────────────────────────╯
`

const trustPrompt = `╭────────────────────────────────────────╮
│ Accessing workspace:                   │
│                                        │
│ /Users/me/.herdr/worktrees/run         │
│ repo/task-test-hello                   │
│                                        │
│ Quick safety check: Is this a project  │
│ ❯ No, exit                             │
│   Yes, I trust this folder             │
╰────────────────────────────────────────╯
`

const askUser = `╭──────────────────────────────────────────────────────────────────────────╮
│ Which storage should the feed use?                                       │
│                                                                          │
│ Storage                                                                  │
│   1. A JSON file                                                         │
│      Simple, 300 entries, rewritten whole                                │
│ ❯ 2. SQLite                                                              │
│      The tasks database is there already                                 │
│   3. Type something.                                                     │
│                                                                          │
│ Enter to select · Tab/Arrow keys to navigate · Esc to cancel             │
╰──────────────────────────────────────────────────────────────────────────╯
`

func TestParseQuestionReadsClaudeCodesDialogs(t *testing.T) {
	cases := []struct {
		name   string
		screen string
		text   string
		labels []string
		picked int
	}{
		{"a Bash permission prompt, one choice wrapped", bashPrompt,
			"Bash command\n\nrm -rf build\nRemove the build folder\n\nDo you want to proceed?",
			[]string{"Yes", "Yes, and don't ask again for rm commands in /Users/me/project", "No, and tell Claude what to do differently (esc)"}, 1},
		{"an Edit prompt with its diff, the cursor on the second choice", editPrompt,
			"Edit file\n\nsrc/app.ts\n\n1  const a = 1;\n2 -const b = 2;\n2 +const b = 3;\n\nDo you want to make this edit to app.ts?",
			[]string{"Yes", "Yes, allow all edits during this session (shift+tab)", "No, and tell Claude what to do differently (esc)"}, 2},
		{"the trust prompt, choices without numbers", trustPrompt,
			"Accessing workspace:\n\n/Users/me/.herdr/worktrees/run\nrepo/task-test-hello\n\nQuick safety check: Is this a project",
			[]string{"No, exit", "Yes, I trust this folder"}, 1},
		{"an AskUserQuestion with a line under each choice and a footer", askUser,
			"Which storage should the feed use?\n\nStorage",
			[]string{"A JSON file Simple, 300 entries, rewritten whole", "SQLite The tasks database is there already", "Type something."}, 2},
	}
	for _, c := range cases {
		q, ok := ParseQuestion(c.screen)
		if !ok {
			t.Fatalf("%s: not recognised", c.name)
		}
		if q.Text != c.text {
			t.Errorf("%s: text\n got %q\nwant %q", c.name, q.Text, c.text)
		}
		if len(q.Options) != len(c.labels) {
			t.Fatalf("%s: %d choices, want %d: %+v", c.name, len(q.Options), len(c.labels), q.Options)
		}
		for i, o := range q.Options {
			if o.Label != c.labels[i] || o.N != i+1 || o.Picked != (o.N == c.picked) {
				t.Errorf("%s: choice %d = %+v, want %q picked=%v", c.name, i+1, o, c.labels[i], i+1 == c.picked)
			}
		}
	}
}

func TestParseQuestionLeavesOtherScreensAlone(t *testing.T) {
	screens := map[string]string{
		"a shell":                              "$ ls\nREADME.md  go.mod\n$ ",
		"an answer with a numbered list":       "● Three steps:\n  1. Build it\n  2. Test it\n  3. Ship it\n\n> ",
		"the empty prompt and its status line": "❯ Try \"fix the failing test\"\n  ⏵⏵ accept edits on (shift+tab to cycle)\n",
		"a cursor on a list without a box":     "Pick one\n❯ Red\n  Green\n",
		"a numbered list that skips":           "│ ❯ 1. Yes │\n│   3. No  │\n",
		"one choice only":                      "│ Really? │\n│ ❯ 1. Yes │\n",
	}
	for name, screen := range screens {
		if q, ok := ParseQuestion(screen); ok {
			t.Errorf("%s was taken for a question: %+v", name, q)
		}
	}
}

func TestParseQuestionTakesTheDialogAtTheBottom(t *testing.T) {
	screen := strings.Replace(bashPrompt, "● I'll remove the build folder.", "│ ❯ 1. An older menu │\n│   2. Gone by now   │\n\n● I'll remove the build folder.", 1)
	q, ok := ParseQuestion(screen)
	if !ok || q.Options[0].Label != "Yes" || !strings.HasPrefix(q.Text, "Bash command") {
		t.Fatalf("the lower dialog was not the one read: %+v", q)
	}
	if q, ok := ParseQuestion(bashPrompt + "❯ \n"); !ok || len(q.Options) != 3 {
		t.Fatalf("a bare prompt under the dialog hid it: %+v", q)
	}
}
