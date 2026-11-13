package sessions

import (
	"errors"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
)

// Liner is what typing a command line into a new shell needs of herdr's client.
type Liner interface {
	PaneAtPrompt(paneID string) (bool, error)
	PaneRead(paneID string) (string, error)
	PaneSendText(paneID, text string) error
	PaneSendKeys(paneID string, keys []string) error
}

// ShellReady is how long TypeLine waits for a new pane's shell to draw its prompt; ShellPoll how often it looks.
var (
	ShellReady = 20 * time.Second
	ShellPoll  = 300 * time.Millisecond
)

// ErrShellNotReady: the shell never drew a prompt, and nothing was typed.
var ErrShellNotReady = errors.New("the shell did not come up; nothing was typed")

// TypeLine types one command line into the shell of a pane remuda just made at the person's press — a run action's
// command, a setup's — and presses Enter, under the pane's lock: once its shell holds the terminal and has drawn
// something (its prompt), so the line is never read before the shell's own start-up is through.
func (in *Input) TypeLine(client Liner, ref events.PaneRef, line string) error {
	if strings.ContainsAny(line, "\r\n") {
		return errors.New("a command line is one line")
	}
	deadline := time.Now().Add(ShellReady)
	for {
		at, err := client.PaneAtPrompt(ref.Pane)
		if err == nil && at {
			if screen, err := client.PaneRead(ref.Pane); err == nil && strings.TrimSpace(screen) != "" {
				break
			}
		}
		if time.Now().After(deadline) {
			if err != nil {
				return err
			}
			return ErrShellNotReady
		}
		time.Sleep(ShellPoll)
	}
	lock := in.lock(ref)
	lock.Lock()
	defer lock.Unlock()
	if err := client.PaneSendText(ref.Pane, line); err != nil {
		return err
	}
	time.Sleep(150 * time.Millisecond)
	return client.PaneSendKeys(ref.Pane, []string{"enter"})
}
