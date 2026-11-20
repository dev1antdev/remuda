package core

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestACommandLineIsQuotedForTheShellAtThePrompt(t *testing.T) {
	words := []string{"claude", "--resume", "abc-12", "--settings", `{"a":"it's"}`}
	for shell, want := range map[string]string{
		"zsh":        `claude --resume abc-12 --settings '{"a":"it'\''s"}'`,
		"sh":         `claude --resume abc-12 --settings '{"a":"it'\''s"}'`,
		"pwsh":       `claude --resume abc-12 --settings '{"a":"it''s"}'`,
		"powershell": `claude --resume abc-12 --settings '{\"a\":\"it''s\"}'`,
		"cmd":        `claude --resume abc-12 --settings "{""a"":""it's""}"`,
	} {
		if got := commandLine(shell, words); got != want {
			t.Errorf("%s: %s, want %s", shell, got, want)
		}
	}
	if got := commandLine("bash", []string{"codex", "a;b", "$HOME", "x y"}); got != `codex 'a;b' '$HOME' 'x y'` {
		t.Errorf("shell signs left bare: %s", got)
	}
}

func layoutJSON(t *testing.T, tree *herdr.LayoutNode) string {
	t.Helper()
	b, err := json.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestASplitAndAClosedPaneReshapeTheTabsTree(t *testing.T) {
	tree := splitBeside(nil, "t1", "t2", "right")
	tree = splitBeside(tree, "t2", "t3", "down")
	want := `{"type":"split","direction":"right","ratio":0.5,"first":{"type":"pane","pane_id":"t1"},"second":{"type":"split","direction":"down","ratio":0.5,"first":{"type":"pane","pane_id":"t2"},"second":{"type":"pane","pane_id":"t3"}}}`
	if got := layoutJSON(t, tree); got != want {
		t.Fatalf("split:\n%s\nwant\n%s", got, want)
	}
	tree = splitBeside(tree, "t9", "t4", "down")
	if panes := tree.Panes(); len(panes) != 4 || panes[3].PaneID != "t4" {
		t.Fatalf("a pane the tree lacks goes beside the whole: %s", layoutJSON(t, tree))
	}
	tree = withoutPane(tree, "t4")
	tree = withoutPane(tree, "t2")
	want = `{"type":"split","direction":"right","ratio":0.5,"first":{"type":"pane","pane_id":"t1"},"second":{"type":"pane","pane_id":"t3"}}`
	if got := layoutJSON(t, tree); got != want {
		t.Fatalf("closed:\n%s\nwant\n%s", got, want)
	}
	if withoutPane(withoutPane(tree, "t1"), "t3") != nil {
		t.Fatal("a tree with no pane left")
	}
}

func TestAnAgentTakesAPromptOnceItHasReported(t *testing.T) {
	started := time.Now().Add(-2 * time.Second)
	cases := []struct {
		name  string
		term  Terminal
		ready bool
	}{
		{"running before the mirror looked, reporting", Terminal{Status: &Status{State: "idle"}}, true},
		{"running before the mirror looked, reporting nothing", Terminal{}, false},
		{"no report since it started", Terminal{agentAt: started, statusAt: started.Add(-time.Minute)}, false},
		{"reported since it started", Terminal{agentAt: started, statusAt: started.Add(time.Second)}, true},
		{"its first report named it", Terminal{agentAt: started, statusAt: started}, true},
		{"reports nothing however long it runs", Terminal{agentAt: started.Add(-time.Hour)}, false},
	}
	for _, c := range cases {
		if got := readyForPrompt(&c.term); got != c.ready {
			t.Errorf("%s: ready %v", c.name, got)
		}
	}
}

func TestAReportBeforeTheForegroundsLookCountsAsTheAgentsFirst(t *testing.T) {
	shell := &Foreground{Pid: 10, Name: "zsh"}
	m := mirror{terminals: map[string]*Terminal{"t1": {Address: "t1", State: "live", Pid: 10, Foreground: shell}}}
	m.apply(Event{Type: "status", Terminal: "t1", Data: json.RawMessage(`{"status":{"state":"idle","app":"claude-code"},"reported":true}`)})
	m.apply(Event{Type: "foreground", Terminal: "t1", Data: json.RawMessage(`{"pid":11,"name":"claude","argv":["claude"]}`)})
	if term := m.terminals["t1"]; agentOf(term) != "claude" || !readyForPrompt(term) {
		t.Fatalf("the agent is %q, ready %v", agentOf(term), readyForPrompt(term))
	}
}

func placed(space, tab string) *Terminal {
	return &Terminal{Address: space + tab, State: "live", Labels: map[string]string{LabelSpace: space, LabelTab: tab}}
}

func TestClosingATabKeepsAWorkspaceSomethingElseIsInNow(t *testing.T) {
	ending := []string{"w1a"}
	cases := []struct {
		name  string
		tabs  []DocTab
		live  []*Terminal
		stays bool
	}{
		{"another tab in the document", []DocTab{{ID: "a"}, {ID: "b"}}, []*Terminal{placed("w1", "a")}, true},
		{"a terminal another request put there meanwhile", []DocTab{{ID: "a"}}, []*Terminal{placed("w1", "a"), placed("w1", "new")}, true},
		{"nothing else", []DocTab{{ID: "a"}}, []*Terminal{placed("w1", "a")}, false},
	}
	for _, c := range cases {
		a := &Adapter{held: true, state: mirror{terminals: map[string]*Terminal{}}}
		for _, term := range c.live {
			a.state.terminals[term.Address] = term
		}
		doc := Document{Workspaces: []DocWorkspace{{ID: "w1", ActiveTab: "a", Tabs: c.tabs}}}
		a.dropTab(&doc, "a", ending)
		if stays := len(doc.Workspaces) == 1; stays != c.stays {
			t.Errorf("%s: the workspace stays %v", c.name, stays)
		}
		if c.stays && slices.ContainsFunc(doc.Workspaces[0].Tabs, func(tab DocTab) bool { return tab.ID == "a" }) {
			t.Errorf("%s: the closed tab is still there", c.name)
		}
	}
}

func TestATerminalTheCoreSaidWentIsNotPutBack(t *testing.T) {
	m := mirror{terminals: map[string]*Terminal{}}
	m.apply(Event{Type: "removed", Terminal: "t7", Data: json.RawMessage(`{}`)})
	if !m.gone["t7"] {
		t.Fatal("a removal of a terminal the mirror never held is forgotten")
	}
}

func TestAPaneClosedOutOfATreeThatPlacesAnotherKeepsTheTab(t *testing.T) {
	doc := Document{Workspaces: []DocWorkspace{{ID: "w1", Tabs: []DocTab{{ID: "a", Layout: splitBeside(nil, "t1", "t2", "right")}}}}}
	if !dropPane(&doc, "a", "t1") || doc.Workspaces[0].Tabs[0].Layout.PaneID != "t2" {
		t.Fatalf("the split written meanwhile is lost: %+v", doc.Workspaces[0].Tabs[0].Layout)
	}
	if dropPane(&doc, "a", "t2") {
		t.Fatal("a tree with no pane left still places one")
	}
}

// A prompt the core pasted and left unsubmitted is said so, and points at the terminal, never at nothing sent; a
// terminal under an older holder keeps the core's words that a terminal started since takes it.
func TestAPromptLeftUnsubmittedIsToldApart(t *testing.T) {
	a := &Adapter{}
	pasted := a.inputRefusal(&Error{Kind: "not_submitted", Message: "t1 has 7 in front, not 5", Hint: "read the terminal before sending again"}, "agent_not_ready", "agent_blocked")
	if !herdr.IsCode(pasted, "not_submitted") || !strings.Contains(pasted.Error(), "pasted but not submitted") || !strings.Contains(pasted.Error(), "read the terminal") || strings.Contains(pasted.Error(), "nothing was sent") {
		t.Fatalf("pasted, no Enter: %v", pasted)
	}
	older := a.inputRefusal(&Error{Kind: "needs_update", Message: "t1 runs in an older rc's holder", Hint: "nothing was typed; a terminal started since rc was updated takes it"}, "agent_not_ready", "agent_blocked")
	if !herdr.IsCode(older, "invalid_request") || !strings.Contains(older.Error(), "older") || !strings.Contains(older.Error(), "a terminal started since") {
		t.Fatalf("an older holder: %v", older)
	}
}

// scripted is a core answering terminal.send and terminal.get in turn, noting each send.
type scripted struct {
	sends []error
	gets  []Terminal
	sent  []map[string]any
}

func (s *scripted) call(_ context.Context, method string, params, result any) error {
	switch method {
	case "terminal.send":
		s.sent = append(s.sent, params.(map[string]any))
		err := s.sends[0]
		s.sends = s.sends[1:]
		return err
	case "terminal.get":
		*result.(*Terminal) = s.gets[0]
		s.gets = s.gets[1:]
		return nil
	}
	return fmt.Errorf("unexpected %s", method)
}

// claudeAt is Claude Code in front under pid, its status state ("" for none reported).
func claudeAt(pid int, state string) Terminal {
	t := Terminal{Foreground: &Foreground{Pid: pid, Name: "claude", Argv: []string{"claude"}}}
	if state != "" {
		t.Status = &Status{State: state, App: "claude-code"}
	}
	return t
}

var untyped = &Error{Kind: "status_mismatch", Message: "t1 moved", Hint: "nothing was typed"}

var unnamed = &Error{Kind: "invalid_params", Message: "unknown variant `done`, expected `idle` or `working`"}

// A send names the status it saw unless the agent works; one the core refused untyped goes once more to the same
// agent as it stands — at rest again after a short turn, or at work for the person's own message alone — and another
// agent, or one leaving, is agent_left. A core that names idle and working alone takes the person's message to a done
// agent without the word, and refuses anyone else's.
func TestASendRefusedUntypedIsReadAgain(t *testing.T) {
	a := &Adapter{}
	for _, c := range []struct {
		name      string
		from      string
		asMessage bool
		sends     []error
		again     Terminal
		want      string
		named     []string // what each send named as `if`, "" for nothing
	}{
		{"a short turn from idle", "idle", false, []error{untyped, nil}, claudeAt(5, "idle"), "", []string{"idle", "idle"}},
		{"a short turn from done", "done", false, []error{untyped, nil}, claudeAt(5, "done"), "", []string{"done", "done"}},
		{"a turn from done, anyone else's", "done", false, []error{untyped}, claudeAt(5, "working"), "agent_working", []string{"done"}},
		{"a turn from idle, anyone else's", "idle", false, []error{untyped}, claudeAt(5, "working"), "agent_working", []string{"idle"}},
		{"a turn from done, the person's message", "done", true, []error{untyped, nil}, claudeAt(5, "working"), "", []string{"done", ""}},
		{"a turn from idle, the person's message", "idle", true, []error{untyped, nil}, claudeAt(5, "working"), "", []string{"idle", ""}},
		{"another agent in front", "idle", true, []error{untyped}, claudeAt(9, "working"), "agent_left", []string{"idle"}},
		{"the agent leaving", "done", false, []error{untyped}, claudeAt(5, ""), "agent_left", []string{"done"}},
		{"done, a core naming idle alone, anyone else's", "done", false, []error{unnamed}, Terminal{}, "invalid_request", []string{"done"}},
		{"done, a core naming idle alone, the person's message", "done", true, []error{unnamed, nil}, Terminal{}, "", []string{"done", ""}},
	} {
		core := &scripted{sends: c.sends, gets: []Terminal{c.again}}
		err := a.sendTo(core.call, "t1", "hello", "claude", claudeAt(5, c.from), c.asMessage)
		if c.want == "" && err != nil || c.want != "" && !herdr.IsCode(err, c.want) {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
		var named []string
		for _, params := range core.sent {
			word, _ := params["if"].(string)
			named = append(named, word)
		}
		if !slices.Equal(named, c.named) {
			t.Errorf("%s: the sends named %q, want %q", c.name, named, c.named)
		}
	}
}

// A send to an agent at work names no status: the person's own message goes to the turn.
func TestASendToAnAgentAtWorkNamesNoStatus(t *testing.T) {
	core := &scripted{sends: []error{nil}}
	if err := (&Adapter{}).sendTo(core.call, "t1", "hello", "claude", claudeAt(5, "working"), true); err != nil {
		t.Fatal(err)
	}
	if _, named := core.sent[0]["if"]; named {
		t.Fatalf("a send to a turn named a status: %v", core.sent[0])
	}
}
