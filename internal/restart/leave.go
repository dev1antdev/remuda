package restart

import (
	"fmt"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// Leaver is what asking an agent to leave needs of herdr's client: the agent's own exit typed, keys pressed when it
// lingers, and the snapshot that shows the shell under it.
type Leaver interface {
	SessionSnapshot() (*herdr.Snapshot, error)
	AgentPrompt(target, text string) error
	PaneSendKeys(paneID string, keys []string) error
}

// Left is how a pane stands once its agent was asked to leave.
type Left int

const (
	LeftShell  Left = iota // the agent is gone and its shell is back
	LeftGone               // the pane closed with the agent: no shell under it
	LeftStill              // the agent did not leave
	LeftOther              // something else runs in the pane now
	LeftUnseen             // herdr never answered: nothing is known
)

// LeavePoll is how often the pane is looked at while its agent leaves: twelve looks for its exit, eight more after a
// Ctrl-C; a var so the tests need not wait seconds.
var LeavePoll = time.Second

// Leave asks the pane's agent to go with its own exit and waits for the shell under it, Ctrl-C twice when it lingers;
// typed hears what it typed into the pane. An error is herdr refusing the exit or the keys.
func Leave(client Leaver, paneID, kind string, typed func(string), say func(string)) (Left, error) {
	a, ok := agents[kind]
	if !ok {
		return LeftStill, fmt.Errorf("remuda does not know how %s leaves", kind)
	}
	if err := client.AgentPrompt(paneID, a.exit); err != nil {
		return LeftStill, fmt.Errorf("%s: %w", a.exit, err)
	}
	if typed != nil {
		typed(a.exit)
	}
	state := waitShell(client, paneID, kind, 12)
	if state == LeftStill {
		say("  still there — Ctrl-C twice")
		if err := client.PaneSendKeys(paneID, []string{"ctrl+c", "ctrl+c"}); err != nil {
			return LeftStill, fmt.Errorf("Ctrl-C: %w", err)
		}
		state = waitShell(client, paneID, kind, 8)
	}
	return state, nil
}

// waitShell looks at the pane until it has no agent, is gone or runs something else; LeftStill after that many looks.
func waitShell(client Leaver, paneID, kind string, looks int) Left {
	seen := false
	for range looks {
		time.Sleep(LeavePoll)
		snap, err := client.SessionSnapshot()
		if err != nil {
			continue
		}
		seen = true
		p, found := Lookup(snap, paneID)
		switch {
		case !found:
			return LeftGone
		case p.Agent == "":
			return LeftShell
		case p.Agent != kind:
			return LeftOther
		}
	}
	if !seen {
		return LeftUnseen
	}
	return LeftStill
}

// SessionOf is the pane's session id, or "" for one that does not look like one (sessionOf).
func SessionOf(p *herdr.PaneInfo) string { return sessionOf(p) }

// Knows says whether remuda can make an agent of this kind leave and bring it back on its session.
func Knows(kind string) bool {
	_, ok := agents[kind]
	return ok
}
