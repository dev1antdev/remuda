package restart

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
)

// named puts a pane in the tab given, with the title given.
func named(p herdr.PaneInfo, tabID, title string) herdr.PaneInfo {
	p.TabID, p.Title = tabID, title
	return p
}

func snapshotWithTabs(tabs []herdr.TabInfo, panes ...herdr.PaneInfo) *herdr.Snapshot {
	snap := snapshot(panes...)
	snap.Tabs = tabs
	return snap
}

func withSession(p herdr.PaneInfo, session string) herdr.PaneInfo {
	p.AgentSession = &herdr.AgentSession{Kind: "id", Value: session}
	return p
}

func TestAPaneThatWentAwayIsListedAsClosed(t *testing.T) {
	m := OpenMemory("")
	at := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	tabs := []herdr.TabInfo{{TabID: "w1:t1", WorkspaceID: "w1", Number: 1, Label: "review"}, {TabID: "w1:t2", WorkspaceID: "w1", Number: 2, Label: "2"}}
	claude := named(agentPane("w1:p1", "claude", "/work/api"), "w1:t1", "Fix the queue")
	codex := named(withSession(agentPane("w1:p2", "codex", "/work/api"), "019a2b3c-4d5e-4f60-8a1b-2c3d4e5f6a7b"), "w1:t2", "codex")
	m.Observe(events.Snapshot{Machine: "gog", Snap: snapshotWithTabs(tabs, claude, codex), At: at})
	m.Observe(events.Snapshot{Machine: "gog", Snap: snapshotWithTabs(tabs[1:], codex), At: at.Add(time.Minute)})

	closed := m.Closed()
	if len(closed) != 1 {
		t.Fatalf("closed: %+v", closed)
	}
	c := closed[0]
	if c.Machine != "gog" || c.Pane != "w1:p1" || c.Agent != "claude" || c.Session != sid || c.Dir != "/work/api" || c.Title != "Fix the queue" || c.Workspace != "api" || c.Tab != "review" || !c.At.Equal(at.Add(time.Minute)) {
		t.Fatalf("the closed entry: %+v", c)
	}
	if _, ok := m.chats["gog|w1:p1"]; ok {
		t.Fatal("a closed pane's chat is forgotten")
	}

	m.Observe(events.Snapshot{Machine: "gog", Snap: snapshotWithTabs(nil), At: at.Add(2 * time.Minute)})
	if closed := m.Closed(); len(closed) != 2 || closed[0].Agent != "codex" || closed[0].Tab != "" {
		t.Fatalf("newest first, an unnamed tab named by nothing: %+v", closed)
	}
}

func TestAWorkspaceRemudaKeepsForItselfIsNeverRemembered(t *testing.T) {
	noGrace(t)
	m := OpenMemory("")
	m.Skip = func(machine, workspace string) bool { return machine == "gog" && workspace == "w1" }
	observe(m, "gog", false, agentPane("w1:p1", "claude", "/state/concierge"))
	observe(m, "gog", true, shellPane("w1:p1", "/state/concierge"))
	if stopped := m.Stopped("gog"); len(stopped) != 0 {
		t.Errorf("a reboot of the concierge offers nothing back: %+v", stopped)
	}
	observe(m, "gog", false)
	if closed := m.Closed(); len(closed) != 0 {
		t.Errorf("its close is not listed: %+v", closed)
	}
	observe(m, "local", false, agentPane("w1:p1", "claude", "/work/api"))
	observe(m, "local", false)
	if closed := m.Closed(); len(closed) != 1 || closed[0].Machine != "local" {
		t.Errorf("another machine's agent is remembered as ever: %+v", closed)
	}
}

func TestAShellOrAnExitIsNotAClose(t *testing.T) {
	m := OpenMemory("")
	observe(m, "local", false, agentPane("w1:p1", "claude", "/work/api"))
	observe(m, "local", false, shellPane("w1:p1", "/work/api"))
	observe(m, "local", false)
	if closed := m.Closed(); len(closed) != 0 {
		t.Fatalf("the person left the agent first; its pane closing later closes no agent: %+v", closed)
	}
	observe(m, "local", false, shellPane("w9:p1", "/tmp"))
	observe(m, "local", false)
	if closed := m.Closed(); len(closed) != 0 {
		t.Fatalf("a shell is never listed: %+v", closed)
	}
}

func TestAPausedMachineNotesNoClose(t *testing.T) {
	m := OpenMemory("")
	observe(m, "local", false, agentPane("w1:p1", "claude", "/work/api"))
	m.Pause("local")
	observe(m, "local", false)
	m.Unpause("local")
	if closed := m.Closed(); len(closed) != 0 {
		t.Fatalf("remuda's own restart of herdr closes nothing: %+v", closed)
	}
}

func TestTheClosedListKeepsTheNewestOncePerSession(t *testing.T) {
	m := OpenMemory("")
	at := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	for i := 0; i < ClosedKeep+5; i++ {
		session := fmt.Sprintf("9b1c2d3e-4f50-4a6b-8c7d-%012d", i%(ClosedKeep+2))
		p := withSession(agentPane(fmt.Sprintf("w1:p%d", i), "claude", "/work/api"), session)
		m.Observe(events.Snapshot{Machine: "local", Snap: snapshot(p), At: at.Add(time.Duration(2*i) * time.Second)})
		m.Observe(events.Snapshot{Machine: "local", Snap: snapshot(), At: at.Add(time.Duration(2*i+1) * time.Second)})
	}
	closed := m.Closed()
	if len(closed) != ClosedKeep {
		t.Fatalf("kept %d, want %d", len(closed), ClosedKeep)
	}
	seen := map[string]bool{}
	for i, c := range closed {
		if seen[c.Session] {
			t.Fatalf("session %s listed twice", c.Session)
		}
		seen[c.Session] = true
		if i > 0 && c.At.After(closed[i-1].At) {
			t.Fatal("not newest first")
		}
	}
	if closed[0].Pane != fmt.Sprintf("w1:p%d", ClosedKeep+4) {
		t.Fatalf("the newest close first: %s", closed[0].Pane)
	}
}

