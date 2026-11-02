package chat

import (
	"strings"
	"testing"
)

// a multi-select AskUserQuestion as Claude Code 2.1.316 draws it: a box on every row, ticked "[✔]", Enter ticking the
// row under the cursor, and a Submit row under the choices that sends what is ticked
const multiSelect = `● Two features need a call.

 ☐ Features

 Which features should ship?

 ❯ 1. [ ] Search
      Find things across machines
   2. [✔] Export
      Save a conversation as Markdown
   3. [ ] Type something

     Submit

 Enter to select · ↑/↓ to navigate · Esc to cancel
`

func TestAMultiSelectIsReadButNotAnsweredByOneChoice(t *testing.T) {
	q, ok := ParseQuestion(multiSelect)
	if !ok || !q.Multi || len(q.Options) != 3 || !q.Options[0].Picked || q.Text != "Which features should ship?" {
		t.Fatalf("the multi-select: %+v %v", q, ok)
	}
	if choices := q.Choices(); len(choices) != 0 {
		t.Fatalf("a push offers choices a tap cannot answer: %v", choices)
	}
	keys, err := Answer(multiSelect, 1, q.Options[1].Label)
	if err == nil || !strings.Contains(err.Error(), "several choices") {
		t.Fatalf("one choice walked into a multi-select: %v %v", keys, err)
	}
	single, _ := ParseQuestion(severalQuestions)
	if single.Multi || len(single.Choices()) != 4 || single.Choices()[2] != single.Options[2].Label {
		t.Fatalf("a single choice keeps its choices: %+v", single)
	}
}
