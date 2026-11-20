package core_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/sessions"
)

// While remuda-core holds a prompt for the person's draft in the terminal, and a restart waits for it, the person's
// keys go at once; the pane's prompts follow the draft in the order they were sent, and the restart after them.
func TestKeysPassAPromptHeldForAPersonsDraftAndARestartWaitingForIt(t *testing.T) {
	core.LiftPromptGate(t)
	client, adapter, pane := idleAgent(t)

	person := typing(t, client, pane, "draft")

	in, ref := sessions.NewInput(nil, nil), events.PaneRef{Machine: "local", Pane: pane}
	first, second := make(chan error, 1), make(chan error, 1)
	go func() { first <- in.Submit(adapter, ref, "first") }()
	time.Sleep(300 * time.Millisecond)
	go func() { second <- in.Submit(adapter, ref, "second") }()
	select {
	case err := <-first:
		t.Fatalf("a prompt went over the person's draft: %v", err)
	case <-time.After(time.Second):
	}

	restart, held := in.Hold(ref.Machine, ref.Pane), make(chan struct{})
	go func() { restart.Lock(); close(held) }()
	start := time.Now()
	if err := in.SendKeys(adapter, ref, []string{"esc"}); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("Esc waited %s behind a prompt the core held and a restart waiting for it", took)
	}
	select {
	case <-held:
		t.Fatal("a restart went ahead of a prompt the core held")
	case <-time.After(300 * time.Millisecond):
	}

	if _, err := person.Write([]byte("\x15")); err != nil { // Ctrl-U: the person clears the draft, and no turn starts
		t.Fatal(err)
	}
	select {
	case <-held:
		restart.Unlock()
	case <-time.After(20 * time.Second):
		t.Fatal("the restart never took the pane once the prompt before it went")
	}
	for _, sent := range []chan error{first, second} {
		select {
		case err := <-sent:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(20 * time.Second):
			t.Fatal("the prompts never went once the person cleared the draft")
		}
	}
	screenSays(t, adapter, pane, "got: second")
	screen, _ := adapter.PaneRead(pane)
	if one, two := strings.Index(screen, "got: first"), strings.Index(screen, "got: second"); one < 0 || one > two || strings.Contains(screen, "draft") {
		t.Fatalf("not first then second, the draft cleared:\n%s", screen)
	}
}