func TestTheClosedListOutlivesARestartAndLeavesWithItsMachine(t *testing.T) {
	dir := t.TempDir()
	m := OpenMemory(dir)
	observe(m, "gog", false, agentPane("w1:p1", "claude", "/work/api"))
	observe(m, "gog", false)
	again := OpenMemory(dir)
	if closed := again.Closed(); len(closed) != 1 || closed[0].Pane != "w1:p1" {
		t.Fatalf("read back: %+v", closed)
	}
	again.Forget("gog")
	if closed := OpenMemory(dir).Closed(); len(closed) != 0 {
		t.Fatalf("a machine removed takes its closed agents along: %+v", closed)
	}
}

// fakePlacer is herdr's client as reopening sees it.
type fakePlacer struct {
	snap    *herdr.Snapshot
	started []string
	tabs    []string
	made    []string
	fail    error
}

func (f *fakePlacer) SessionSnapshot() (*herdr.Snapshot, error) { return f.snap, nil }
func (f *fakePlacer) AgentStart(paneID, kind string) error {
	return f.AgentStartWith(paneID, kind, kind, nil, 0)
}
func (f *fakePlacer) AgentStartWith(paneID, kind, name string, args []string, timeout time.Duration) error {
	f.started = append(f.started, fmt.Sprintf("%s %s %v", paneID, kind, args))
	return f.fail
}
func (f *fakePlacer) WorkspaceCreate(cwd, label string) (*herdr.WorkspaceCreateResult, error) {
	f.made = append(f.made, cwd)
	res := &herdr.WorkspaceCreateResult{}
	res.Workspace.WorkspaceID = "w9"
	res.RootPane.PaneID, res.RootPane.TabID = "w9:p1", "w9:t1"
	return res, nil
}
func (f *fakePlacer) TabCreate(workspaceID, label string) (*herdr.TabCreateResult, error) {
	f.tabs = append(f.tabs, workspaceID+" "+label)
	res := &herdr.TabCreateResult{}
	res.Tab.TabID = workspaceID + ":t7"
	res.RootPane.PaneID = workspaceID + ":p7"
	return res, nil
}

func closedOne(t *testing.T, r *Restarter) Closed {
	t.Helper()
	observe(r.Memory, "local", false, named(agentPane("w1:p1", "claude", "/work/api"), "w1:t1", "Fix the queue"))
	r.Memory.Observe(events.Snapshot{Machine: "local", Snap: snapshot(), At: time.Now()})
	closed := r.Memory.Closed()
	if len(closed) != 1 {
		t.Fatalf("closed: %+v", closed)
	}
	return closed[0]
}

func TestReopenResumesInATabOfTheFoldersWorkspace(t *testing.T) {
	r := &Restarter{Memory: OpenMemory("")}
	c := closedOne(t, r)
	c.Tab = "review"
	client := &fakePlacer{snap: &herdr.Snapshot{
		Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1", Label: "api", Worktree: &herdr.WorktreeInfo{CheckoutPath: "/work/api"}}},
	}}
	place, err := r.reopenWith(client, "local", c)
	if err != nil {
		t.Fatal(err)
	}
	if place.Pane != "w1:p7" || len(client.tabs) != 1 || client.tabs[0] != "w1 review" || len(client.made) != 0 {
		t.Fatalf("place %+v, tabs %v, made %v", place, client.tabs, client.made)
	}
	if len(client.started) != 1 || client.started[0] != "w1:p7 claude [--resume "+sid+"]" {
		t.Fatalf("started: %v", client.started)
	}
	if len(r.Memory.Closed()) != 0 {
		t.Fatal("a reopened agent leaves the list")
	}
}

func TestReopenMakesAWorkspaceWhenTheFoldersIsGone(t *testing.T) {
	r := &Restarter{Memory: OpenMemory("")}
	c := closedOne(t, r)
	client := &fakePlacer{snap: &herdr.Snapshot{}}
	place, err := r.reopenWith(client, "local", c)
	if err != nil || place.Pane != "w9:p1" || len(client.made) != 1 || client.made[0] != "/work/api" {
		t.Fatalf("place %+v, err %v, made %v", place, err, client.made)
	}
}

func TestReopenOfAConversationOpenAlreadyNamesItsPane(t *testing.T) {
	r := &Restarter{Memory: OpenMemory("")}
	c := closedOne(t, r)
	client := &fakePlacer{snap: snapshot(agentPane("w4:p2", "claude", "/work/api"))}
	_, err := r.reopenWith(client, "local", c)
	var open *OpenAlready
	if !errors.As(err, &open) || open.Pane != "w4:p2" || len(client.started) != 0 {
		t.Fatalf("err %v, started %v", err, client.started)
	}
	if len(r.Memory.Closed()) != 0 {
		t.Fatal("an entry whose conversation runs again leaves the list")
	}
}

func TestAReopenThatFailsKeepsTheEntry(t *testing.T) {
	r := &Restarter{Memory: OpenMemory("")}
	c := closedOne(t, r)
	client := &fakePlacer{snap: &herdr.Snapshot{}, fail: errors.New("agent_not_ready")}
	if _, err := r.reopenWith(client, "local", c); err == nil {
		t.Fatal("a failed start is an error")
	}
	if len(r.Memory.Closed()) != 1 {
		t.Fatal("the entry stays for another try")
	}
}
