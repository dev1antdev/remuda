package core

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr"
)

func term(address string, created int64, labels map[string]string) *Terminal {
	return &Terminal{Address: address, CreatedMs: created, Labels: labels, State: "live", Pid: 100, Cwd: "/home/me/api", Cols: 80, Rows: 24}
}

func placedIn(space, tab string) map[string]string {
	return map[string]string{LabelSpace: space, LabelTab: tab}
}

func split(a, b string) *herdr.LayoutNode {
	return &herdr.LayoutNode{Type: "split", Direction: "right", Ratio: 0.5,
		First: &herdr.LayoutNode{Type: "pane", PaneID: a}, Second: &herdr.LayoutNode{Type: "pane", PaneID: b}}
}

func TestTheSnapshotLaysTheDocumentOutAndPlacesEveryLiveTerminal(t *testing.T) {
	m := mirror{version: "0.2.0", terminals: map[string]*Terminal{
		"t1": term("t1", 1, placedIn("w1", "a")),
		"t2": term("t2", 2, nil),
		"t3": term("t3", 3, placedIn("w1", "b")),
		"t4": term("t4", 4, placedIn("w1", "gone")),
		"t5": term("t5", 5, nil),
		"t6": term("t6", 6, placedIn("w9", "x")),
	}, doc: Document{Workspaces: []DocWorkspace{
		{ID: "w1", Label: "api", ActiveTab: "b", Tabs: []DocTab{
			{ID: "a", Label: "agents", Layout: split("t2", "t1")},
			{ID: "b"},
			{ID: "c", Layout: &herdr.LayoutNode{Type: "pane", PaneID: "t99"}},
		}},
		{ID: "w2", Label: "closed", Tabs: []DocTab{{ID: "d", Layout: &herdr.LayoutNode{Type: "pane", PaneID: "t98"}}}},
	}}}
	snap := m.snapshot(nil)

	var spaces []string
	for _, ws := range snap.Workspaces {
		spaces = append(spaces, ws.WorkspaceID+":"+ws.Label+":"+ws.ActiveTabID)
	}
	if want := []string{"w1:api:b", "core-t5:api:core-t5-t5", "w9:api:x"}; !slices.Equal(spaces, want) {
		t.Fatalf("workspaces %v, want %v", spaces, want)
	}
	var tabs []string
	for _, tab := range snap.Tabs {
		tabs = append(tabs, tab.WorkspaceID+"/"+tab.TabID+"#"+string(rune('0'+tab.Number))+":"+tab.Label)
	}
	if want := []string{"w1/a#1:agents", "w1/b#2:", "w1/gone#3:", "core-t5/core-t5-t5#1:", "w9/x#1:"}; !slices.Equal(tabs, want) {
		t.Fatalf("tabs %v, want %v", tabs, want)
	}
	var panes []string
	for _, p := range snap.Panes {
		panes = append(panes, p.TabID+"/"+p.PaneID)
	}
	if want := []string{"a/t2", "a/t1", "b/t3", "gone/t4", "core-t5-t5/t5", "x/t6"}; !slices.Equal(panes, want) {
		t.Fatalf("panes %v, want %v", panes, want)
	}
	if cols, rows := snap.PaneSize("t1"); cols != 80 || rows != 24 {
		t.Fatalf("t1 is %dx%d", cols, rows)
	}
	if snap.Version != "remuda-core 0.2.0" {
		t.Fatalf("version %q", snap.Version)
	}
}

