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
	if s := host.Scripts()[0]; !strings.Contains(s, "cd '/home/me/repo/.worktrees/task-20'") || !strings.Contains(s, " status --porcelain") || strings.Contains(s, "--force") {
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
	plain := RemoveScript(info, nil)
	if !strings.Contains(plain, "cd '/repo'") || !strings.Contains(plain, "git worktree remove '/repo/.worktrees/task-20'") || !strings.Contains(plain, "git branch -d 'task-20-x'") || strings.Contains(plain, "--force") || strings.Contains(plain, "-D") {
		t.Fatalf("script = %q", plain)
	}
	merged := RemoveScript(info, []Landed{{Branch: "task-20-x", Commit: "abc123"}})
	if !strings.Contains(merged, `git update-ref --no-deref -d 'refs/heads/task-20-x' "$tip"`) || !strings.Contains(merged, "' abc123 '") || strings.Contains(merged, "--force") {
		t.Fatalf("merged script = %q", merged)
	}
	if detached := RemoveScript(Info{Path: "/w", Repo: "/repo"}, []Landed{{Branch: "x", Commit: "abc"}}); strings.Contains(detached, "git branch") {
		t.Fatalf("a detached worktree has no branch to delete: %q", detached)
	}
}

func TestGitsComplaintIsTheRefusalNotItsHint(t *testing.T) {
	got := lastLine("error: the branch 'x' is not fully merged\nhint: If you are sure, run 'git branch -D x'\nhint: Disable this message\n", "none")
	if got != "error: the branch 'x' is not fully merged" {
		t.Errorf("got %q", got)
	}
	if got := lastLine(" \n", "none"); got != "none" {
		t.Errorf("nothing said reads %q", got)
	}
}

func TestDiscardForcesOnlyItsOwnScript(t *testing.T) {
	info := Info{Path: "/repo/.worktrees/arena-3-2", Branch: "arena-3-2", Repo: "/repo"}
	s := DiscardScript(info)
	if !strings.Contains(s, "git worktree remove --force '/repo/.worktrees/arena-3-2'") || !strings.Contains(s, "git branch -D 'arena-3-2'") {
		t.Fatalf("script = %q", s)
	}
	refused := &shell.Fake{Name: "box", Answer: func(string) (string, error) {
		return "fatal: '/repo/.worktrees/arena-3-2' is locked\n", &shell.ExitError{Code: 4}
	}}
	if err := Discard(refused, info); err == nil || !strings.Contains(err.Error(), "is locked") {
		t.Fatalf("err = %v", err)
	}
}
