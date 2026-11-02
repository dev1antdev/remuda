package chat

import (
	"strings"
	"testing"
)

// Claude Code 2.1.316 draws its permission and trust dialogs with a top border alone (a round border with no sides and
// no bottom, its title and content padded one column) and a diff inside one between dashed rules; the input box has a
// rule above and below.
const (
	rule       = "────────────────────────────────────────────────────────────────────────────"
	dashedRule = "╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌"
)

const framedBash = `● I'll remove the build folder.

● Bash(rm -rf build)

` + rule + `
 Bash command

   rm -rf build
   Remove the build folder

 Do you want to proceed?
 ❯ 1. Yes
   2. Yes, and don't ask again for rm commands in
   /Users/me/project
   3. No, and tell Claude what to do differently (esc)

 Esc to cancel · Tab to amend
`

const framedEdit = rule + `
 Edit file
 src/app.ts
` + dashedRule + `
   1  const a = 1;
   2 -const b = 2;
   2 +const b = 3;
` + dashedRule + `
 Do you want to make this edit to app.ts?
   1. Yes
 ❯ 2. Yes, allow all edits during this session (shift+tab)
   3. No, and tell Claude what to do differently (esc)

 Esc to cancel · Tab to amend
`

const framedTrust = `
` + rule + `
 Accessing workspace:

 /Users/me/.herdr/worktrees/run/repo/task-test-hello

 Quick safety check: Is this a project you created or one you trust? (Like your
 own code, a well-known open source project, or work from your team). If not,
 take a moment to review what's in this folder first.

 ❯ No, exit
   Yes, I trust this folder

 Enter to confirm · Esc to cancel
`

func TestParseQuestionReadsADialogFramedByATopRule(t *testing.T) {
	cases := []struct {
		name   string
		screen string
		text   string
		labels []string
		picked int
	}{
		{"a Bash prompt: its words from the rule down, blank rows and all", framedBash,
			"Bash command\n\nrm -rf build\nRemove the build folder\n\nDo you want to proceed?",
			[]string{"Yes", "Yes, and don't ask again for rm commands in /Users/me/project", "No, and tell Claude what to do differently (esc)"}, 1},
		{"an Edit prompt: the diff's dashed rules read as breaks", framedEdit,
			"Edit file\nsrc/app.ts\n\n1  const a = 1;\n2 -const b = 2;\n2 +const b = 3;\n\nDo you want to make this edit to app.ts?",
			[]string{"Yes", "Yes, allow all edits during this session (shift+tab)", "No, and tell Claude what to do differently (esc)"}, 2},
		{"the trust prompt: choices without numbers or a box", framedTrust,
			"Accessing workspace:\n\n/Users/me/.herdr/worktrees/run/repo/task-test-hello\n\nQuick safety check: Is this a project you created or one you trust? (Like your\nown code, a well-known open source project, or work from your team). If not,\ntake a moment to review what's in this folder first.",
			[]string{"No, exit", "Yes, I trust this folder"}, 1},
	}
	for _, c := range cases {
		q, ok := ParseQuestion(c.screen)
		if !ok {
			t.Errorf("%s: not recognised", c.name)
			continue
		}
		if q.Text != c.text {
			t.Errorf("%s: text\n got %q\nwant %q", c.name, q.Text, c.text)
		}
		if len(q.Options) != len(c.labels) {
			t.Errorf("%s: %d choices, want %d: %+v", c.name, len(q.Options), len(c.labels), q.Options)
			continue
		}
		for i, o := range q.Options {
			if o.Label != c.labels[i] || o.N != i+1 || o.Picked != (o.N == c.picked) {
				t.Errorf("%s: choice %d = %+v, want %q picked=%v", c.name, i+1, o, c.labels[i], i+1 == c.picked)
			}
		}
	}
	keys, err := Answer(framedTrust, 1, "Yes, I trust this folder")
	if err != nil || strings.Join(keys, " ") != "down enter" {
		t.Errorf("trusting the folder: %v %v", keys, err)
	}
}

func TestParseQuestionLeavesTheInputBoxAlone(t *testing.T) {
	screens := map[string]string{
		"two lines typed into the input box":         "● Done.\n\n" + rule + "\n❯ first line\n  second line\n" + rule + "\n  ⏵⏵ accept edits on (shift+tab to cycle)\n",
		"a list above the input box, a rule over it": "● Here is a rule:\n" + rule + "\nPick one\n❯ Red\n  Green\n\n" + rule + "\n❯ \n" + rule + "\n",
		"a list under a rule with nothing between":   rule + "\n❯ Red\n  Green\n",
	}
	for name, screen := range screens {
		if q, ok := ParseQuestion(screen); ok {
			t.Errorf("%s was taken for a question: %+v", name, q)
		}
	}
}

func TestParseQuestionTakesNoRuleInTheConversationForAFrame(t *testing.T) {
	screen := "● Bash(printf rule)\n  ⎿  " + rule + "\n" + rule + "\n● Asking now.\n\n Which storage should the feed use?\n   1. A JSON file\n ❯ 2. SQLite\n\n Enter to select · Esc to cancel\n"
	q, ok := ParseQuestion(screen)
	if !ok || q.Text != "Which storage should the feed use?" || len(q.Options) != 2 || !q.Options[1].Picked {
		t.Fatalf("the conversation under a rule was read as the dialog's words: %+v %v", q, ok)
	}
	if _, ok := ParseQuestion(rule + "\n● Pick one\n❯ Red\n  Green\n"); ok {
		t.Fatal("an unnumbered list under a rule, the conversation between, was taken for a question")
	}
}