func TestTheAgentIsTheForegroundProcessOrWhatTheProgramReported(t *testing.T) {
	claudeStatus := &Status{State: "working", App: "claude-code"}
	cases := []struct {
		name string
		t    Terminal
		want string
	}{
		{"its own process", Terminal{Pid: 1, Foreground: &Foreground{Pid: 2, Name: "claude"}}, "claude"},
		{"through node", Terminal{Pid: 1, Foreground: &Foreground{Pid: 2, Name: "node", Argv: []string{"node", "/opt/bin/codex"}}}, "codex"},
		{"windows", Terminal{Pid: 1, Foreground: &Foreground{Pid: 2, Name: `C:\Users\me\.local\bin\claude.exe`}}, "claude"},
		{"reported", Terminal{Pid: 1, Foreground: &Foreground{Pid: 2, Name: "bun"}, Status: claudeStatus}, "claude"},
		{"its shell back", Terminal{Pid: 1, Foreground: &Foreground{Pid: 1, Name: "zsh"}, Status: claudeStatus}, ""},
		{"a shell", Terminal{Pid: 1, Foreground: &Foreground{Pid: 1, Name: "bash"}}, ""},
		{"an app remuda does not know", Terminal{Status: &Status{State: "idle", App: "Aider"}}, "aider"},
	}
	for _, c := range cases {
		if got := agentOf(&c.t); got != c.want {
			t.Errorf("%s: agent %q, want %q", c.name, got, c.want)
		}
	}
	for state, want := range map[string]string{"working": "working", "blocked": "blocked", "done": "done", "idle": "idle", "error": "done", "odd": "unknown"} {
		if got := statusOf(&Terminal{Status: &Status{State: state}}); got != want {
			t.Errorf("status %s reads %s, want %s", state, got, want)
		}
	}
	if got := statusOf(&Terminal{}); got != herdr.StatusUnknown {
		t.Errorf("no status reads %s", got)
	}
}

func coreEvent(kind, terminal string, data any) Event {
	raw, _ := json.Marshal(data)
	return Event{Type: kind, Terminal: terminal, Data: raw}
}

func TestEachEventMovesTheMirrorAndSaysWhatItWasInHerdrsWords(t *testing.T) {
	m := mirror{terminals: map[string]*Terminal{"t1": term("t1", 1, nil)}}
	m.terminals["t1"].Foreground = &Foreground{Pid: 100, Name: "zsh"}
	steps := []struct {
		ev   Event
		kind string
	}{
		{coreEvent("foreground", "t1", Foreground{Pid: 200, Name: "claude", Cwd: "/home/me/web"}), "pane.agent_detected"},
		{coreEvent("status", "t1", map[string]any{"status": Status{State: "working", App: "claude-code"}, "reported": true}), herdr.EventAgentStatus},
		{coreEvent("title", "t1", map[string]string{"title": "⠋ Refactoring"}), "pane.updated"},
		{coreEvent("labels", "t1", map[string]any{"labels": placedIn("w1", "a")}), "pane.moved"},
		{coreEvent("size", "t1", map[string]int{"cols": 120, "rows": 40}), "layout.updated"},
		{coreEvent("created", "t2", Terminal{Address: "t2", State: "live"}), "pane.created"},
		{coreEvent("finished", "t2", map[string]any{"reason": "exited"}), "pane.closed"},
	}
	for _, step := range steps {
		told, ok := m.apply(step.ev)
		if !ok || told.Kind != step.kind {
			t.Fatalf("%s told %+v %v, want %s", step.ev.Type, told, ok, step.kind)
		}
	}
	t1 := m.terminals["t1"]
	if agentOf(t1) != "claude" || statusOf(t1) != "working" || t1.Title != "⠋ Refactoring" || t1.Labels[LabelTab] != "a" || t1.Cols != 120 {
		t.Fatalf("t1 is %+v", t1)
	}
	if _, there := m.terminals["t2"]; there {
		t.Fatal("a finished terminal stays in the mirror")
	}
	pane := m.snapshot(nil).Panes[0]
	if pane.TitleStripped != "Refactoring" || pane.CWD != "/home/me/api" || pane.ForegroundCWD != "/home/me/web" || pane.Agent != "claude" {
		t.Fatalf("pane %+v", pane)
	}
	if _, ok := m.apply(coreEvent("title", "t9", map[string]string{"title": "x"})); ok {
		t.Fatal("an event of a terminal the mirror never held is told")
	}
}

func TestADocumentThatIsNoDocumentReadsAsEmpty(t *testing.T) {
	for _, raw := range []string{"", "null", `{"workspaces":[{"id":"w1","tabs":[]}]}`} {
		if _, err := parseDocument(json.RawMessage(raw)); err != nil {
			t.Errorf("%q: %v", raw, err)
		}
	}
	if _, err := parseDocument(json.RawMessage(`[1,2]`)); err == nil {
		t.Error("an array reads as a document")
	}
}

