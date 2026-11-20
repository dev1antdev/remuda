package core_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/sessions"
)

// While remuda-core holds a prompt for the person's draft in the terminal, the person's keys from the page go at once,
// and the pane's prompts follow the draft in the order they were sent.
func TestKeysPassAPromptHeldForAPersonsDraft(t *testing.T) {
	core.LiftPromptGate(t)
	client, adapter, pane := idleAgent(t)

	person, err := client.Attach(context.Background(), core.AttachParams{Terminal: pane, Cols: 80, Rows: 24, KeepSize: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { person.Close() })
	go func() { _, _ = io.Copy(io.Discard, person) }()
	if _, err := person.Write([]byte("draft")); err != nil {
		t.Fatal(err)
	}

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

	start := time.Now()
	if err := in.SendKeys(adapter, ref, []string{"esc"}); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("Esc waited %s behind a prompt the core held", took)
	}

	if _, err := person.Write([]byte("\x15")); err != nil { // Ctrl-U: the person clears the draft, and no turn starts
		t.Fatal(err)
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
