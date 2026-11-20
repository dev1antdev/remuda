package core_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/core/coretest"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/shell"
)

// gitRepo is a repository with one commit in a HOME of the test's own, git told nothing of the person's config.
func gitRepo(t *testing.T) (home, repo string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	home = t.TempDir()
	if real, err := filepath.EvalSymlinks(home); err == nil {
		home = real // git names folders by their real path (a Mac's /var is /private/var)
	}
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "no-config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	repo = filepath.Join(home, "code", "app")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "a.txt")
	git(t, repo, "commit", "-qm", "init")
	return home, repo
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestWorktreesAreMadeAndOpenedOnACoreMachineWhereHerdrPutsThem(t *testing.T) {
	home, repo := gitRepo(t)
	client := coretest.Server(t).Client(t)
	adapter := adapterOf(t, client, core.AdapterHooks{Shell: shell.Local{Name: "local"}})
	eventually(t, adapter, func(*herdr.Snapshot) bool { return true })

	made, err := adapter.WorkspaceCreate(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	source := made.Workspace.WorkspaceID
	want := herdr.WorktreeInfo{CheckoutPath: repo, RepoKey: filepath.Join(repo, ".git"), RepoName: "app", RepoRoot: repo}
	if tree := made.Workspace.Worktree; tree == nil || *tree != want {
		t.Fatalf("the main checkout reads as %+v, want %+v", tree, want)
	}

	created, err := adapter.WorktreeCreate(source, "feat/x", "", "")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".herdr", "worktrees", "app", "feat-x")
	if created.Worktree.Path != path || created.Worktree.Branch != "feat/x" {
		t.Fatalf("made %+v, want it at %s", created.Worktree, path)
	}
	if got := git(t, path, "branch", "--show-current"); got != "feat/x" {
		t.Errorf("the worktree is on %q", got)
	}
	snap, _ := adapter.SessionSnapshot()
	ws := snap.Workspace(created.Workspace.WorkspaceID)
	if ws == nil || ws.Worktree == nil || !ws.Worktree.IsLinkedWorktree || ws.Worktree.RepoRoot != repo || snap.WorkspaceDir(ws) != path {
		t.Fatalf("the worktree's workspace is %+v", ws)
	}
	if created.RootPane.WorkspaceID != ws.WorkspaceID || snap.Pane(created.RootPane.PaneID) == nil {
		t.Errorf("its pane %+v is not in the snapshot", created.RootPane)
	}

	git(t, repo, "branch", "kept")
	other := filepath.Join(home, "elsewhere", "kept")
	again, err := adapter.WorktreeCreateAt(source, "kept", "", "mine", other)
	if err != nil {
		t.Fatal(err)
	}
	if got := git(t, other, "branch", "--show-current"); got != "kept" || again.Workspace.Label != "mine" {
		t.Errorf("an existing branch in a folder of the caller's: on %q, labelled %q", got, again.Workspace.Label)
	}

	if _, err := adapter.WorktreeCreate(source, "feat/x", "", ""); err == nil || !strings.Contains(err.Error(), "git worktree add") {
		t.Errorf("a branch checked out already was not refused in git's words: %v", err)
	}

	open, err := adapter.WorktreeOpen(source, path, "")
	if err != nil || !open.AlreadyOpen || open.Workspace.WorkspaceID != ws.WorkspaceID {
		t.Errorf("opening a checkout open already answered %+v, %v", open, err)
	}
	if _, err := adapter.WorktreeOpen(source, t.TempDir(), ""); err == nil {
		t.Error("a folder outside git was opened as a checkout")
	}
}

func TestAWorkspaceOutsideGitHasNoCheckoutAndOneWithoutAShellMakesNoWorktree(t *testing.T) {
	_, repo := gitRepo(t)
	client := coretest.Server(t).Client(t)
	withShell := adapterOf(t, client, core.AdapterHooks{Shell: shell.Local{Name: "local"}})
	eventually(t, withShell, func(*herdr.Snapshot) bool { return true })

	plain, err := withShell.WorkspaceCreate(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if plain.Workspace.Worktree != nil {
		t.Errorf("a folder outside git reads as a checkout: %+v", plain.Workspace.Worktree)
	}

	without := adapterOf(t, client, core.AdapterHooks{})
	eventually(t, without, func(*herdr.Snapshot) bool { return true })
	made, err := without.WorkspaceCreate(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := without.WorktreeCreate(made.Workspace.WorkspaceID, "y", "", ""); err == nil {
		t.Error("a worktree was made with no shell to run git")
	}
}
