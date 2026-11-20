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

// promptsNeed is the hello feature of a remuda-core that, as it types a send, checks the program in front of the
// terminal is still the one named (`foreground`, Core 33). A prompt goes to an agent only through such a core: an agent
// that left a moment before would otherwise have the prompt typed into its shell, which would run it. A test lifts it.
var promptsNeed = "send_foreground"

// sendWithin bounds a prompt: the core holds it while the person finishes a draft in the terminal (60 s at most since
// their last key) and then watches its turn start (5 s). A shorter bound would cancel a prompt typed already, and the
// person would send it twice.
const sendWithin = 75 * time.Second

// startSeen is how long a started agent's command may leave the shell at its prompt before the start counts as failed.
const startSeen = 10 * time.Second

// AgentStart starts an agent of a kind in the pane's shell.
func (a *Adapter) AgentStart(paneID, kind string) error {
	return a.AgentStartWith(paneID, kind, kind, nil, 30*time.Second)
}

// AgentStartWith types the agent's command line into the pane's shell, at its prompt, as herdr's agent.start runs it:
// the shell stays under the agent and comes back when it leaves. A shell not at its prompt yet is agent_pane_busy, which
// the caller tries again. The name is herdr's to keep agents apart; the core needs none.
func (a *Adapter) AgentStartWith(paneID, kind, name string, args []string, timeout time.Duration) error {
	program, ok := herdr.AgentCommand(kind)
	if !ok {
		return &herdr.Error{Code: "agent_kind_unknown", Message: fmt.Sprintf("no agent %q", kind)}
	}
	t, err := a.terminal(paneID)
	if err != nil {
		return err
	}
	if !atPrompt(&t) {
		return &herdr.Error{Code: "agent_pane_busy", Message: paneID + " is not at its shell's prompt"}
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
// POSIX quoting for every other shell. A word made only of letters, digits and _ . / : = + - stays as it is. Windows
// PowerShell 5.1 drops the double quotes inside an argument it hands a program (pwsh 7.3 and up passes them), so there
// they go escaped as the program reads them, \".
func commandLine(shell string, words []string) string {
	quote := script.Quote
	switch shell {
	case "pwsh":
		quote = func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	case "powershell":
		quote = func(s string) string {
			return "'" + strings.ReplaceAll(strings.ReplaceAll(s, "'", "''"), `"`, `\"`) + "'"
		}
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
// that named it counts), or, one running before the mirror looked, it has a status. An agent that reports none never
// is: the core reads no dialog off the screen, and an Enter typed into one could approve it.
func readyForPrompt(t *Terminal) bool {
	if t.agentAt.IsZero() {
		return t.Status != nil
	}
	return !t.statusAt.IsZero() && !t.statusAt.Before(t.agentAt)
}

// AgentPrompt hands the agent in the pane a message, only through a core that checks the agent is still in front as it
// types (promptsNeed, the agent's pid named as `foreground`): the core pastes it and presses Enter, and refuses while
// the agent asks something (agent_blocked). A pane whose agent has not come up, or that has none, gets nothing: what is written
// for an agent never reaches a shell. The core's own view of the terminal is read right before, so an agent that left
// since the mirror's last news is not taken for there; an agent whose status is cleared (Claude Code reports
// state=clear on its way out) gets nothing, and a send to an idle agent names that status (`if`), so one that clears
// it as the prompt goes has it refused too (status_mismatch).
func (a *Adapter) AgentPrompt(target, text string) error {
	t, err := a.terminal(target)
	if err != nil {
		return err
	}
	agent, err := agentIn(&t, target)
	if err != nil {
		return err
	}
	if !readyForPrompt(&t) {
		return &herdr.Error{Code: "agent_not_ready", Message: "the agent in " + target + " has reported no status yet (one that never does takes no prompt on remuda-core)"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), sendWithin)
	defer cancel()
	cn, err := a.client.connection(ctx)
	if err != nil {
		return a.refusal(err, "agent_not_found")
	}
	if promptsNeed != "" && !cn.hello.Has(promptsNeed) {
		return &herdr.Error{Code: "invalid_request", Message: "a prompt to an agent waits for a remuda-core that checks, as it types, that the agent is still in front (Core 33)"}
	}
	now, err := a.client.Get(ctx, target)
	if err != nil {
		return a.refusal(err, "agent_not_found")
	}
	if agentOf(&now) != agent {
		return &herdr.Error{Code: "agent_not_found", Message: "the agent in " + target + " has left"}
	}
	if now.Status == nil {
		return &herdr.Error{Code: "agent_not_ready", Message: "the agent in " + target + " cleared its status, as one leaving does: nothing was sent"}
	}
	if now.Foreground == nil {
		return &herdr.Error{Code: "agent_not_ready", Message: "remuda-core has not seen what runs in front of " + target + " yet"}
	}
	params := map[string]any{"terminal": target, "text": text, "foreground": now.Foreground.Pid}
	if statusOf(&now) == herdr.StatusIdle {
		params["if"] = herdr.StatusIdle // not working: a turn ending as the prompt goes would refuse it
	}
	err = a.client.Call(ctx, "terminal.send", params, nil)
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
	case IsKind(err, "status_mismatch"):
		return &herdr.Error{Code: notReady, Message: "the agent moved as the prompt went, nothing was sent: " + err.Error()}
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

// keyBytes are the bytes sessions types for the keys herdr 0.9.5 has no name for; the core names them, and encodes each
// for the program's keyboard modes.
var keyBytes = map[string]string{"\x1bOH": "home", "\x1bOF": "end", "\x1b[5~": "pageup", "\x1b[6~": "pagedown", "\x1b[3~": "delete"}

// PaneSendText types text as it is, byte for byte, into a terminal its shell holds — a command line remuda types into a
// shell it just opened — through a viewer of the terminal's own that leaves its size alone, since the core takes text
// only as a paste. It goes as the terminal's automatic input, starting no draft the core would hold a prompt back for.
// Into a pane an agent holds it types nothing (agent_pane_busy): a person's typing there comes with the live terminal,
// and remuda's own would be pasted ahead of the agent's next prompt.
func (a *Adapter) PaneSendText(paneID, text string) error {
	if key, ok := keyBytes[text]; ok {
		return a.PaneSendKeys(paneID, []string{key})
	}
	t, err := a.terminal(paneID)
	if err != nil {
		return err
	}
	if agentOf(&t) != "" {
		return &herdr.Error{Code: "agent_pane_busy", Message: "typing into an agent's terminal on remuda-core comes with its live terminal"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	stream, err := a.client.Attach(ctx, AttachParams{Terminal: paneID, Cols: t.Cols, Rows: t.Rows, KeepSize: true})
	if err != nil {
		return a.refusal(err, "pane_not_found")
	}
	defer stream.Close()
	_, err = stream.WriteAutomatic([]byte(text))
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