// typing is a person in the terminal of the pane, who has typed text there and not sent it yet.
func typing(t *testing.T, client *core.Client, pane, text string) *core.Stream {
	t.Helper()
	person, err := client.Attach(context.Background(), core.AttachParams{Terminal: pane, Cols: 80, Rows: 24, KeepSize: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { person.Close() })
	go func() { _, _ = io.Copy(io.Discard, person) }()
	if _, err := person.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
	return person
}

// waitFor is the answer on sent, within the time a prompt held for a draft may take.
func waitFor(t *testing.T, sent chan error) error {
	t.Helper()
	select {
	case err := <-sent:
		return err
	case <-time.After(30 * time.Second):
		t.Fatal("the prompt never came back")
		return nil
	}
}

// A prompt waiting for the person's draft goes to the agent when the person sends the draft and so starts a turn: a
// message to the agent at work, after the draft, never refused as though the agent had gone.
func TestAPromptWaitingForADraftGoesWhenThePersonSendsIt(t *testing.T) {
	core.LiftPromptGate(t)
	client, adapter, pane := idleAgent(t)
	person := typing(t, client, pane, "slow")
	in, ref := sessions.NewInput(nil, nil), events.PaneRef{Machine: "local", Pane: pane}
	sent := make(chan error, 1)
	go func() { sent <- in.Submit(adapter, ref, "after the draft") }()
	time.Sleep(time.Second)
	if _, err := person.Write([]byte("\r")); err != nil {
		t.Fatal(err)
	}
	if err := waitFor(t, sent); err != nil {
		t.Fatalf("a prompt behind a draft the person sent: %v", err)
	}
	screenSays(t, adapter, pane, "got: after the draft")
	screen, _ := adapter.PaneRead(pane)
	if draft, prompt := strings.Index(screen, "got: slow"), strings.Index(screen, "got: after the draft"); draft < 0 || draft > prompt {
		t.Fatalf("not the draft, then the prompt:\n%s", screen)
	}
}

// A prompt whose agent leaves the terminal while it waits for the person's draft sends nothing, and the same delivery
// tried again never reaches the agent started in its place, nor the shell between them.
func TestAPromptNeverReachesWhatReplacedItsAgent(t *testing.T) {
	core.LiftPromptGate(t)
	client, adapter, pane := idleAgent(t)
	if hello, err := client.Hello(context.Background()); err != nil || !hello.Has("send_foreground") {
		t.Skip("this remuda-core does not check the agent as it types (send_foreground), and would type the prompt into the shell")
	}
	person := typing(t, client, pane, "draft")
	delivery := sessions.NewInput(nil, nil).Deliver(adapter, events.PaneRef{Machine: "local", Pane: pane}, "echo typed-past-its-agent")
	sent := make(chan error, 1)
	go func() { sent <- delivery.Try() }()
	time.Sleep(time.Second)
	if _, err := person.Write([]byte("\x03")); err != nil { // Ctrl-C: the agent goes, its shell takes the terminal
		t.Fatal(err)
	}
	if err := waitFor(t, sent); !herdr.IsCode(err, "agent_left") && !herdr.IsCode(err, "agent_not_ready") {
		t.Fatalf("a prompt whose agent left as it waited: %v", err)
	}
	startStandIn(t, adapter, pane)
	if err := delivery.Try(); !herdr.IsCode(err, "agent_left") {
		t.Fatalf("the delivery tried again on the agent started in its place: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	if screen, _ := adapter.PaneRead(pane); strings.Contains(screen, "typed-past-its-agent") {
		t.Fatalf("the prompt reached the shell or the new agent:\n%s", screen)
	}
}

// A first prompt the agent refused while it asked something, tried again once another agent took the pane, is
// agent_left: the task never reaches the agent started in its place.
func TestADeliveryAfterAQuestionNeverReachesAReplacement(t *testing.T) {
	core.LiftPromptGate(t)
	client, adapter, pane := idleAgent(t)
	person := typing(t, client, pane, "ask\r")
	eventually(t, adapter, func(s *herdr.Snapshot) bool { return s.Pane(pane).AgentStatus == herdr.StatusBlocked })
	delivery := sessions.NewInput(nil, nil).Deliver(adapter, events.PaneRef{Machine: "local", Pane: pane}, "the old task")
	if err := delivery.Try(); !herdr.IsCode(err, "agent_blocked") {
		t.Fatalf("a prompt to an agent asking something: %v", err)
	}
	if _, err := person.Write([]byte("\x03")); err != nil {
		t.Fatal(err)
	}
	startStandIn(t, adapter, pane)
	if err := delivery.Try(); !herdr.IsCode(err, "agent_left") {
		t.Fatalf("the first prompt tried again on the agent started in its place: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	if screen, _ := adapter.PaneRead(pane); strings.Contains(screen, "got: the old task") {
		t.Fatalf("the new agent got the old task:\n%s", screen)
	}
}

// A prompt for an agent at rest (a restart's exit, a sleep's) is refused untyped when the person's own draft starts a
// turn while it waits: only the person's message goes to a turn.
func TestARestartsExitNeverGoesToATurnThePersonStarted(t *testing.T) {
	core.LiftPromptGate(t)
	client, adapter, pane := idleAgent(t)
	person := typing(t, client, pane, "slow")
	sent := make(chan error, 1)
	go func() { sent <- adapter.AgentPrompt(pane, "exit-while-it-works") }()
	time.Sleep(time.Second)
	if _, err := person.Write([]byte("\r")); err != nil {
		t.Fatal(err)
	}
	if err := waitFor(t, sent); !herdr.IsCode(err, "agent_not_ready") {
		t.Fatalf("a prompt for an agent at rest, behind a turn the person started: %v", err)
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool { return s.Pane(pane).AgentStatus == herdr.StatusIdle })
	if screen, _ := adapter.PaneRead(pane); strings.Contains(screen, "exit-while-it-works") {
		t.Fatalf("the prompt went to the turn:\n%s", screen)
	}
}
