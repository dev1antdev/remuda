package sessions

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
)

// paper is a herdr client on paper: who runs in the pane, the screens it shows one after another, and every call made.
type paper struct {
	agent   string
	status  []string // what agent.get answers, call after call; the last one stands
	screens []string // what pane.read answers, call after call; the last one stands
	getErr  error
	calls   []string
}

func (p *paper) next(list *[]string) string {
	if len(*list) == 0 {
		return ""
	}
	head := (*list)[0]
	if len(*list) > 1 {
		*list = (*list)[1:]
	}
	return head
}

func (p *paper) AgentGet(string) (herdr.AgentInfo, error) {
	p.calls = append(p.calls, "get")
	if p.getErr != nil {
		return herdr.AgentInfo{}, p.getErr
	}
	return herdr.AgentInfo{Agent: p.agent, Status: p.next(&p.status)}, nil
}

func (p *paper) PaneSendInput(_, text string, keys []string) error {
	p.calls = append(p.calls, "paste:"+text+strings.Join(keys, "+"))
	return nil
}

func (p *paper) PaneRead(string) (string, error) {
	p.calls = append(p.calls, "read")
	return p.next(&p.screens), nil
}

func (p *paper) AgentPrompt(_, text string) error {
	p.calls = append(p.calls, "prompt:"+text)
	return nil
}

func (p *paper) PaneSendText(_, text string) error {
	p.calls = append(p.calls, "text:"+text)
	return nil
}

func (p *paper) PaneSendKeys(_ string, keys []string) error {
	p.calls = append(p.calls, "keys:"+strings.Join(keys, "+"))
	return nil
}

func quick(t *testing.T) {
	first, poll, limit := SettleFirst, SettlePoll, SettleFor
	SettleFirst, SettlePoll, SettleFor = time.Millisecond, time.Millisecond, 40*time.Millisecond
	t.Cleanup(func() { SettleFirst, SettlePoll, SettleFor = first, poll, limit })
}

const (
	shot     = "look at this @.remuda/uploads/20261115-shot.png"
	idleBox  = "╭────╮\n│ > [Pasted text #1] │\n╰────╯\n  ⏵⏵ accept edits on"
	busyFoot = "╭────╮\n│ >  │\n╰────╯\n  Pasting…"
)

func submit(p *paper, text string) error {
	return (&Input{locks: map[events.PaneRef]*sync.Mutex{}}).Submit(p, events.PaneRef{Machine: "local", Pane: "w1:p1"}, text)
}

// A message that names a picture goes to Claude Code as a paste, and Enter only once its footer no longer says Pasting.
func TestAMessageWithAPictureIsPastedThenEntered(t *testing.T) {
	quick(t)
	p := &paper{agent: "claude", status: []string{"idle"}, screens: []string{busyFoot, busyFoot, idleBox}}
	if err := submit(p, shot); err != nil {
		t.Fatal(err)
	}
	want := "get|paste:" + shot + "|read|read|read|get|keys:enter"
	if got := strings.Join(p.calls, "|"); got != want {
		t.Fatalf("calls\n got %s\nwant %s", got, want)
	}
}

// Any other message, and any other agent's, goes through herdr's own submit as before.
func TestOtherMessagesKeepHerdrsOwnSubmit(t *testing.T) {
	quick(t)
	p := &paper{agent: "claude", status: []string{"idle"}}
	if err := submit(p, "merge and release"); err != nil || strings.Join(p.calls, "|") != "prompt:merge and release" {
		t.Fatalf("a plain message: %v, %v", err, p.calls)
	}
	codex := &paper{agent: "codex", status: []string{"idle"}}
	if err := submit(codex, shot); err != nil || strings.Join(codex.calls, "|") != "get|prompt:"+shot {
		t.Fatalf("another agent's: %v, %v", err, codex.calls)
	}
	shell := &paper{getErr: &herdr.Error{Code: "agent_not_found"}}
	if err := submit(shell, shot); err != nil || strings.Join(shell.calls, "|") != "get|prompt:"+shot {
		t.Fatalf("a pane herdr names no agent in: %v, %v", err, shell.calls)
	}
}

// An agent waiting on a question gets nothing; one that asks between the paste and the Enter gets no Enter.
func TestAQuestionNeverGetsTheEnter(t *testing.T) {
	quick(t)
	asking := &paper{agent: "claude", status: []string{"blocked"}}
	if err := submit(asking, shot); !errors.Is(err, ErrAgentBusy) || strings.Join(asking.calls, "|") != "get" {
		t.Fatalf("an agent asking: %v, %v", err, asking.calls)
	}
	meanwhile := &paper{agent: "claude", status: []string{"working", "blocked"}, screens: []string{idleBox}}
	err := submit(meanwhile, shot)
	if !errors.Is(err, ErrAskedMeanwhile) {
		t.Fatalf("a question put up meanwhile: %v", err)
	}
	if got := strings.Join(meanwhile.calls, "|"); strings.Contains(got, "keys:") || !strings.Contains(got, "paste:") {
		t.Fatalf("calls %s", got)
	}
}

// A footer that never stops saying Pasting is waited out, then Enter goes all the same.
func TestThePasteIsWaitedForOnlySoLong(t *testing.T) {
	quick(t)
	p := &paper{agent: "claude", status: []string{"idle"}, screens: []string{busyFoot}}
	began := time.Now()
	if err := submit(p, shot); err != nil {
		t.Fatal(err)
	}
	if got := p.calls[len(p.calls)-1]; got != "keys:enter" {
		t.Fatalf("the last call is %s", got)
	}
	if time.Since(began) > 2*time.Second {
		t.Fatalf("waited %s", time.Since(began))
	}
}

func TestNamesPicture(t *testing.T) {
	for text, want := range map[string]bool{
		"see @.remuda/uploads/a.png":       true,
		"/tmp/Screen Shot.JPEG is the one": true,
		"shot.webp":                        true,
		"merge and release":                false,
		"the pngs are fine":                false,
		"notes.pngx":                       false,
		"a.go and b.ts":                    false,
	} {
		if got := NamesPicture(text); got != want {
			t.Errorf("NamesPicture(%q) = %v", text, got)
		}
	}
}

func TestStillPastingReadsTheFooterAlone(t *testing.T) {
	if !stillPasting(busyFoot) {
		t.Error("a footer that says Pasting is not read")
	}
	if stillPasting(idleBox) {
		t.Error("a footer at rest reads as pasting")
	}
	talk := "why does it say Pasting… forever?\n" + strings.Repeat("line\n", 12) + idleBox
	if stillPasting(talk) {
		t.Error("the word in the conversation above the footer counts")
	}
}
