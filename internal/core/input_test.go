package core

import (
	"encoding/json"
	"slices"
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
