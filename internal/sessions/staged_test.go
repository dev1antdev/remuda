package sessions

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
)

// ledger notes a fake client's calls in order; each prompt waits for a token on sends.
type ledger struct {
	mu    sync.Mutex
	calls []string
	sends chan struct{}
}

func (l *ledger) note(call string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, call)
}

func (l *ledger) seen() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.calls)
}

func (l *ledger) PaneSendText(_, text string) error { l.note("text:" + text); return nil }

func (l *ledger) PaneSendKeys(_ string, keys []string) error {
	l.note("keys:" + strings.Join(keys, "+"))
	return nil
}

// holder stages its prompts as remuda-core does: the look answers at once with the pid in front, the send waits.
type holder struct {
	ledger
	sendErr error
	pid     int
	asks    bool   // the agent in front asks something: the look refuses it
	asked   []bool // asMessage, look after look
}

func newHolder() *holder { return &holder{ledger: ledger{sends: make(chan struct{}, 4)}, pid: 100} }

func (h *holder) StagePrompt(_, text string, front int, asMessage bool) (int, func() error, error) {
	h.note("look:" + text)
	h.mu.Lock()
	h.asked = append(h.asked, asMessage)
	pid, asks := h.pid, h.asks
	h.mu.Unlock()
	if front != 0 && front != pid {
		return 0, nil, &herdr.Error{Code: "agent_left", Message: "another agent in front"}
	}
	if asks {
		return pid, nil, &herdr.Error{Code: "agent_blocked", Message: "asks first"}
	}
	return pid, func() error {
		<-h.sends
		h.note("sent:" + text)
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.sendErr
	}, nil
}

func (h *holder) AgentPrompt(pane, text string) error {
	_, send, err := h.StagePrompt(pane, text, 0, false)
	if err != nil {
		return err
	}
	return send()
}

// waiter is herdr's agent.prompt held up until a token comes: a client that stages nothing.
type waiter struct{ ledger }

func (w *waiter) AgentPrompt(_, text string) error {
	w.note("prompt:" + text)
	<-w.sends
	w.note("sent:" + text)
	return nil
}

var onePane = events.PaneRef{Machine: "local", Pane: "w1:p1"}

func until(t *testing.T, h *ledger, call string) {
	t.Helper()
	for start := time.Now(); time.Since(start) < 2*time.Second; time.Sleep(time.Millisecond) {
		if slices.Contains(h.seen(), call) {
			return
		}
	}
	t.Fatalf("%s never came: %v", call, h.seen())
}

func within(t *testing.T, what string, do func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- do() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	case <-time.After(time.Second):
		t.Fatalf("%s waited behind the prompt", what)
	}
}

// Keys pass a prompt remuda-core holds for a person's draft: the pane's lock covered its look alone.
func TestKeysPassAPromptTheCoreHolds(t *testing.T) {
	h, in := newHolder(), NewInput(nil, nil)
	sent := make(chan error, 1)
	go func() { sent <- in.Submit(h, onePane, "hello") }()
	until(t, &h.ledger, "look:hello")
	within(t, "Esc", func() error { return in.SendKeys(h, onePane, []string{"esc"}) })
	h.sends <- struct{}{}
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
	if got := h.seen(); !slices.Equal(got, []string{"look:hello", "keys:esc", "sent:hello"}) {
		t.Fatalf("calls %v", got)
	}
}

// A client that stages nothing — herdr, whose agent.prompt answers at once — keeps the pane's lock through its prompt.
func TestHerdrsPromptHoldsThePanesLock(t *testing.T) {
	w, in := &waiter{ledger{sends: make(chan struct{}, 1)}}, NewInput(nil, nil)
	go func() { _ = in.Submit(w, onePane, "hello") }()
	until(t, &w.ledger, "prompt:hello")
	keys := make(chan error, 1)
	go func() { keys <- in.SendKeys(w, onePane, []string{"esc"}) }()
	select {
	case <-keys:
		t.Fatal("a key passed herdr's prompt")
	case <-time.After(100 * time.Millisecond):
	}
	w.sends <- struct{}{}
	if err := <-keys; err != nil {
		t.Fatal(err)
	}
	if got := w.seen(); !slices.Equal(got, []string{"prompt:hello", "sent:hello", "keys:esc"}) {
		t.Fatalf("calls %v", got)
	}
}

