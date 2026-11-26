package sessions

import (
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestHandoffAnswersTheTrustQuestionOnceAndThenLeaves(t *testing.T) {
	client := &fakeClient{
		screen:    "Accessing workspace:\n/w/wt\n❯ No, exit\n  Yes, I trust this folder\n",
		promptErr: &herdr.Error{Code: "agent_blocked"},
	}
	trusted := 0
	h := Handoff{
		Input: NewInput(nil, nil), Client: client, Ref: events.PaneRef{Machine: "m", Pane: "w1:p1"},
		Prompt: "go", Dir: "/w/wt", Retry: time.Millisecond, Timeout: time.Second,
		Trusted: func() {
			trusted++
			client.promptErr = nil // the agent is free once its folder is trusted
		},
	}
	last, timedOut := h.Run()
	if last != nil || timedOut || trusted != 1 || len(client.keys) != 2 {
		t.Fatalf("last %v, timedOut %v, trusted %d, keys %v", last, timedOut, trusted, client.keys)
	}
	if strings.Join(client.keys[0], " ") != "down" || strings.Join(client.keys[1], " ") != "enter" {
		t.Fatalf("keys %v, want down then enter", client.keys)
	}
	if len(client.typed) != 1 || client.typed[0] != "prompt:go" {
		t.Fatalf("typed %v", client.typed)
	}
}

func TestHandoffSaysOnceAboutAnotherQuestionAndTimesOut(t *testing.T) {
	client := &fakeClient{screen: "Do you want to proceed?\n❯ Yes\n  No", promptErr: &herdr.Error{Code: "agent_blocked"}}
	asked := 0
	h := Handoff{
		Input: NewInput(nil, nil), Client: client, Ref: events.PaneRef{Machine: "m", Pane: "w1:p1"},
		Prompt: "go", Dir: "/w/wt", Retry: time.Millisecond, Timeout: 15 * time.Millisecond,
		Notify: func() { asked++ },
	}
	last, timedOut := h.Run()
	if !timedOut || !herdr.IsCode(last, "agent_blocked") || asked != 1 {
		t.Fatalf("last %v, timedOut %v, asked %d", last, timedOut, asked)
	}
}
