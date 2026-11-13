package sessions

import (
	"errors"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
)

// shellComingUp is a new pane whose shell holds the terminal before it has drawn its prompt
type shellComingUp struct {
	looks int
	sent  []string
}

func (s *shellComingUp) PaneAtPrompt(string) (bool, error) { s.looks++; return s.looks > 1, nil }
func (s *shellComingUp) PaneRead(string) (string, error) {
	if s.looks < 3 {
		return "", nil
	}
	return "me@box app % ", nil
}
func (s *shellComingUp) PaneSendText(_, text string) error { s.sent = append(s.sent, text); return nil }
func (s *shellComingUp) PaneSendKeys(_ string, k []string) error {
	s.sent = append(s.sent, k...)
	return nil
}

func TestALineIsTypedOnceTheShellHasDrawnItsPrompt(t *testing.T) {
	ShellPoll = time.Millisecond
	defer func() { ShellPoll = 300 * time.Millisecond }()
	in := NewInput(nil, nil)
	shell := &shellComingUp{}
	if err := in.TypeLine(shell, events.PaneRef{Machine: "local", Pane: "w1:p2"}, "npm run dev"); err != nil {
		t.Fatal(err)
	}
	if shell.looks < 3 || len(shell.sent) != 2 || shell.sent[0] != "npm run dev" || shell.sent[1] != "enter" {
		t.Errorf("looked %d times, sent %v", shell.looks, shell.sent)
	}
	if err := in.TypeLine(shell, events.PaneRef{Machine: "local", Pane: "w1:p2"}, "a\nb"); err == nil {
		t.Error("two lines were typed as one")
	}
}

type deadShell struct{ shellComingUp }

func (deadShell) PaneAtPrompt(string) (bool, error) { return false, nil }

func TestAShellThatNeverComesUpIsTypedNothing(t *testing.T) {
	ShellPoll, ShellReady = time.Millisecond, 20*time.Millisecond
	defer func() { ShellPoll, ShellReady = 300*time.Millisecond, 20*time.Second }()
	shell := &deadShell{}
	if err := NewInput(nil, nil).TypeLine(shell, events.PaneRef{Pane: "w1:p2"}, "make"); !errors.Is(err, ErrShellNotReady) || len(shell.sent) != 0 {
		t.Errorf("got %v, sent %v", err, shell.sent)
	}
}