// A pane's staged prompts go one after another: the second is looked at only once the first went.
func TestAPanesStagedPromptsGoInTurn(t *testing.T) {
	h, in := newHolder(), NewInput(nil, nil)
	first, second := make(chan error, 1), make(chan error, 1)
	go func() { first <- in.SubmitToAgent(h, onePane, "first") }()
	until(t, &h.ledger, "look:first")
	go func() { second <- in.SubmitToAgent(h, onePane, "second") }()
	time.Sleep(50 * time.Millisecond)
	h.sends <- struct{}{}
	h.sends <- struct{}{}
	if err := errors.Join(<-first, <-second); err != nil {
		t.Fatal(err)
	}
	if got := h.seen(); !slices.Equal(got, []string{"look:first", "sent:first", "look:second", "sent:second"}) {
		t.Fatalf("calls %v", got)
	}
}

// A prompt pasted but left unsubmitted is said so and kept, never taken for one that went or for nothing sent.
func TestAPastedPromptLeftUnsubmittedIsSaidSo(t *testing.T) {
	h, in := newHolder(), NewInput(nil, nil)
	h.sendErr = &herdr.Error{Code: "not_submitted", Message: "pasted, no Enter"}
	h.sends <- struct{}{}
	err := in.Submit(h, onePane, "hello")
	if !errors.Is(err, ErrNotSubmitted) || !NotDelivered(err) || errors.Is(err, ErrNoAgent) {
		t.Fatalf("refusal %v", err)
	}
	if !strings.Contains(err.Error(), "look at the terminal") {
		t.Fatalf("words %q", err)
	}
}

// A restart's or a sleep's hold keeps a staged prompt out until it lets go.
func TestAStagedPromptWaitsForAHold(t *testing.T) {
	h, in := newHolder(), NewInput(nil, nil)
	hold := in.Hold(onePane.Machine, onePane.Pane)
	hold.Lock()
	sent := make(chan error, 1)
	go func() { sent <- in.Submit(h, onePane, "hello") }()
	time.Sleep(50 * time.Millisecond)
	if got := h.seen(); len(got) > 0 {
		t.Fatalf("a prompt was looked at under a hold: %v", got)
	}
	hold.Unlock()
	h.sends <- struct{}{}
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
}

// A hold waits for a staged prompt already on its way, and meanwhile holds nothing the person's keys need.
func TestAHoldWaitsForAPromptOnItsWayWithoutHoldingKeysUp(t *testing.T) {
	h, in := newHolder(), NewInput(nil, nil)
	sent := make(chan error, 1)
	go func() { sent <- in.Submit(h, onePane, "hello") }()
	until(t, &h.ledger, "look:hello")
	hold, held := in.Hold(onePane.Machine, onePane.Pane), make(chan struct{})
	go func() { hold.Lock(); close(held) }()
	within(t, "Esc", func() error { return in.SendKeys(h, onePane, []string{"esc"}) })
	select {
	case <-held:
		t.Fatal("the hold went ahead of a prompt on its way")
	case <-time.After(50 * time.Millisecond):
	}
	h.sends <- struct{}{}
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
	<-held
	hold.Unlock()
}

// A first prompt whose agent left the front as it went is not tried again, so it never reaches what replaced it.
func TestAFirstPromptIsNotTriedAgainOnceItsAgentLeft(t *testing.T) {
	retry := FirstPromptRetry
	FirstPromptRetry = time.Millisecond
	t.Cleanup(func() { FirstPromptRetry = retry })
	h, in := newHolder(), NewInput(nil, nil)
	h.sendErr = &herdr.Error{Code: "agent_left", Message: "gone from the front"}
	h.sends <- struct{}{}
	if err := in.FirstPrompt(h, onePane, "the task", time.Minute); !errors.Is(err, ErrNoAgent) {
		t.Fatalf("refusal %v", err)
	}
	if got := h.seen(); !slices.Equal(got, []string{"look:the task", "sent:the task"}) {
		t.Fatalf("tried again: %v", got)
	}
}

