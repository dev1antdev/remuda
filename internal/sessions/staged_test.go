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

// holder stages its prompts as remuda-core does: the look answers at once, the send waits.
type holder struct {
	ledger
	sendErr error
}

func newHolder() *holder { return &holder{ledger: ledger{sends: make(chan struct{}, 4)}} }

func (h *holder) StagePrompt(_, text string) (func() error, error) {
	h.note("look:" + text)
	return func() error {
		<-h.sends
		h.note("sent:" + text)
		return h.sendErr
	}, nil
}

func (h *holder) AgentPrompt(pane, text string) error {
	send, err := h.StagePrompt(pane, text)
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

// Keys and a screen terminal's typing pass a prompt remuda-core holds for a person's draft: the pane's lock covered its look alone.
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
