package push

import (
	"strings"
	"sync"
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
		return Asked{Text: bashDialog, Choices: []string{"Yes", "Yes, and don't ask again for rm commands in /Users/me/project", "No, and tell Claude what to do differently (esc)"}, Key: "0f1e2d3c4b5a6978"}, true
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
	if p.Dialog != "0f1e2d3c4b5a6978" {
		t.Errorf("the dialog's key does not ride with its choices: %q", p.Dialog)
	}
}

func TestQuestionPushWithoutADialogStaysPlain(t *testing.T) {
	cases := map[string]struct {
		asked Asked
		ok    bool
	}{
		"no dialog on the screen": {Asked{}, false},
		"one choice":              {Asked{Text: "Really?", Choices: []string{"Yes"}, Key: "k"}, true},
	}
	for name, c := range cases {
		p := askedPayload(Payload{Kind: "input", Body: "claude is waiting for an answer"}, c.asked, c.ok)
		if len(p.Actions) != 0 || len(p.Choices) != 0 || p.Dialog != "" {
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

// The screen is read once the delay has passed, over ssh for a remote pane: an answer at the desk during that read must
// still call the push off, since the timer it stopped had already fired.
func TestAnAnswerWhileTheScreenIsReadCallsThePushOff(t *testing.T) {
	shrinkDelays(t)
	s, sent := newTestService(t)
	reading := make(chan struct{}, 1)
	release := make(chan struct{})
	s.Question = func(machine, pane string) (Asked, bool) {
		reading <- struct{}{}
		<-release
		return Asked{Text: bashDialog, Choices: []string{"Yes", "No"}}, true
	}
	var mu sync.Mutex
	var recorded []string
	s.OnAlert = func(p Payload) {
		mu.Lock()
		recorded = append(recorded, p.Title)
		mu.Unlock()
	}
	s.OnStatus(blockedEvent())
	select {
	case <-reading:
	case <-time.After(time.Second):
		t.Fatal("the screen was never read")
	}
	answered := blockedEvent()
	answered.From, answered.To = "blocked", "working"
	s.OnStatus(answered)
	close(release)
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(sent.get()) != 0 || len(recorded) != 0 {
		t.Fatalf("answered at the desk while the screen was read: sent %+v, recorded %v", sent.get(), recorded)
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
