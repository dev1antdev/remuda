package core

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/script"
)

// sendWithin bounds a prompt: the core holds it while the person finishes a draft in the terminal (60 s at most since
// their last key) and then watches its turn start (5 s).
const sendWithin = 75 * time.Second

// unreportedReady is how long an agent that reports no status holds its terminal before a prompt goes to it anyway: one
// that reports is ready once it has said something since it started.
const unreportedReady = 10 * time.Second

// startSeen is how long a started agent's command may leave the shell at its prompt before the start counts as failed.
const startSeen = 10 * time.Second

// agentPrograms are the commands agent kinds start as, where the command is not the kind's own name.
var agentPrograms = map[string]string{"cursor": "cursor-agent"}

var kindRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// AgentStart starts an agent of a kind in the pane's shell.
func (a *Adapter) AgentStart(paneID, kind string) error {
	return a.AgentStartWith(paneID, kind, kind, nil, 30*time.Second)
}

// AgentStartWith types the agent's command line into the pane's shell, at its prompt, as herdr's agent.start runs it:
// the shell stays under the agent and comes back when it leaves. A shell not at its prompt yet is agent_pane_busy, which
// the caller tries again. The name is herdr's to keep agents apart; the core needs none.
func (a *Adapter) AgentStartWith(paneID, kind, name string, args []string, timeout time.Duration) error {
	if !kindRe.MatchString(kind) {
		return &herdr.Error{Code: "agent_kind_unknown", Message: fmt.Sprintf("no agent %q", kind)}
	}
	t, err := a.terminal(paneID)
	if err != nil {
		return err
	}
	if !atPrompt(&t) {
		return &herdr.Error{Code: "agent_pane_busy", Message: paneID + " is not at its shell's prompt"}
	}
	program := kind
	if p, ok := agentPrograms[kind]; ok {
		program = p
	}
	line := commandLine(programName(t.Foreground.Name), append([]string{program}, args...))
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if _, err := a.client.Send(ctx, paneID, line); err != nil {
		return a.inputRefusal(err, "agent_pane_busy", "agent_pane_busy")
	}
	return a.waitForStart(ctx, paneID, kind)
}

// waitForStart waits until something the command started holds the terminal, or the shell is still at its prompt
// startSeen later (a command it could not run).
func (a *Adapter) waitForStart(ctx context.Context, paneID, kind string) error {
	deadline := time.Now().Add(startSeen)
	for {
		t, err := a.terminal(paneID)
		if err != nil {
			return err
		}
		if !atPrompt(&t) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s did not start in %s: its shell is still at its prompt", kind, paneID)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s did not start in %s: %w", kind, paneID, ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// commandLine is a command's words as the shell at the prompt reads them: PowerShell's and cmd's quoting on Windows,
// POSIX quoting for every other shell. A word made only of letters, digits and _ . / : = + - stays as it is.
func commandLine(shell string, words []string) string {
	quote := script.Quote
	switch shell {
	case "pwsh", "powershell":
		quote = func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	case "cmd":
		quote = func(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
	}
	quoted := make([]string, len(words))
	for i, word := range words {
		if plainWord.MatchString(word) {
			quoted[i] = word
		} else {
			quoted[i] = quote(word)
		}
	}
	return strings.Join(quoted, " ")
}

var plainWord = regexp.MustCompile(`^[A-Za-z0-9_./:=+-]+$`)

// readyForPrompt says an agent has come up for a prompt: it reported a status since it took the terminal (the report
// that named it counts), or it reports none and has held the terminal unreportedReady.
func readyForPrompt(t *Terminal, now time.Time) bool {
	reported := !t.statusAt.IsZero() && !t.statusAt.Before(t.agentAt)
	return t.agentAt.IsZero() || reported || now.Sub(t.agentAt) >= unreportedReady
}

// AgentPrompt hands the agent in the pane a message: the core pastes it and presses Enter once the person is done
// with a draft in the terminal, and refuses while the agent asks something (agent_blocked). A pane whose agent has not
// come up, or that has none, gets nothing: what is written for an agent never reaches a shell.
func (a *Adapter) AgentPrompt(target, text string) error {
	t, err := a.terminal(target)
	if err != nil {
		return err
	}
	if agentOf(&t) == "" {
		return &herdr.Error{Code: "agent_not_found", Message: "no agent in " + target}
	}
	if !readyForPrompt(&t, time.Now()) {
		return &herdr.Error{Code: "agent_not_ready", Message: "the agent in " + target + " has not come up yet"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), sendWithin)
	defer cancel()
	_, err = a.client.Send(ctx, target, text)
	return a.inputRefusal(err, "agent_not_ready", "agent_blocked")
}

// inputRefusal is the core's refusal of typed input in herdr's words: a terminal still starting or between shells is
// notReady, a program asking something is blocked, one gone is pane_not_found.
func (a *Adapter) inputRefusal(err error, notReady, blocked string) error {
	switch {
	case IsKind(err, "unavailable"):
		return &herdr.Error{Code: notReady, Message: err.Error()}
	case IsKind(err, "blocked"):
		return &herdr.Error{Code: blocked, Message: err.Error()}
	case IsKind(err, "finished"):
		return &herdr.Error{Code: "pane_not_found", Message: err.Error()}
	}
	return a.refusal(err, "pane_not_found")
}

// PaneSendKeys presses named keys at once, as the person pressed them: Esc, an answer's arrows and Enter, Ctrl-C.
func (a *Adapter) PaneSendKeys(paneID string, keys []string) error {
	if _, err := a.terminal(paneID); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	_, err := a.client.Keys(ctx, paneID, keys, true)
	return a.inputRefusal(err, "pane_not_found", "pane_not_found")
}

// PaneSendText types text as it is, byte for byte: through a viewer of the terminal's own that leaves its size alone,
// since the core takes text only as a paste.
func (a *Adapter) PaneSendText(paneID, text string) error {
	t, err := a.terminal(paneID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	stream, err := a.client.Attach(ctx, AttachParams{Terminal: paneID, Cols: t.Cols, Rows: t.Rows, KeepSize: true})
	if err != nil {
		return a.refusal(err, "pane_not_found")
	}
	defer stream.Close()
	_, err = stream.Write([]byte(text))
	return a.refusal(err, "pane_not_found")
}

// PaneSendInput is the screen terminal's typing: the text, then the keys. A paste of several lines is refused, since
// typed raw every line would run; the live terminal brackets a paste for the program that asked.
func (a *Adapter) PaneSendInput(paneID, text string, keys []string) error {
	if strings.ContainsAny(text, "\r\n") {
		return notOnCore("a paste of several lines from the screen terminal")
	}
	if text != "" {
		if err := a.PaneSendText(paneID, text); err != nil {
			return err
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return a.PaneSendKeys(paneID, keys)
}
