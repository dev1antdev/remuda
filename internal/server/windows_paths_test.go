package server

import (
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestAWindowsRepositorysFolderIsReadOffItsSharedGitDir(t *testing.T) {
	tree := &herdr.WorktreeInfo{RepoKey: `C:\Users\me\app\.git`}
	if got := FolderOf(tree, "", `C:\Users\me\.herdr\worktrees\app\feature`); got != `C:\Users\me\app` {
		t.Fatalf("FolderOf = %q", got)
	}
	if got := FolderOf(&herdr.WorktreeInfo{RepoKey: "/home/me/app/.git"}, "", "/x"); got != "/home/me/app" {
		t.Fatalf("a POSIX one = %q", got)
	}
}
