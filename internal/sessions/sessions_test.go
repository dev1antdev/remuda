package sessions

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
)

// fakeClient answers like herdr: prompts refused by code, and a record of what was typed.
type fakeClient struct {
	withArgs  []string // the last AgentStartWith's arguments
	promptErr *herdr.Error
	typed     []string
	keys      [][]string
	starts    int
	startErrs []*herdr.Error
}

func (f *fakeClient) AgentPrompt(target, text string) error {
	if f.promptErr != nil {
		return f.promptErr
	}
	f.typed = append(f.typed, "prompt:"+text)
	return nil
}

func (f *fakeClient) PaneSendText(paneID, text string) error {
	f.typed = append(f.typed, "text:"+text)
	return nil
}

func (f *fakeClient) PaneSendKeys(paneID string, keys []string) error {
	f.keys = append(f.keys, keys)
	return nil
}

func (f *fakeClient) AgentStartWith(paneID, kind, name string, args []string, timeout time.Duration) error {
	f.withArgs = append([]string{kind, name}, args...)
	return f.AgentStart(paneID, kind)
}

func (f *fakeClient) AgentStart(paneID, kind string) error {
	f.starts++
	if len(f.startErrs) > 0 {
		err := f.startErrs[0]
		f.startErrs = f.startErrs[1:]
		return err
	}
	return nil
}

func TestSubmitPrefersThePromptAndTypesForAShell(t *testing.T) {
	in := NewInput(nil, nil)
	ref := events.PaneRef{Machine: "local", Pane: "w1:p1"}
	agent := &fakeClient{}
	if err := in.Submit(agent, ref, "hello"); err != nil || len(agent.typed) != 1 || agent.typed[0] != "prompt:hello" {
		t.Fatalf("agent: %v %v", err, agent.typed)
	}
	shell := &fakeClient{promptErr: &herdr.Error{Code: "agent_not_found"}}
	if err := in.Submit(shell, ref, "ls"); err != nil || len(shell.typed) != 1 || shell.typed[0] != "text:ls" || len(shell.keys) != 1 || shell.keys[0][0] != "enter" {
		t.Fatalf("shell: %v %v %v", err, shell.typed, shell.keys)
	}
	blocked := &fakeClient{promptErr: &herdr.Error{Code: "agent_blocked"}}
	if err := in.Submit(blocked, ref, "x"); !errors.Is(err, ErrAgentBusy) || len(blocked.typed) != 0 {
		t.Fatalf("blocked: %v %v", err, blocked.typed)
	}
	if in.Lock("local", "w1:p1") != in.Lock("local", "w1:p1") || in.Lock("local", "w1:p1") == in.Lock("local", "w1:p2") {
		t.Fatal("one lock per pane")
	}
}

func TestRefOK(t *testing.T) {
	for _, bad := range []string{"", "-x", "a b", "a..b", "a\tb", "x\x7f"} {
		if RefOK(bad) {
			t.Errorf("RefOK(%q) accepted", bad)
		}
	}
	for _, good := range []string{"main", "task-12-hello", "feature/x", "v1.2"} {
		if !RefOK(good) {
			t.Errorf("RefOK(%q) refused", good)
		}
	}
}

func TestKeysAndAgents(t *testing.T) {
	keys, err := Keys([]string{"Enter", "ctrl+C"})
	if err != nil || keys[0] != "enter" || keys[1] != "ctrl+c" {
		t.Fatalf("keys: %v %v", keys, err)
	}
	if _, err := Keys([]string{"ctrl+alt+del"}); err == nil {
		t.Fatal("an unknown key was allowed")
	}
	if !KnownAgent("claude") || !KnownAgent("") || !KnownAgent("shell") || KnownAgent("skynet") {
		t.Fatal("agents")
	}
	client := &fakeClient{startErrs: []*herdr.Error{{Code: "agent_pane_busy"}, {Code: "agent_not_ready"}}}
	if err := StartAgent(client, "w1:p1", "claude"); err != nil || client.starts != 3 {
		t.Fatalf("start retried %d times: %v", client.starts, err)
	}
	refused := &fakeClient{startErrs: []*herdr.Error{{Code: "agent_name_taken"}}}
	if err := StartAgent(refused, "w1:p1", "claude"); !herdr.IsCode(err, "agent_name_taken") || refused.starts != 1 {
		t.Fatalf("another refusal is final: %v", err)
	}
}

func TestResumeArgs(t *testing.T) {
	cases := map[string]string{"claude": "--resume s1", "codex": "resume s1", "gemini": "--resume s1", "kimi": "--session s1"}
	for kind, want := range cases {
		args, ok := ResumeArgs(kind, "s1")
		if !ok || strings.Join(args, " ") != want {
			t.Errorf("%s: got %v %v, want %q", kind, args, ok, want)
		}
	}
	if _, ok := ResumeArgs("shell", "s1"); ok {
		t.Errorf("a shell resumes nothing")
	}
}

