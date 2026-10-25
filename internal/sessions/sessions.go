// Package sessions is remuda's input side of herdr: the agents it offers to
// start, and how text and keys reach a pane, one pane's traffic at a time.
package sessions

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/watch"
)

// Agent is one kind remuda offers to start, by herdr manifest kind; the page is served the same list.
type Agent struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

var Agents = []Agent{
	{"claude", "Claude Code"}, {"codex", "Codex"}, {"gemini", "Gemini"}, {"kimi", "Kimi Code"}, {"pi", "pi"},
	{"opencode", "OpenCode"}, {"cursor", "Cursor"}, {"amp", "Amp"}, {"grok", "Grok"}, {"qwen", "Qwen"},
}

// KnownAgent accepts a kind remuda starts, or "shell" / "" for no agent.
func KnownAgent(kind string) bool {
	if kind == "" || kind == "shell" {
		return true
	}
	for _, agent := range Agents {
		if agent.ID == kind {
			return true
		}
	}
	return false
}

// RefOK is a branch or base name git gets as an argument: never something it would read as an option, no spaces or control characters.
func RefOK(name string) bool {
	if name == "" || strings.HasPrefix(name, "-") || strings.Contains(name, "..") {
		return false
	}
	for _, r := range name {
		if r <= ' ' || r == 0x7f {
			return false
		}
	}
	return true
}

// Starter is what starting an agent needs of herdr's client.
type Starter interface {
	AgentStart(paneID, kind string) error
	AgentStartWith(paneID, kind, name string, args []string, timeout time.Duration) error
}

// Start is the person's choices for an agent about to start: an earlier conversation to come back to, a model, arguments of their own.
type Start struct {
	Resume string   // a session id of the folder's, "" for a fresh conversation
	Model  string   // the model as the agent's own CLI names it, "" for its default
	Args   []string // extra arguments, as the person wrote them
}

// StartAgent starts an agent in a pane whose shell may still be coming up, retrying herdr's busy answer for a few seconds.
func StartAgent(client Starter, paneID, kind string) error {
	return StartAgentWith(client, paneID, kind, Start{})
}

// StartAgentResumed starts an agent on one of its earlier conversations: the kind's own resume arguments after the session id.
func StartAgentResumed(client Starter, paneID, kind, session string) error {
	return StartAgentWith(client, paneID, kind, Start{Resume: session})
}

// StartAgentWith starts an agent with the person's choices: the resume arguments first, then the model's, then their own.
func StartAgentWith(client Starter, paneID, kind string, start Start) error {
	args, err := StartArgs(kind, start)
	if err != nil {
		return err
	}
	if args == nil {
		return startAgent(func() error { return client.AgentStart(paneID, kind) })
	}
	return startAgent(func() error { return client.AgentStartWith(paneID, kind, kind, args, 30*time.Second) })
}

// StartArgs is the command line a Start comes to for a kind, nil when it asks for nothing beyond the agent's defaults.
func StartArgs(kind string, start Start) ([]string, error) {
	var args []string
	if start.Resume != "" {
		resume, ok := ResumeArgs(kind, start.Resume)
		if !ok {
			return nil, fmt.Errorf("%s cannot resume a session", kind)
		}
		args = append(args, resume...)
	}
	if start.Model != "" {
		model, ok := ModelArgs(kind, start.Model)
		if !ok {
			return nil, fmt.Errorf("%s has no model flag remuda knows", kind)
		}
		args = append(args, model...)
	}
	args = append(args, start.Args...)
	return args, nil
}

