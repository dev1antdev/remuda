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

	promptErr  error // what agent.prompt answers
	atPrompt   bool  // the shell holds the terminal
	atErr      error
	leaveAfter error // what agent.get answers from its second call
	gets       int
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
	p.gets++
	if p.getErr != nil {
		return herdr.AgentInfo{}, p.getErr
	}
	if p.gets > 1 && p.leaveAfter != nil {
		return herdr.AgentInfo{}, p.leaveAfter
	}
	return herdr.AgentInfo{Agent: p.agent, Status: p.next(&p.status)}, nil
}

func (p *paper) PaneAtPrompt(string) (bool, error) { return p.atPrompt, p.atErr }

func (p *paper) PaneRead(string) (string, error) {
	p.calls = append(p.calls, "read")
	return p.next(&p.screens), nil
}

func (p *paper) AgentPrompt(_, text string) error {
	p.calls = append(p.calls, "prompt:"+text)
	return p.promptErr
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

// A message that names a picture goes by herdr's submit, and Enter again only once the footer no longer says Pasting.
func TestAMessageWithAPictureIsEnteredAfterThePaste(t *testing.T) {
	quick(t)
	p := &paper{agent: "claude", status: []string{"idle"}, screens: []string{busyFoot, busyFoot, idleBox}}
	if err := submit(p, shot); err != nil {
		t.Fatal(err)
	}
	want := "prompt:" + shot + "|get|read|read|read|get|keys:enter"
	if got := strings.Join(p.calls, "|"); got != want {
		t.Fatalf("calls\n got %s\nwant %s", got, want)
	}
}

// Any other message, and any other agent's, is herdr's submit alone.
func TestOtherMessagesKeepHerdrsOwnSubmit(t *testing.T) {
	quick(t)
	p := &paper{agent: "claude", status: []string{"idle"}}
	if err := submit(p, "merge and release"); err != nil || strings.Join(p.calls, "|") != "prompt:merge and release" {
		t.Fatalf("a plain message: %v, %v", err, p.calls)
	}
	codex := &paper{agent: "codex", status: []string{"idle"}}
	if err := submit(codex, shot); err != nil || strings.Join(codex.calls, "|") != "prompt:"+shot+"|get" {
		t.Fatalf("another agent's: %v, %v", err, codex.calls)
	}
}

// What herdr's submit refuses goes nowhere: no paste of remuda's own, and no Enter.
func TestAPictureMessageGoesOnlyWhereHerdrSubmits(t *testing.T) {
	quick(t)
	for code, want := range map[string]error{"agent_not_found": ErrNoAgent, "agent_not_ready": ErrNoAgent, "agent_blocked": ErrAgentBusy} {
		p := &paper{agent: "claude", status: []string{"idle"}, promptErr: &herdr.Error{Code: code}}
		if err := submit(p, shot); !errors.Is(err, want) || strings.Join(p.calls, "|") != "prompt:"+shot {
			t.Errorf("%s: %v, %v", code, err, p.calls)
		}
	}
}

// One that asks between the paste and the Enter gets no Enter.
func TestAQuestionNeverGetsTheEnter(t *testing.T) {
	quick(t)
	meanwhile := &paper{agent: "claude", status: []string{"working", "blocked"}, screens: []string{idleBox}}
	if err := submit(meanwhile, shot); !errors.Is(err, ErrAskedMeanwhile) {
		t.Fatalf("a question put up meanwhile: %v", err)
	}
	if got := strings.Join(meanwhile.calls, "|"); strings.Contains(got, "keys:") {
		t.Fatalf("calls %s", got)
	}
}

// An agent gone after the submit gets no Enter.
func TestAnAgentGoneNeverGetsTheEnter(t *testing.T) {
	quick(t)
	for name, p := range map[string]*paper{
		"herdr no longer names it":  {agent: "claude", status: []string{"idle"}, screens: []string{idleBox}, leaveAfter: &herdr.Error{Code: "agent_not_found"}},
		"gone at once":              {getErr: &herdr.Error{Code: "agent_not_found"}},
		"a shell took the terminal": {agent: "claude", status: []string{"idle"}, screens: []string{idleBox}, atPrompt: true},
		"the terminal not known":    {agent: "claude", status: []string{"idle"}, screens: []string{idleBox}, atErr: errors.New("no answer")},
	} {
		if err := submit(p, shot); !errors.Is(err, ErrLeftMeanwhile) || strings.Contains(strings.Join(p.calls, "|"), "keys:") {
			t.Errorf("%s: %v, %v", name, err, p.calls)
		}
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
