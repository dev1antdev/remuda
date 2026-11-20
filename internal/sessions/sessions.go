// Package sessions is remuda's input side of herdr: the agents it offers to
// start, and how text and keys reach a pane, one pane's traffic at a time.
package sessions

import (
	"errors"
	"fmt"
	"regexp"
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

// Start is the person's choices for an agent about to start: an earlier conversation to come back to, a model, arguments
// of their own, and whether it denies what would wait on a permission prompt (an agent nobody watches).
type Start struct {
	Resume      string   // a session id of the folder's, "" for a fresh conversation
	Fork        bool     // with Resume: start on a copy of that conversation, the original left as it is
	Model       string   // the model as the agent's own CLI names it, "" for its default
	Args        []string // extra arguments, as the person wrote them
	DenyPrompts bool     // a call that would wait on a permission prompt is denied and the turn carries on (CanDenyPrompts)
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
// herdr names an agent by its kind and refuses a second of that name, so a taken name is tried once more as the kind
// with the pane's id, the way the restarts name theirs: a second Claude Code, or an agent on each of ten tasks, can run.
func StartAgentWith(client Starter, paneID, kind string, start Start) error {
	args, err := StartArgs(kind, start)
	if err != nil {
		return err
	}
	try := func(name string) error {
		if args == nil && name == kind {
			return client.AgentStart(paneID, kind)
		}
		list := args
		if list == nil {
			list = []string{}
		}
		return client.AgentStartWith(paneID, kind, name, list, 30*time.Second)
	}
	err = startAgent(func() error { return try(kind) })
	if herdr.IsCode(err, "agent_name_taken") {
		err = startAgent(func() error { return try(UniqueName(kind, paneID)) })
	}
	return err
}

var nameRe = regexp.MustCompile(`[^a-z0-9_-]+`)

// UniqueName is an agent's name when its kind's is taken: the kind and the pane's id, "codex-w1-p3", at most 32 characters.
func UniqueName(kind, paneID string) string {
	n := kind + "-" + nameRe.ReplaceAllString(strings.ToLower(paneID), "-")
	if len(n) > 32 {
		n = n[:32]
	}
	return strings.TrimRight(n, "-")
}

// StartArgs is the command line a Start comes to for a kind, nil when it asks for nothing beyond the agent's defaults:
// the resume arguments, the model's, then the person's own with the deny setting merged in (withDenySetting).
func StartArgs(kind string, start Start) ([]string, error) {
	var args []string
	switch {
	case start.Resume != "" && start.Fork:
		fork, ok := ForkArgs(kind, start.Resume)
		if !ok {
			return nil, fmt.Errorf("%s cannot fork a conversation", kind)
		}
		args = append(args, fork...)
	case start.Resume != "":
		resume, ok := ResumeArgs(kind, start.Resume)
		if !ok {
			return nil, fmt.Errorf("%s cannot resume a session", kind)
		}
		args = append(args, resume...)
	case start.Fork:
		return nil, fmt.Errorf("a fork needs the session to copy")
	}
	if start.Model != "" {
		model, ok := ModelArgs(kind, start.Model)
		if !ok {
			return nil, fmt.Errorf("%s has no model flag remuda knows", kind)
		}
		args = append(args, model...)
	}
	own := start.Args
	if start.DenyPrompts {
		if !CanDenyPrompts(kind) {
			return nil, fmt.Errorf("%s cannot be told to deny what would wait on a permission prompt", kind)
		}
		var err error
		if own, err = withDenySetting(own); err != nil {
			return nil, err
		}
	}
	args = append(args, own...)
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

// forkFlags is how each agent starts on a copy of a conversation, as its own CLI documents it: the arguments before
// the session id and after it; a kind missing here cannot fork (Gemini CLI and Kimi Code document no fork).
var forkFlags = map[string]struct{ before, after []string }{
	"claude": {before: []string{"--resume"}, after: []string{"--fork-session"}},
	"codex":  {before: []string{"fork"}},
}

// ForkArgs is the agent's command-line arguments that start it on a copy of a session, false for a kind that cannot.
func ForkArgs(kind, session string) ([]string, bool) {
	flags, ok := forkFlags[kind]
	if !ok {
		return nil, false
	}
	args := append(append([]string(nil), flags.before...), session)
	return append(args, flags.after...), true
}

// CanFork says whether remuda knows how to start the kind on a copy of a conversation.
func CanFork(kind string) bool {
	_, ok := forkFlags[kind]
	return ok
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

// StagedPrompter is a client whose prompt is looked at first and typed after, maybe a minute later: remuda-core holds a
// prompt while a person finishes a draft in the terminal, and checks as it types that the agent is still in front and
// that nothing else's input interleaves. The look needs the pane's lock; the wait does not, so the person's keys and
// answers pass a prompt the core holds. The look answers the agent's pid, with a refusal of that agent too (it asks
// something, or works), which a delivery tried again names as front:
// any other agent in front is refused (agent_left); asMessage, the person's own message, goes to a turn started
// meanwhile, everyone else's to an agent at rest alone.
type StagedPrompter interface {
	StagePrompt(target, text string, front int, asMessage bool) (pid int, send func() error, err error)
}

// Input is the one door for what people type into panes: every delivery holds the pane's lock, so nothing interleaves.
// A staged prompt holds it for its look alone, and the pane's prompts go one after another by a lock of their own.
type Input struct {
	Machines *machines.Manager
	Watch    *watch.Watcher

	mu      sync.Mutex
	locks   map[events.PaneRef]*sync.Mutex
	prompts map[events.PaneRef]*sync.Mutex
}

func NewInput(m *machines.Manager, w *watch.Watcher) *Input {
	return &Input{Machines: m, Watch: w, locks: map[events.PaneRef]*sync.Mutex{}, prompts: map[events.PaneRef]*sync.Mutex{}}
}

// Lock is the pane's typing lock; a restart holds it from the exit to the resumed agent.
func (in *Input) Lock(machineID, paneID string) sync.Locker {
	return in.lock(events.PaneRef{Machine: machineID, Pane: paneID})
}

// Hold is the pane's whole hold, a restart's or a sleep's from an agent's exit to the next one's start: the prompts'
// lock, then the typing lock, as Prompt takes them, so no prompt looked at before goes in between either. An answer
// takes the typing lock alone (Lock), and so passes a prompt remuda-core holds.
func (in *Input) Hold(machineID, paneID string) sync.Locker {
	ref := events.PaneRef{Machine: machineID, Pane: paneID}
	return &held{prompts: in.paneMutex(in.prompts, ref), typing: in.lock(ref)}
}

type held struct{ prompts, typing *sync.Mutex }

func (h *held) Lock() {
	h.prompts.Lock()
	h.typing.Lock()
}

func (h *held) Unlock() {
	h.typing.Unlock()
	h.prompts.Unlock()
}

func (in *Input) lock(ref events.PaneRef) *sync.Mutex {
	return in.paneMutex(in.locks, ref)
}

func (in *Input) paneMutex(set map[events.PaneRef]*sync.Mutex, ref events.PaneRef) *sync.Mutex {
	in.mu.Lock()
	defer in.mu.Unlock()
	if set[ref] == nil {
		set[ref] = &sync.Mutex{}
	}
	return set[ref]
}

// Prompt hands text to the pane's agent once and answers the client's own refusal: herdr's agent.prompt under the
// pane's lock, or, through a StagedPrompter, the look under that lock and the typing after it.
func (in *Input) Prompt(client Prompter, ref events.PaneRef, text string) error {
	return in.Deliver(client, ref, text).Try()
}

// Delivery is a prompt for a pane's agent, tried until the agent takes it (a first prompt, a task's, an arena lane's).
// On remuda-core its first look binds it to the agent it saw, so no later try reaches what replaced that agent.
type Delivery struct {
	in        *Input
	client    Prompter
	ref       events.PaneRef
	text      string
	asMessage bool
	then      func() error
	front     int
}

// Deliver is a delivery of text to the pane's agent, the agent at rest.
func (in *Input) Deliver(client Prompter, ref events.PaneRef, text string) *Delivery {
	return &Delivery{in: in, client: client, ref: ref, text: text}
}

// Try is one attempt, then run under the pane's lock once the prompt went; for herdr in the same hold as the prompt.
func (d *Delivery) Try() error {
	lock := d.in.lock(d.ref)
	staged, ok := d.client.(StagedPrompter)
	if !ok {
		lock.Lock()
		defer lock.Unlock()
		if err := d.client.AgentPrompt(d.ref.Pane, d.text); err != nil || d.then == nil {
			return err
		}
		return d.then()
	}
	order := d.in.paneMutex(d.in.prompts, d.ref)
	order.Lock()
	defer order.Unlock()
	lock.Lock()
	pid, send, err := staged.StagePrompt(d.ref.Pane, d.text, d.front, d.asMessage)
	lock.Unlock()
	if pid != 0 {
		d.front = pid
	}
	if err != nil {
		return err
	}
	if err := send(); err != nil || d.then == nil {
		return err
	}
	lock.Lock()
	defer lock.Unlock()
	return d.then()
}

// Submit delivers what a person wrote for the pane's agent, the way the chat does: herdr's own submit, and for a message
// that names a picture the Enter Claude Code loses of it (settle.go). Without an agent nothing goes. On remuda-core a
// turn the person's own draft started meanwhile takes it as a message.
func (in *Input) Submit(client Prompter, ref events.PaneRef, text string) error {
	d := &Delivery{in: in, client: client, ref: ref, text: text, asMessage: true}
	if two, ok := client.(interface {
		Settler
		Prompter
	}); ok && NamesPicture(text) {
		d.then = func() error { return enterAfterPaste(two, ref.Pane) }
	}
	return refused(d.Try())
}

// ErrNoAgent: the pane has no agent up to take the text, and what was written for one must not reach its shell, which
// would run every line.
var ErrNoAgent = errors.New("no agent is ready in that pane; nothing was sent")

// ErrNotSubmitted: remuda-core pasted the text and then found the agent gone from the terminal's front, so it pressed no
// Enter; the text may sit in the input line of whatever runs there now, the shell included.
var ErrNotSubmitted = errors.New("your message was pasted into the terminal but not submitted: the agent left its front as it went, so no Enter was pressed — look at the terminal and clear the line before pressing Enter there")

// refused words a client's refusal of a prompt for the person: the agent asking something, none up to take it (or the
// one looked at gone from the front, agent_left), or a paste left unsubmitted.
func refused(err error) error {
	switch {
	case herdr.IsCode(err, "agent_blocked"):
		return ErrAgentBusy
	case herdr.IsCode(err, "agent_not_found"), herdr.IsCode(err, "agent_not_ready"), herdr.IsCode(err, "agent_left"):
		return ErrNoAgent
	case herdr.IsCode(err, "not_submitted"):
		return ErrNotSubmitted
	}
	return err
}

// HandOff delivers text remuda composed at the person's press — a pull request's failures or reviews — as their own
// message, which on remuda-core a turn under way takes; never with the Enter Submit adds after a picture, which is for
// what the person wrote and would read a dialog the work just put up as their message unsent.
func (in *Input) HandOff(client Prompter, ref events.PaneRef, text string) error {
	return refused((&Delivery{in: in, client: client, ref: ref, text: text, asMessage: true}).Try())
}

// SubmitToAgent delivers text remuda has for an agent at rest — a schedule's prompt, a booked "continue", a tidy's
// /compact — through Prompt: on remuda-core, never into a turn.
func (in *Input) SubmitToAgent(client Prompter, ref events.PaneRef, text string) error {
	return refused(in.Prompt(client, ref, text))
}

// FirstPromptRetry and FirstPromptTimeout pace the delivery of a first prompt: a try every so often until the agent takes it.
var (
	FirstPromptRetry   = 2 * time.Second
	FirstPromptTimeout = 2 * time.Minute
)

// FirstPrompt hands the text a person wrote for an agent they just started over once it is up: Prompt, tried again every
// FirstPromptRetry while the agent is still coming up, until it takes the text or the timeout passes. A dialog on the
// agent's screen (Claude Code's trust question for the folder, which is the person's to answer) is waited through,
// never answered; the error says why the text was not delivered.
func (in *Input) FirstPrompt(client Prompter, ref events.PaneRef, text string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	delivery := in.Deliver(client, ref, text)
	var last error
	for {
		time.Sleep(FirstPromptRetry)
		err := delivery.Try()
		switch {
		case err == nil:
			return nil
		case herdr.IsCode(err, "agent_not_ready") || herdr.IsCode(err, "agent_not_found") || herdr.IsCode(err, "agent_blocked"):
			last = err
		default:
			return refused(err)
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

// ErrAgentWorking: the agent is in the middle of a turn, and a schedule's prompt waits for its next run rather than piling up behind it.
var ErrAgentWorking = errors.New("the agent is still working; nothing was sent")

// SubmitToFolder gives the text to the agent running in a machine's folder, the scheduler's door: never while that agent
// works (ErrAgentWorking) or asks something (ErrAgentBusy); the pane's title names where it went, or would have.
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
	if pane.AgentStatus == herdr.StatusWorking {
		return pane.DisplayTitle(), ErrAgentWorking
	}
	if err := in.SubmitToAgent(client, events.PaneRef{Machine: machineID, Pane: pane.PaneID}, text); err != nil {
		return pane.DisplayTitle(), err
	}
	return pane.DisplayTitle(), nil
}
