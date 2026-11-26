package server

import (
	"testing"

	"github.com/dev1antdev/remuda/internal/coreapi"
)

func TestAWorktreeAnotherWorkspaceUsesIsLeftStanding(t *testing.T) {
	tree := "/home/me/.herdr/worktrees/app/task-1"
	snap := &coreapi.Snapshot{
		Workspaces: []coreapi.WorkspaceInfo{
			{WorkspaceID: "w1", Worktree: &coreapi.WorktreeInfo{CheckoutPath: tree, IsLinkedWorktree: true}},
			{WorkspaceID: "w2", Worktree: &coreapi.WorktreeInfo{CheckoutPath: "/home/me/code/app"}},
		},
		Panes: []coreapi.PaneInfo{
			{PaneID: "w1:p1", WorkspaceID: "w1", CWD: tree},
			{PaneID: "w2:p1", WorkspaceID: "w2", CWD: "/home/me/code/app"},
		},
	}
	if usedElsewhere(snap, "w1", tree) {
		t.Fatal("the workspace's own pane counted as another's")
	}
	snap.Panes = append(snap.Panes, coreapi.PaneInfo{PaneID: "w2:p2", WorkspaceID: "w2", CWD: "/home/me/code/app", ForegroundCWD: tree + "/src"})
	if !usedElsewhere(snap, "w1", tree) {
		t.Fatal("a pane of another workspace inside the worktree went unseen")
	}
	snap.Panes = snap.Panes[:2]
	snap.Workspaces = append(snap.Workspaces, coreapi.WorkspaceInfo{WorkspaceID: "w3", Worktree: &coreapi.WorktreeInfo{CheckoutPath: tree, IsLinkedWorktree: true}})
	if !usedElsewhere(snap, "w1", tree) {
		t.Fatal("a second workspace on the same worktree went unseen")
	}
	if usedElsewhere(snap, "w1", "/home/me/.herdr/worktrees/app/task") {
		t.Fatal("a folder whose name is a prefix of another's counted as inside it")
	}
}
