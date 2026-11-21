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
// types (promptsNeed, the agent's pid named as `foreground`), on the very connection that said so — a connection lost
// meanwhile fails the prompt rather than dial a core that may not check: the core pastes it and presses Enter, and refuses while
// the agent asks something (agent_blocked). A pane whose agent has not come up, or that has none, gets nothing: what is written
// for an agent never reaches a shell. The core's own view of the terminal is read right before, so an agent that left
// since the mirror's last news is not taken for there; an agent whose status is cleared (Claude Code reports
// state=clear on its way out) gets nothing, and a send names the status it saw (`if`), so one that clears it or
// starts a turn as the prompt goes has it refused too (status_mismatch). It goes to an agent at rest alone (a
// restart's exit, a sleep's): see sendTo for the person's own message.
func (a *Adapter) AgentPrompt(target, text string) error {
	_, send, err := a.StagePrompt(target, text, 0, false)
	if err != nil {
		return err
	}
	return send()
}

// StagePrompt is AgentPrompt in two steps: the look at the agent now, and the send it answers, which the core may hold
// a minute while a person finishes a draft. sessions holds the pane's lock for the look alone, so the person's keys and
// answers pass a prompt the core holds; the core itself types nothing else between the paste and its Enter. It answers
// the pid of the agent it saw, which a delivery tried again names as front: any other agent in front is agent_left,
// so a prompt never reaches what replaced the one it was for. The pid comes with every refusal once an agent was seen
// in front — not ready yet, asking something, at work — so even a delivery no try reached is bound from its first
// sight of the agent. asMessage is the person's own message, which a turn started meanwhile takes as a message.
func (a *Adapter) StagePrompt(target, text string, front int, asMessage bool) (int, func() error, error) {
	t, err := a.terminal(target)
	if err != nil {
		return 0, nil, err
	}
	pid := pidIn(&t)
	if front != 0 && pid != 0 && pid != front {
		return 0, nil, agentLeft(target)
	}
	agent, err := agentIn(&t, target)
	if err != nil {
		return 0, nil, err
	}
	if front == 0 {
		front = pid // the agent this look first saw is the one it is for, to the end of the look
	}
	if !readyForPrompt(&t) {
		return pid, nil, &herdr.Error{Code: "agent_not_ready", Message: "the agent in " + target + " has reported no status yet (one that never does takes no prompt on remuda-core)"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	cn, err := a.client.connection(ctx)
	if err != nil {
		return pid, nil, a.refusal(err, "agent_not_found")
	}
	if promptsNeed != "" && !cn.hello.Has(promptsNeed) {
		return pid, nil, &herdr.Error{Code: "invalid_request", Message: "a prompt to an agent waits for a remuda-core that checks, as it types, that the agent is still in front (Core 33)"}
	}
	var now Terminal
	if err := cn.call(ctx, "terminal.get", map[string]any{"terminal": target}, &now, nil); err != nil {
		return pid, nil, a.refusal(err, "agent_not_found")
	}
	if pidIn(&now) != 0 {
		pid = pidIn(&now)
	}
	if front != 0 && pid != 0 && pid != front {
		return 0, nil, agentLeft(target)
	}
	if agentOf(&now) != agent {
		return pid, nil, &herdr.Error{Code: "agent_not_found", Message: "the agent in " + target + " has left"}
	}
	if now.Status == nil {
		return pid, nil, &herdr.Error{Code: "agent_not_ready", Message: "the agent in " + target + " cleared its status, as one leaving does: nothing was sent"}
	}
	if now.Foreground == nil {
		return pid, nil, &herdr.Error{Code: "agent_not_ready", Message: "remuda-core has not seen what runs in front of " + target + " yet"}
	}
	switch statusOf(&now) {
	case herdr.StatusBlocked:
		return pid, nil, &herdr.Error{Code: "agent_blocked", Message: "the agent in " + target + " asks something: answer it first, nothing was sent"}
	case herdr.StatusWorking:
		if !asMessage {
			return pid, nil, agentWorking(target)
		}
	}
	call := func(ctx context.Context, method string, params, result any) error {
		return cn.call(ctx, method, params, result, nil)
	}
	return pid, func() error { return a.sendTo(call, target, text, agent, now, asMessage) }, nil
}

// caller is one call to the core, the connection the look was made on.
type caller func(ctx context.Context, method string, params, result any) error

// sendTo is the send a look answers. One the core refused untyped (status_mismatch: the status the send named moved,
// or another program took the front) is read again: the same agent, in front under the same pid with a status, gets
// one more send as it stands now — at rest again after a short turn, or, asMessage, a turn the person's own draft
// started, which takes it as a message; anything else is agent_left. A turn started meanwhile refuses everyone else's.
func (a *Adapter) sendTo(call caller, target, text, agent string, now Terminal, asMessage bool) error {
	err := sendAs(call, target, text, &now, asMessage)
	if IsKind(err, "status_mismatch") {
		again, left := a.stillThere(call, target, agent, now.Foreground.Pid)
		if left != nil {
			return left
		}
		if !asMessage && statusOf(&again) == herdr.StatusWorking {
			return agentWorking(target)
		}
		err = sendAs(call, target, text, &again, asMessage)
	}
	return a.inputRefusal(err, "agent_not_ready", "agent_blocked")
}

// sendAs is the send naming the status it saw unless the agent works (`if`), so a send that waited while the agent
// moved — cleared its status on its way out, or started a turn a person's draft set off — is refused untyped
// (status_mismatch); only the person's own message goes to an agent at work, naming nothing, since a turn takes it
// as a message and one ending as it goes would refuse it. A core whose `if` names idle and working alone (before Core
// 41's send_if_any_status) refuses any other word before it types (invalid_params): the person's message then goes
// without it, and a prompt for the agent at rest — done, say — is refused, since nothing would keep it out of a turn a
// person's draft started.
func sendAs(call caller, target, text string, now *Terminal, asMessage bool) error {
	state := now.Status.State
	named := state != "" && statusOf(now) != herdr.StatusWorking
	err := sendPrompt(call, target, text, now, named)
	if named && state != herdr.StatusIdle && IsKind(err, "invalid_params") {
		if !asMessage {
			return &herdr.Error{Code: "invalid_request", Message: "this remuda-core holds a prompt only for an agent idle or at work, not " + state + ": nothing was sent to " + target}
		}
		err = sendPrompt(call, target, text, now, false)
	}
	return err
}

// sendPrompt is terminal.send to the agent in front under its pid, naming its status as `if` when named.
func sendPrompt(call caller, target, text string, now *Terminal, named bool) error {
	params := map[string]any{"terminal": target, "text": text, "foreground": now.Foreground.Pid}
	if named {
		params["if"] = now.Status.State
	}
	ctx, cancel := context.WithTimeout(context.Background(), sendWithin)
	defer cancel()
	return call(ctx, "terminal.send", params, nil)
}

// stillThere is the terminal read again after a send the core refused untyped: agent_left unless the same agent leads
// it under the same pid with a status.
func (a *Adapter) stillThere(call caller, target, agent string, pid int) (Terminal, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	var now Terminal
	if err := call(ctx, "terminal.get", map[string]any{"terminal": target}, &now); err != nil {
		return now, a.refusal(err, "agent_not_found")
	}
	if agentOf(&now) != agent || now.Foreground == nil || now.Foreground.Pid != pid || now.Status == nil {
		return now, agentLeft(target)
	}
	return now, nil
}

// pidIn is the pid in front of the terminal, 0 when none was seen there.
func pidIn(t *Terminal) int {
	if t.Foreground == nil {
		return 0
	}
	return t.Foreground.Pid
}

// agentWorking refuses a prompt for an agent at rest to one at work: it waits for the agent's next rest.
func agentWorking(target string) error {
	return &herdr.Error{Code: "agent_working", Message: "the agent in " + target + " is at work, and this prompt is for it at rest: nothing was sent"}
}

func agentLeft(target string) error {
	return &herdr.Error{Code: "agent_left", Message: "the agent in " + target + " left the terminal's front as the prompt went: nothing was sent"}
}

// inputRefusal is the core's refusal of typed input in herdr's words: a terminal still starting or between shells is
// notReady, a program asking something is blocked, one gone is pane_not_found. A send the core pasted and then left
// unsubmitted, the agent gone from the front before its Enter, keeps the core's not_submitted, the text maybe still
// in the terminal's input line; one through a terminal whose holder is older (needs_update) is invalid_request.
func (a *Adapter) inputRefusal(err error, notReady, blocked string) error {
	switch {
	case IsKind(err, "unavailable"):
		return &herdr.Error{Code: notReady, Message: err.Error()}
	case IsKind(err, "blocked"):
		return &herdr.Error{Code: blocked, Message: err.Error()}
	case IsKind(err, "status_mismatch"):
		return &herdr.Error{Code: notReady, Message: "the agent moved as the prompt went, nothing was sent: " + err.Error()}
	case IsKind(err, "not_submitted"):
		return &herdr.Error{Code: "not_submitted", Message: "the prompt was pasted but not submitted, look at the terminal: " + err.Error()}
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
