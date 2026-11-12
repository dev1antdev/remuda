package app

import (
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestCheckoutDirsLeaveTheLinkedWorktreesOut(t *testing.T) {
	snap := &herdr.Snapshot{Workspaces: []herdr.WorkspaceInfo{
		{WorkspaceID: "w1", Worktree: &herdr.WorktreeInfo{CheckoutPath: "/home/me/app/"}},
		{WorkspaceID: "w2", Worktree: &herdr.WorktreeInfo{CheckoutPath: "/home/me/.herdr/worktrees/app/task-3", IsLinkedWorktree: true}},
		{WorkspaceID: "w3", Worktree: &herdr.WorktreeInfo{CheckoutPath: "/home/me/app"}},
		{WorkspaceID: "w4", Worktree: &herdr.WorktreeInfo{CheckoutPath: "/home/me/site"}},
	}}
	got := checkoutDirs(snap)
	if len(got) != 2 || got[0] != (workspaceDir{"/home/me/app", "w1"}) || got[1] != (workspaceDir{"/home/me/site", "w4"}) {
		t.Fatalf("%+v", got)
	}
	if checkoutDirs(nil) != nil {
		t.Fatal("no snapshot, folders")
	}
}

func TestPickDirTakesTheHomeFolderElseTheSameRepository(t *testing.T) {
	dirs := []workspaceDir{{"/srv/site", "w1"}, {"/srv/app-clone", "w2"}, {"/srv/app", "w3"}}
	repositories := map[string]string{"/srv/site": "github.com/team/site", "/srv/app-clone": "github.com/team/app", "/srv/app": "github.com/team/app"}
	of := func(dir string) string { return repositories[dir] }
	if at := pickDir(dirs, true, "/srv/app", "github.com/team/app", of); at == nil || at.workspace != "w3" {
		t.Fatalf("home: %+v", at)
	}
	if at := pickDir(dirs, false, "/srv/app", "github.com/team/app", of); at == nil || at.workspace != "w2" {
		t.Fatalf("elsewhere: %+v", at)
	}
	if at := pickDir(dirs, false, "/srv/app", "", of); at != nil {
		t.Fatalf("a repository of no remote matched elsewhere: %+v", at)
	}
	if at := pickDir(dirs, true, "/srv/app", "", of); at == nil || at.workspace != "w3" {
		t.Fatalf("home with no remote: %+v", at)
	}
	if at := pickDir(dirs, false, "/srv/app", "github.com/team/other", of); at != nil {
		t.Fatalf("another repository matched: %+v", at)
	}
}