func startAgent(start func() error) error {
	deadline := time.Now().Add(12 * time.Second)
	for {
		err := start()
		if err == nil || !(herdr.IsCode(err, "agent_pane_busy") || herdr.IsCode(err, "agent_not_ready")) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// resumeFlags is how each agent is told which conversation to come back to; the id goes last, as the agent's own CLI wants it.
var resumeFlags = map[string][]string{
	"claude": {"--resume"},
	"codex":  {"resume"},
	"gemini": {"--resume"},
	"kimi":   {"--session"},
}

// ResumeArgs is the agent's command-line arguments that resume a session, false for a kind that has none.
func ResumeArgs(kind, session string) ([]string, bool) {
	flags, ok := resumeFlags[kind]
	if !ok {
		return nil, false
	}
	return append(append([]string(nil), flags...), session), true
}

// modelFlags is how each agent is told which model to run; the name goes right after.
var modelFlags = map[string]string{
	"claude": "--model",
	"codex":  "--model",
	"gemini": "--model",
	"kimi":   "--model",
}

// ModelArgs is the agent's command-line arguments that pick a model, false for a kind whose flag remuda does not know.
func ModelArgs(kind, model string) ([]string, bool) {
	flag, ok := modelFlags[kind]
	if !ok {
		return nil, false
	}
	return []string{flag, model}, true
}

// SplitArgs reads what a person typed as arguments the way a POSIX shell would: whitespace separates, double and single
// quotes group, a backslash escapes the next character outside single quotes. A control character or an unclosed quote is an error.
func SplitArgs(input string) ([]string, error) {
	var args []string
	var word strings.Builder
	inWord, quote, escaped := false, rune(0), false
	for _, r := range input {
		switch {
		case (r < ' ' && r != '\t' && r != '\n') || r == 0x7f:
			return nil, errors.New("arguments cannot hold control characters")
		case escaped:
			word.WriteRune(r)
			escaped = false
		case quote == '\'' && r == '\'':
			quote = 0
		case quote == '\'':
			word.WriteRune(r)
		case r == '\\':
			escaped, inWord = true, true
		case quote == '"' && r == '"':
			quote = 0
		case quote == '"':
			word.WriteRune(r)
		case r == '"' || r == '\'':
			quote, inWord = r, true
		case r == ' ' || r == '\t' || r == '\n':
			if inWord {
				args = append(args, word.String())
				word.Reset()
				inWord = false
			}
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 || escaped {
		return nil, errors.New("arguments end inside a quote")
	}
	if inWord {
		args = append(args, word.String())
	}
	return args, nil
}

// Prompter is what delivering text and keys needs of herdr's client.
type Prompter interface {
	AgentPrompt(target, text string) error
	PaneSendText(paneID, text string) error
	PaneSendKeys(paneID string, keys []string) error
}

// ErrAgentBusy: the agent shows a menu; the message must not land in it.
var ErrAgentBusy = errors.New("the agent is waiting for an answer — answer its prompt first")

// allowedKeys is herdr's pane.send_keys grammar as observed live: lowercase names, "+" for modifiers.
var allowedKeys = map[string]bool{
	"enter": true, "esc": true, "tab": true, "shift+tab": true, "space": true,
	"up": true, "down": true, "left": true, "right": true,
	"home": true, "end": true, "pageup": true, "pagedown": true,
	"backspace": true, "delete": true,
	"ctrl+c": true, "ctrl+d": true, "ctrl+u": true, "ctrl+z": true,
	"ctrl+r": true, "ctrl+l": true, "ctrl+k": true,
}

// Keys lowercases the names and refuses one herdr's grammar has no word for.
func Keys(names []string) ([]string, error) {
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.ToLower(name)
		if !allowedKeys[name] {
			return nil, fmt.Errorf("key %q not allowed", name)
		}
		out = append(out, name)
	}
	return out, nil
}

// Input is the one door for what people type into panes: every delivery holds the pane's lock, so nothing interleaves.
type Input struct {
	Machines *machines.Manager
	Watch    *watch.Watcher

	mu    sync.Mutex
	locks map[events.PaneRef]*sync.Mutex
}

func NewInput(m *machines.Manager, w *watch.Watcher) *Input {
	return &Input{Machines: m, Watch: w, locks: map[events.PaneRef]*sync.Mutex{}}
}

// Lock is the pane's typing lock; a restart holds it from the exit to the resumed agent.
func (in *Input) Lock(machineID, paneID string) sync.Locker {
	return in.lock(events.PaneRef{Machine: machineID, Pane: paneID})
}

func (in *Input) lock(ref events.PaneRef) *sync.Mutex {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.locks[ref] == nil {
		in.locks[ref] = &sync.Mutex{}
	}
	return in.locks[ref]
}

// Submit delivers text to a pane the way the chat does: herdr's own submit for an agent, typing like a keyboard for a shell.
func (in *Input) Submit(client Prompter, ref events.PaneRef, text string) error {
	lock := in.lock(ref)
	lock.Lock()
	defer lock.Unlock()
	err := client.AgentPrompt(ref.Pane, text)
	switch {
	case err == nil:
		return nil
	case herdr.IsCode(err, "agent_blocked"):
		return ErrAgentBusy
	case herdr.IsCode(err, "agent_not_found") || herdr.IsCode(err, "agent_not_ready"):
		// the settle gap before Enter: a TUI still redrawing would take it as a newline
		if err := client.PaneSendText(ref.Pane, text); err != nil {
			return err
		}
		time.Sleep(150 * time.Millisecond)
		return client.PaneSendKeys(ref.Pane, []string{"enter"})
	default:
		return err
	}
}

// FirstPromptRetry and FirstPromptTimeout pace the delivery of a first prompt: a try every so often until the agent takes it.
var (
	FirstPromptRetry   = 2 * time.Second
	FirstPromptTimeout = 2 * time.Minute
)

// FirstPrompt hands the text a person wrote for an agent they just started over once it is up: agent.prompt under the pane's
// lock, tried again every FirstPromptRetry while the agent is still coming up, until it takes the text or the timeout passes.
// A dialog on the agent's screen (Claude Code's trust question for the folder, which is the person's to answer) is waited
// through, never answered; the error says why the text was not delivered.
func (in *Input) FirstPrompt(client Prompter, ref events.PaneRef, text string, timeout time.Duration) error {
	lock := in.lock(ref)
	deadline := time.Now().Add(timeout)
	var last error
	for {
		time.Sleep(FirstPromptRetry)
		lock.Lock()
		err := client.AgentPrompt(ref.Pane, text)
		lock.Unlock()
		switch {
		case err == nil:
			return nil
		case herdr.IsCode(err, "agent_not_ready") || herdr.IsCode(err, "agent_not_found") || herdr.IsCode(err, "agent_blocked"):
			last = err
		default:
			return err
		}
		if time.Now().After(deadline) {
			break
		}
	}
	if herdr.IsCode(last, "agent_blocked") {
		return errors.New("the agent asks something first (its dialog is on the screen); answer it and send the text yourself")
	}
	return fmt.Errorf("the agent did not come up within %s (%v)", timeout.Round(time.Second), last)
}

// SendKeys presses named keys in a pane; the names went through Keys.
func (in *Input) SendKeys(client Prompter, ref events.PaneRef, keys []string) error {
	lock := in.lock(ref)
	lock.Lock()
	defer lock.Unlock()
	return client.PaneSendKeys(ref.Pane, keys)
}

// SubmitToFolder gives the text to the agent running in a machine's folder, the scheduler's door; the pane's title names where it went.
func (in *Input) SubmitToFolder(machineID, dir, text string) (string, error) {
	mc, err := in.Machines.Get(machineID)
	if err != nil {
		return "", err
	}
	client, _, err := mc.Ready()
	if err != nil {
		return "", err
	}
	snap, _ := in.Watch.Snapshot(machineID)
	if snap == nil {
		return "", fmt.Errorf("%s has no snapshot yet", mc.Label())
	}
	pane := snap.AgentPaneIn(dir)
	if pane == nil {
		return "", fmt.Errorf("no agent is running in %s on %s", dir, mc.Label())
	}
	if err := in.Submit(client, events.PaneRef{Machine: machineID, Pane: pane.PaneID}, text); err != nil {
		return "", err
	}
	return pane.DisplayTitle(), nil
}