func TestStartArgsOrderAndModelFlags(t *testing.T) {
	args, err := StartArgs("claude", Start{Resume: "0123-abcd", Model: "opus", Args: []string{"--permission-mode", "plan"}})
	if err != nil || strings.Join(args, " ") != "--resume 0123-abcd --model opus --permission-mode plan" {
		t.Fatalf("args: %v %v", args, err)
	}
	for kind, want := range map[string]string{"claude": "--model m", "codex": "--model m", "gemini": "--model m", "kimi": "--model m"} {
		if got, ok := ModelArgs(kind, "m"); !ok || strings.Join(got, " ") != want {
			t.Errorf("%s: %v %v", kind, got, ok)
		}
	}
	if _, ok := ModelArgs("pi", "m"); ok {
		t.Error("pi has no model flag remuda knows")
	}
	if _, err := StartArgs("pi", Start{Model: "m"}); err == nil {
		t.Error("a model for a kind without the flag must be refused")
	}
	if args, err := StartArgs("pi", Start{Args: []string{"--yolo"}}); err != nil || strings.Join(args, " ") != "--yolo" {
		t.Errorf("extra arguments work for every kind: %v %v", args, err)
	}
	if args, err := StartArgs("claude", Start{}); err != nil || args != nil {
		t.Errorf("nothing asked, nothing passed: %v %v", args, err)
	}
	f := &fakeClient{}
	if err := StartAgentWith(f, "w1:p1", "claude", Start{Model: "sonnet", Args: []string{"-p", "hi there"}}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.withArgs, "|"); got != "claude|claude|--model|sonnet|-p|hi there" {
		t.Errorf("args: %q", got)
	}
	plain := &fakeClient{}
	if err := StartAgentWith(plain, "w1:p1", "claude", Start{}); err != nil || plain.withArgs != nil || plain.starts != 1 {
		t.Errorf("a plain start goes through agent.start without arguments: %v %v", plain.withArgs, err)
	}
}

func TestSplitArgs(t *testing.T) {
	cases := map[string][]string{
		"":                                  nil,
		"   ":                               nil,
		"--permission-mode plan":            {"--permission-mode", "plan"},
		`--append-system-prompt "be terse"`: {"--append-system-prompt", "be terse"},
		`-p 'it''s'`:                        {"-p", "its"},
		`a\ b c`:                            {"a b", "c"},
		`"a \"q\" b"`:                       {`a "q" b`},
		"x\ty\nz":                           {"x", "y", "z"},
		`--dangerously-skip-permissions`:    {"--dangerously-skip-permissions"},
	}
	for in, want := range cases {
		got, err := SplitArgs(in)
		if err != nil || strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("SplitArgs(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{`"open`, `'open`, `tail\`, "a\x1bb", "a\x7fb"} {
		if _, err := SplitArgs(bad); err == nil {
			t.Errorf("SplitArgs(%q) accepted", bad)
		}
	}
}

// readyAfter answers agent_not_ready for the first n prompts, then takes one.
type readyAfter struct {
	fakeClient
	notReady int
}

func (f *readyAfter) AgentPrompt(target, text string) error {
	if f.notReady > 0 {
		f.notReady--
		return &herdr.Error{Code: "agent_not_ready"}
	}
	return f.fakeClient.AgentPrompt(target, text)
}

func TestFirstPromptWaitsForTheAgent(t *testing.T) {
	FirstPromptRetry = time.Millisecond
	defer func() { FirstPromptRetry = 2 * time.Second }()
	in := NewInput(nil, nil)
	ref := events.PaneRef{Machine: "local", Pane: "w1:p1"}
	client := &readyAfter{notReady: 3}
	if err := in.FirstPrompt(client, ref, "build it", time.Second); err != nil || strings.Join(client.typed, ",") != "prompt:build it" {
		t.Fatalf("delivered once the agent is up: %v %v", err, client.typed)
	}
	never := &fakeClient{promptErr: &herdr.Error{Code: "agent_not_ready"}}
	if err := in.FirstPrompt(never, ref, "x", 20*time.Millisecond); err == nil || !strings.Contains(err.Error(), "did not come up") || len(never.typed) != 0 {
		t.Fatalf("timeout: %v %v", err, never.typed)
	}
	blocked := &fakeClient{promptErr: &herdr.Error{Code: "agent_blocked"}}
	if err := in.FirstPrompt(blocked, ref, "x", 20*time.Millisecond); err == nil || !strings.Contains(err.Error(), "asks something first") || len(blocked.keys) != 0 {
		t.Fatalf("a dialog is waited through, never answered: %v %v", err, blocked.keys)
	}
	broken := &fakeClient{promptErr: &herdr.Error{Code: "pane_not_found"}}
	if err := in.FirstPrompt(broken, ref, "x", time.Second); !herdr.IsCode(err, "pane_not_found") {
		t.Fatalf("another refusal ends the wait at once: %v", err)
	}
}

func TestStartAgentResumed(t *testing.T) {
	f := &fakeClient{}
	if err := StartAgentResumed(f, "w1:p1", "claude", "0123-abcd"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.withArgs, " "); got != "claude claude --resume 0123-abcd" {
		t.Errorf("args: %q", got)
	}
	if err := StartAgentResumed(f, "w1:p1", "shell", "0123-abcd"); err == nil {
		t.Errorf("a shell cannot resume")
	}
}
