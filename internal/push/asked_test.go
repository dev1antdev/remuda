package push

import (
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
)

const bashDialog = "Bash command\n\nrm -rf build\nRemove the build folder\n\nDo you want to proceed?"

func TestQuestionPushCarriesTheChoicesAsButtons(t *testing.T) {
	shrinkDelays(t)
	s, sent := newTestService(t)
	var asked []string
	s.Question = func(machine, pane string) (Asked, bool) {
		asked = append(asked, machine+"/"+pane)
		return Asked{Text: bashDialog, Choices: []string{"Yes", "Yes, and don't ask again for rm commands in /Users/me/project", "No, and tell Claude what to do differently (esc)"}}, true
	}
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "p1"}, Workspace: "api", Agent: "claude", From: "working", To: "blocked"})
	time.Sleep(80 * time.Millisecond)
	got := sent.get()
	if len(got) != 1 || len(asked) != 1 || asked[0] != "local/p1" {
		t.Fatalf("one push after one read of the screen: %+v, read %v", got, asked)
	}
	p := got[0]
	if p.Body != "Bash command · rm -rf build" {
		t.Errorf("body %q", p.Body)
	}
	if len(p.Actions) != 2 || p.Actions[0].Action != "choice-0" || p.Actions[0].Title != "Yes" || p.Actions[1].Action != "choice-1" || !strings.HasSuffix(p.Actions[1].Title, "…") {
		t.Errorf("actions %+v", p.Actions)
	}
	if len(p.Choices) != 3 || p.Choices[2] != "No, and tell Claude what to do differently (esc)" {
		t.Errorf("choices %v", p.Choices)
	}
}

func TestQuestionPushWithoutADialogStaysPlain(t *testing.T) {
	cases := map[string]struct {
		asked Asked
		ok    bool
	}{
		"no dialog on the screen": {Asked{}, false},
		"one choice":              {Asked{Text: "Really?", Choices: []string{"Yes"}}, true},
	}
	for name, c := range cases {
		p := askedPayload(Payload{Kind: "input", Body: "claude is waiting for an answer"}, c.asked, c.ok)
		if len(p.Actions) != 0 || len(p.Choices) != 0 {
			t.Errorf("%s: actions %+v choices %v", name, p.Actions, p.Choices)
		}
		if c.ok && p.Body != "Really?" || !c.ok && p.Body != "claude is waiting for an answer" {
			t.Errorf("%s: body %q", name, p.Body)
		}
	}
	// no Question input at all: the push as built
	s, _ := newTestService(t)
	if p := s.asked(Payload{Body: "as built"}); p.Body != "as built" || p.Actions != nil {
		t.Errorf("without a reader: %+v", p)
	}
}

func TestQuestionBodyAndCut(t *testing.T) {
	if got := questionBody("Edit file\n\nsrc/app.ts\n\nDo you want to make this edit?"); got != "Edit file · src/app.ts" {
		t.Errorf("body %q", got)
	}
	if got := questionBody("   \n"); got != "" {
		t.Errorf("empty dialog %q", got)
	}
	long := strings.Repeat("слово ", 30)
	if got := cutRunes(long, 24); len([]rune(got)) != 24 || !strings.HasSuffix(got, "…") {
		t.Errorf("cut %q", got)
	}
	if got := cutRunes("short", 24); got != "short" {
		t.Errorf("uncut %q", got)
	}
}
