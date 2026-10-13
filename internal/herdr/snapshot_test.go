package herdr

import "testing"

func TestSnapshotQuestions(t *testing.T) {
	snap := &Snapshot{
		Workspaces: []WorkspaceInfo{
			{WorkspaceID: "w1", Label: "api"},
			{WorkspaceID: "w2", Label: "tree", Worktree: &WorktreeInfo{CheckoutPath: "/repo/.worktrees/x"}},
			{WorkspaceID: "w3", Label: "empty"},
		},
		Panes: []PaneInfo{
			{PaneID: "w1:p1", WorkspaceID: "w1", CWD: "/repo"},
			{PaneID: "w1:p2", WorkspaceID: "w1", CWD: "/repo", Agent: "claude"},
			{PaneID: "w2:p1", WorkspaceID: "w2", ForegroundCWD: "/elsewhere", Agent: "codex"},
		},
	}
	if snap.Pane("w1:p2") == nil || snap.Pane("nope") != nil {
		t.Fatal("Pane")
	}
	if snap.WorkspaceLabel("w2") != "tree" || snap.WorkspaceLabel("w9") != "" {
		t.Fatal("WorkspaceLabel")
	}
	if dirs := snap.WorkspaceDirs(); len(dirs) != 2 || dirs[0] != "/repo" || dirs[1] != "/repo/.worktrees/x" {
		t.Fatalf("WorkspaceDirs: %v", dirs)
	}
	if pane := snap.AgentPaneIn("/repo"); pane == nil || pane.PaneID != "w1:p2" {
		t.Fatalf("AgentPaneIn: %+v", pane)
	}
	if snap.AgentPaneIn("/nowhere") != nil {
		t.Fatal("a folder without a workspace has no agent pane")
	}
}