func TestALabelTakenAwayLeavesTheTerminal(t *testing.T) {
	m := mirror{terminals: map[string]*Terminal{"t1": term("t1", 1, placedIn("w1", "a"))}}
	if _, ok := m.apply(coreEvent("labels", "t1", map[string]any{"labels": map[string]string{LabelSpace: "w1"}})); !ok {
		t.Fatal("not told")
	}
	if _, there := m.terminals["t1"].Labels[LabelTab]; there {
		t.Fatalf("labels %v", m.terminals["t1"].Labels)
	}
}

func TestAPanesFolderIsItsShellsAndAWorkspacesTheDocuments(t *testing.T) {
	m := mirror{terminals: map[string]*Terminal{"t1": term("t1", 1, placedIn("w1", "a"))},
		doc: Document{Workspaces: []DocWorkspace{{ID: "w1", Folder: "/home/me/repo", Tabs: []DocTab{{ID: "a"}}}}}}
	m.apply(coreEvent("foreground", "t1", Foreground{Pid: 100, Name: "zsh", Cwd: "/home/me/repo/web"}))
	m.apply(coreEvent("foreground", "t1", Foreground{Pid: 300, Name: "vim", Cwd: "/tmp"}))
	snap := m.snapshot(nil)
	if p := snap.Pane("t1"); p.CWD != "/home/me/repo/web" || p.ForegroundCWD != "/tmp" {
		t.Fatalf("pane %+v", p)
	}
	if dir := snap.WorkspaceDir(&snap.Workspaces[0]); dir != "/home/me/repo" {
		t.Fatalf("workspace dir %q", dir)
	}
}

// A shell the core lost and gives a new one is marked restored, whether the mirror heard it happen or found it so when
// it filled again, until an agent takes the terminal; a shell that only moved on is not.
func TestAShellTheCoreGaveANewOneIsRestoredUntilAnAgentTakesIt(t *testing.T) {
	restored := func(m *mirror, address string) bool { return m.terminals[address].restored }
	m := mirror{terminals: map[string]*Terminal{"t1": term("t1", 1, nil)}}
	m.terminals["t1"].Foreground = &Foreground{Pid: 200, Name: "claude"}
	m.apply(coreEvent("status", "t1", map[string]any{"status": nil}))
	m.apply(coreEvent("lost", "t1", map[string]any{"reason": "server_stop"}))
	if !restored(&m, "t1") || m.snapshot(nil).Panes[0].Agent != "" || !m.snapshot(nil).Panes[0].Restored {
		t.Fatalf("a lost shell: %+v", m.terminals["t1"])
	}
	m.apply(coreEvent("restarted", "t1", Terminal{Address: "t1", State: "live", Pid: 300, Foreground: &Foreground{Pid: 300, Name: "zsh"}}))
	if !restored(&m, "t1") {
		t.Fatal("the new shell is not marked restored")
	}
	m.apply(coreEvent("foreground", "t1", Foreground{Pid: 301, Name: "claude"}))
	if restored(&m, "t1") {
		t.Fatal("an agent took the terminal, and it still reads restored")
	}

	a := &Adapter{answered: make(chan struct{}), streams: map[*eventStream]struct{}{}, state: mirror{terminals: map[string]*Terminal{
		"t1": {Address: "t1", State: "live", Pid: 100}, "t2": {Address: "t2", State: "live", Pid: 100}, "t3": {Address: "t3", State: "live", Pid: 100},
	}}}
	a.fill([]Terminal{
		{Address: "t1", State: "live", Pid: 101},
		{Address: "t2", State: "live", Pid: 100},
		{Address: "t3", State: "live", Reason: "host_shutdown"},
		{Address: "t4", State: "live", Pid: 100},
	}, Document{}, "0.2.0")
	for address, want := range map[string]bool{"t1": true, "t2": false, "t3": true, "t4": false} {
		if got := restored(&a.state, address); got != want {
			t.Errorf("%s restored %v after a fill, want %v", address, got, want)
		}
	}
}