// A delivery tried again is bound to the agent its first look saw: once another agent leads the pane, it stops there,
// and a refusal it may try again after (the agent asking first) never sends the prompt to what replaced the agent.
func TestADeliveryTriedAgainNeverReachesAReplacement(t *testing.T) {
	for refused, want := range map[string][]string{
		"at the look": {"look:the task", "look:the task"},
		"at the send": {"look:the task", "sent:the task", "look:the task"},
	} {
		h, in := newHolder(), NewInput(nil, nil)
		if refused == "at the look" {
			h.asks = true
		} else {
			h.sendErr = &herdr.Error{Code: "agent_blocked", Message: "asks first"}
			h.sends <- struct{}{}
		}
		d := in.Deliver(h, onePane, "the task")
		if err := d.Try(); !herdr.IsCode(err, "agent_blocked") {
			t.Fatalf("%s: first try %v", refused, err)
		}
		h.pid, h.asks, h.sendErr = 200, false, nil
		if err := d.Try(); !herdr.IsCode(err, "agent_left") {
			t.Fatalf("%s: a try with another agent in front: %v", refused, err)
		}
		if got := h.seen(); !slices.Equal(got, want) {
			t.Fatalf("%s: calls %v", refused, got)
		}
	}
}

// Only the person's own message may go to a turn started meanwhile; every other prompt is for an agent at rest.
func TestOnlyThePersonsMessageGoesToATurn(t *testing.T) {
	h, in := newHolder(), NewInput(nil, nil)
	for range 3 {
		h.sends <- struct{}{}
	}
	if err := errors.Join(in.Submit(h, onePane, "mine"), in.SubmitToAgent(h, onePane, "a schedule's"), in.HandOff(h, onePane, "a review I sent")); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.asked, []bool{true, false, true}) {
		t.Fatalf("as a message: %v", h.asked)
	}
}

// A pull request's hand-off naming a picture goes as it is, no Enter after it: a dialog the work puts up at once is
// no failure of the hand-off, where the person's own message would be said to be still unsent.
func TestAHandOffNamingAPictureGetsNoEnterAfterIt(t *testing.T) {
	quick(t)
	review := "review: see " + shot
	handed := &paper{agent: claudeAgent, status: []string{statusBlocked}, screens: []string{busyFoot}}
	if err := NewInput(nil, nil).HandOff(handed, onePane, review); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(handed.calls, []string{"prompt:" + review}) {
		t.Fatalf("calls %v", handed.calls)
	}
	submitted := &paper{agent: claudeAgent, status: []string{statusBlocked}, screens: []string{busyFoot}}
	if err := NewInput(nil, nil).Submit(submitted, onePane, review); !errors.Is(err, ErrAskedMeanwhile) {
		t.Fatalf("the person's own message: %v", err)
	}
}

// A prompt for an agent at rest refused because the agent is at work is ErrAgentWorking, which a schedule skips
// quietly, and a first prompt waits it out.
func TestAPromptForAnAgentAtRestFindingItAtWorkWaits(t *testing.T) {
	h, in := newHolder(), NewInput(nil, nil)
	h.sendErr = &herdr.Error{Code: "agent_working", Message: "at work"}
	h.sends <- struct{}{}
	if err := in.SubmitToAgent(h, onePane, "a schedule's"); !errors.Is(err, ErrAgentWorking) {
		t.Fatalf("a schedule's prompt to an agent at work: %v", err)
	}
	retry := FirstPromptRetry
	FirstPromptRetry = time.Millisecond
	t.Cleanup(func() { FirstPromptRetry = retry })
	h.sends <- struct{}{}
	go func() {
		for !slices.Contains(h.seen(), "sent:the first") {
			time.Sleep(time.Millisecond)
		}
		h.mu.Lock()
		h.sendErr = nil
		h.mu.Unlock()
		h.sends <- struct{}{}
	}()
	if err := in.FirstPrompt(h, onePane, "the first", time.Minute); err != nil {
		t.Fatalf("a first prompt waiting out a turn: %v", err)
	}
}
