package collisions

import (
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestAWorkspaceIsWeighedByItsAgentsAndItsWork(t *testing.T) {
	snap := &herdr.Snapshot{
		Workspaces: []herdr.WorkspaceInfo{
			{WorkspaceID: "w1", Label: "web", Worktree: &herdr.WorktreeInfo{CheckoutPath: "/r/web"}},
			{WorkspaceID: "w2", Label: "notes"},
			{WorkspaceID: "w3", Label: "empty"},
		},
		Panes: []herdr.PaneInfo{
			{PaneID: "w1:p1", WorkspaceID: "w1", CWD: "/r/web"},
			{PaneID: "w1:p2", WorkspaceID: "w1", Agent: "claude", AgentStatus: herdr.StatusIdle},
			{PaneID: "w1:p3", WorkspaceID: "w1", Agent: "codex", AgentStatus: herdr.StatusWorking},
			{PaneID: "w2:p1", WorkspaceID: "w2", CWD: "/notes"},
		},
	}
	workspaces := WorkspacesOf("local", snap, Lookup{
		Repo:    func(dir string) string { return map[string]string{"/r/web": "remote r"}[dir] },
		Changed: func(dir string) int { return map[string]int{"/notes": 4}[dir] },
		Branch:  func(dir string) string { return "b" + dir },
		OpenPR:  func(pane string) bool { return pane == "w1:p3" },
	})
	if len(workspaces) != 2 {
		t.Fatalf("a workspace with no folder is none: %+v", workspaces)
	}
	web, notes := workspaces[0], workspaces[1]
	if web.Repo != "remote r" || web.Pane != "w1:p2" || !web.Agents || !web.Working || !web.PR || web.Branch != "b/r/web" || !web.active() {
		t.Fatalf("web: %+v", web)
	}
	if notes.Agents || notes.Changed != 4 || notes.Repo != "" || notes.active() {
		t.Fatalf("a folder outside git is never active: %+v", notes)
	}
}
