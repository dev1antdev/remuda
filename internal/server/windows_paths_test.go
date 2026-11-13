package server

import (
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestAViewerReadsAWindowsMachinesWorkspaceFilesOnly(t *testing.T) {
	folders := []string{`C:\Users\me\app\`} // herdr names a folder this way
	for path, want := range map[string]bool{
		`C:\Users\me\app\src\main.go`:      true,
		`c:\users\me\APP\README.md`:        true,
		`C:\Users\me\app`:                  true,
		`C:\Users\me\application\x.go`:     false,
		`C:\Users\me\.ssh\id_ed25519`:      false,
		`C:\Users\me\app\..\.ssh\id_rsa`:   false,
		`C:\Users\me\AppData\Roaming\x.go`: false,
	} {
		if got := withinFolders(path, folders, false); got != want {
			t.Errorf("withinFolders(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestAWindowsRepositorysFolderIsReadOffItsSharedGitDir(t *testing.T) {
	tree := &herdr.WorktreeInfo{RepoKey: `C:\Users\me\app\.git`}
	if got := FolderOf(tree, "", `C:\Users\me\.herdr\worktrees\app\feature`); got != `C:\Users\me\app` {
		t.Fatalf("FolderOf = %q", got)
	}
	if got := FolderOf(&herdr.WorktreeInfo{RepoKey: "/home/me/app/.git"}, "", "/x"); got != "/home/me/app" {
		t.Fatalf("a POSIX one = %q", got)
	}
}
