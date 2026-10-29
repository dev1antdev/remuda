package worktrees

import (
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
)

func TestInspectReadsTheWorktreeAndRefusesNothingItself(t *testing.T) {
	host := &shell.Fake{Name: "box", Answer: func(string) (string, error) { return "task-20-pr-to-end\n/home/me/repo\n M internal/x.go\n", nil }}
	info, err := Inspect(host, "/home/me/repo/.worktrees/task-20")
	if err != nil {
		t.Fatal(err)
	}
	if info.Path != "/home/me/repo/.worktrees/task-20" || info.Branch != "task-20-pr-to-end" || info.Repo != "/home/me/repo" || info.Dirty != "M internal/x.go" {
		t.Fatalf("info = %+v", info)
	}
	if s := host.Scripts()[0]; !strings.Contains(s, "cd '/home/me/repo/.worktrees/task-20'") || !strings.Contains(s, "git status --porcelain") || strings.Contains(s, "--force") {
		t.Fatalf("script = %q", s)
	}

	clean, _ := ParseInspect("/w", "HEAD\n/repo\n")
	if clean.Branch != "" || clean.Dirty != "" || clean.Repo != "/repo" {
		t.Fatalf("a detached, clean worktree: %+v", clean)
	}
	if _, err := ParseInspect("/w", "the folder is not there\n"); err == nil || err.Error() != "the folder is not there" {
		t.Fatalf("err = %v", err)
	}
	gone := &shell.Fake{Name: "box", Answer: func(string) (string, error) { return "not a git worktree\n", &shell.ExitError{Code: 3} }}
	if _, err := Inspect(gone, "/w"); err == nil || err.Error() != "not a git worktree" {
		t.Fatalf("err = %v", err)
	}
}

func TestRemoveNeverForcesAndDeletesTheBranchOnlyAsAsked(t *testing.T) {
	info := Info{Path: "/repo/.worktrees/task-20", Branch: "task-20-x", Repo: "/repo"}
	plain := RemoveScript(info, false)
	if !strings.Contains(plain, "cd '/repo'") || !strings.Contains(plain, "git worktree remove '/repo/.worktrees/task-20'") || !strings.Contains(plain, "git branch -d 'task-20-x'") || strings.Contains(plain, "--force") || strings.Contains(plain, "-D") {
		t.Fatalf("script = %q", plain)
	}
	merged := RemoveScript(info, true)
	if !strings.Contains(merged, "git branch -D 'task-20-x'") || strings.Contains(merged, "--force") {
		t.Fatalf("merged script = %q", merged)
	}
	if detached := RemoveScript(Info{Path: "/w", Repo: "/repo"}, true); strings.Contains(detached, "git branch") {
		t.Fatalf("a detached worktree has no branch to delete: %q", detached)
	}

	refused := &shell.Fake{Name: "box", Answer: func(string) (string, error) {
		return "fatal: '/repo/.worktrees/task-20' contains modified or untracked files, use --force to delete it\n", &shell.ExitError{Code: 4}
	}}
	if err := Remove(refused, info, false); err == nil || !strings.Contains(err.Error(), "contains modified or untracked files") {
		t.Fatalf("err = %v", err)
	}
	unmerged := &shell.Fake{Name: "box", Answer: func(string) (string, error) {
		return "error: the branch 'task-20-x' is not fully merged\nhint: If you are sure you want to delete it, run 'git branch -D task-20-x'\n", &shell.ExitError{Code: 5}
	}}
	err := Remove(unmerged, info, false)
	if err == nil || !strings.HasPrefix(err.Error(), "the worktree is removed; the branch stays: ") {
		t.Fatalf("err = %v", err)
	}
	ok := &shell.Fake{Name: "box", Answer: func(string) (string, error) { return "Deleted branch task-20-x (was abc1234).\n", nil }}
	if err := Remove(ok, info, true); err != nil {
		t.Fatal(err)
	}
}
