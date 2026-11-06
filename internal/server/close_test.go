package server

import (
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestAWorktreeAnotherWorkspaceUsesIsLeftStanding(t *testing.T) {
	tree := "/home/me/.herdr/worktrees/app/task-1"
	snap := &herdr.Snapshot{
		Workspaces: []herdr.WorkspaceInfo{
			{WorkspaceID: "w1", Worktree: &herdr.WorktreeInfo{CheckoutPath: tree, IsLinkedWorktree: true}},
			{WorkspaceID: "w2", Worktree: &herdr.WorktreeInfo{CheckoutPath: "/home/me/code/app"}},
		},
		Panes: []herdr.PaneInfo{
			{PaneID: "w1:p1", WorkspaceID: "w1", CWD: tree},
			{PaneID: "w2:p1", WorkspaceID: "w2", CWD: "/home/me/code/app"},
		},
	}
	if usedElsewhere(snap, "w1", tree) {
		t.Fatal("the workspace's own pane counted as another's")
	}
	snap.Panes = append(snap.Panes, herdr.PaneInfo{PaneID: "w2:p2", WorkspaceID: "w2", CWD: "/home/me/code/app", ForegroundCWD: tree + "/src"})
	if !usedElsewhere(snap, "w1", tree) {
		t.Fatal("a pane of another workspace inside the worktree went unseen")
	}
	snap.Panes = snap.Panes[:2]
	snap.Workspaces = append(snap.Workspaces, herdr.WorkspaceInfo{WorkspaceID: "w3", Worktree: &herdr.WorktreeInfo{CheckoutPath: tree, IsLinkedWorktree: true}})
	if !usedElsewhere(snap, "w1", tree) {
		t.Fatal("a second workspace on the same worktree went unseen")
	}
	if usedElsewhere(snap, "w1", "/home/me/.herdr/worktrees/app/task") {
		t.Fatal("a folder whose name is a prefix of another's counted as inside it")
	}
}
