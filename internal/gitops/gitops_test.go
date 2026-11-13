package gitops

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
)

func needGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	// the person's own git settings (a signing key, hooks, a template) never reach the tests' repositories
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Ada")
	t.Setenv("GIT_AUTHOR_EMAIL", "ada@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Ada")
	t.Setenv("GIT_COMMITTER_EMAIL", "ada@example.com")
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, path, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, path), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, dir, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil {
		return "<gone>"
	}
	return string(data)
}

// repository is main with two commits and a branch "side" one commit off the first.
func repository(t *testing.T) string {
	t.Helper()
	needGit(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	run(t, dir, "init", "-q", "-b", "main")
	write(t, dir, "a.txt", "one\n")
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-qm", "first")
	run(t, dir, "checkout", "-qb", "side")
	write(t, dir, "b.txt", "side\n")
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-qm", "side")
	run(t, dir, "checkout", "-q", "main")
	write(t, dir, "a.txt", "one\ntwo\n")
	run(t, dir, "commit", "-qam", "second")
	return dir
}

var host = shell.Local{Name: "local"}

// seen is a request as the page sends it: HEAD and the branch as they stand now.
func seen(t *testing.T, dir string, req Request) Request {
	t.Helper()
	req.Head = run(t, dir, "rev-parse", "HEAD")
	cmd := exec.Command("git", "symbolic-ref", "-q", "--short", "HEAD")
	cmd.Dir = dir
	out, _ := cmd.Output()
	req.Branch = strings.TrimSpace(string(out))
	return req
}

func do(t *testing.T, dir string, req Request) Result {
	t.Helper()
	result, err := (&Service{}).Run(host, dir, seen(t, dir, req))
	if err != nil {
		t.Fatalf("%s: %v", req.Op, err)
	}
	return result
}

func TestCheckoutsBranchesAndTags(t *testing.T) {
	dir := repository(t)
	if r := do(t, dir, Request{Op: "checkout", Ref: "side"}); !r.OK || run(t, dir, "symbolic-ref", "--short", "HEAD") != "side" {
		t.Fatalf("checkout: %+v", r)
	}
	first := run(t, dir, "rev-parse", "main~1")
	if r := do(t, dir, Request{Op: "branch-create", Name: "fix/x", Sha: first, Checkout: true}); !r.OK || run(t, dir, "symbolic-ref", "--short", "HEAD") != "fix/x" {
		t.Fatalf("a branch made and checked out: %+v", r)
	}
	if r := do(t, dir, Request{Op: "branch-rename", Ref: "fix/x", Name: "fix/y"}); !r.OK || run(t, dir, "symbolic-ref", "--short", "HEAD") != "fix/y" {
		t.Fatalf("a rename: %+v", r)
	}
	if r := do(t, dir, Request{Op: "tag-add", Name: "v1", Sha: first, Message: "the first"}); !r.OK || run(t, dir, "cat-file", "-t", "v1") != "tag" {
		t.Fatalf("an annotated tag: %+v", r)
	}
	if r := do(t, dir, Request{Op: "tag-delete", Ref: "v1"}); !r.OK || run(t, dir, "tag") != "" {
		t.Fatalf("a tag deleted: %+v", r)
	}
	if r := do(t, dir, Request{Op: "checkout-commit", Sha: first}); !r.OK {
		t.Fatalf("a detached checkout: %+v", r)
	}
	if r := do(t, dir, Request{Op: "checkout", Ref: "main"}); !r.OK {
		t.Fatalf("back to main: %+v", r)
	}
	run(t, dir, "remote", "add", "origin", "https://example.invalid/app.git") // never reached: nothing here fetches
	run(t, dir, "update-ref", "refs/remotes/origin/topic", "side")
	if r := do(t, dir, Request{Op: "checkout-remote", Ref: "origin/topic", Name: "topic"}); !r.OK || run(t, dir, "rev-parse", "--abbrev-ref", "topic@{upstream}") != "origin/topic" {
		t.Fatalf("a remote branch checked out as a tracking one: %+v", r)
	}
}

func TestDeletingAnUnmergedBranchNeedsForceAndSaysWhy(t *testing.T) {
	dir := repository(t)
	r := do(t, dir, Request{Op: "branch-delete", Ref: "side"})
	if r.OK || !r.NotMerged || !strings.Contains(r.Output, "side") {
		t.Fatalf("-d of an unmerged branch: %+v", r)
	}
	if forced := do(t, dir, Request{Op: "branch-delete", Ref: "side", Force: true}); !forced.OK || strings.Contains(run(t, dir, "branch"), "side") {
		t.Fatalf("-D: %+v", forced)
	}
}

func TestMergeRebaseCherryPickRevertAndReset(t *testing.T) {
	dir := repository(t)
	if r := do(t, dir, Request{Op: "merge", Ref: "side", NoFF: true}); !r.OK || read(t, dir, "b.txt") != "side\n" || run(t, dir, "rev-list", "--count", "--merges", "HEAD") != "1" {
		t.Fatalf("a merge: %+v", r)
	}
	merge := run(t, dir, "rev-parse", "HEAD")
	if r := do(t, dir, Request{Op: "revert", Sha: merge}); !r.OK || read(t, dir, "b.txt") != "<gone>" {
		t.Fatalf("a revert of the merge, against its first parent: %+v", r)
	}
	if r := do(t, dir, Request{Op: "reset", Sha: merge, Mode: "hard"}); !r.OK || read(t, dir, "b.txt") != "side\n" {
		t.Fatalf("a hard reset: %+v", r)
	}
	write(t, dir, "c.txt", "third\n")
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-qm", "third")
	third := run(t, dir, "rev-parse", "HEAD")
	run(t, dir, "checkout", "-q", "side")
	if r := do(t, dir, Request{Op: "rebase", Sha: run(t, dir, "rev-parse", "main~2")}); !r.OK || read(t, dir, "a.txt") != "one\ntwo\n" || read(t, dir, "b.txt") != "side\n" {
		t.Fatalf("side rebased on the second commit: %+v", r)
	}
	if r := do(t, dir, Request{Op: "cherry-pick", Sha: third}); !r.OK || read(t, dir, "c.txt") != "third\n" {
		t.Fatalf("a cherry-pick: %+v", r)
	}
	if r := do(t, dir, Request{Op: "reset", Sha: run(t, dir, "rev-parse", "HEAD~1"), Mode: "soft"}); !r.OK || run(t, dir, "diff", "--cached", "--name-only") != "c.txt" {
		t.Fatalf("a soft reset keeps the change staged: %+v", r)
	}
}

func TestAMergeThatStopsOnConflictsIsLeftInProgressAndAborted(t *testing.T) {
	dir := repository(t)
	run(t, dir, "checkout", "-q", "side")
	write(t, dir, "a.txt", "theirs\n")
	run(t, dir, "commit", "-qam", "clash")
	run(t, dir, "checkout", "-q", "main")
	r := do(t, dir, Request{Op: "merge", Ref: "side"})
	if r.OK || r.Operation != "merge" || r.Conflicts != 1 || !strings.Contains(r.Output, "CONFLICT") {
		t.Fatalf("a merge on a conflict: %+v", r)
	}
	if _, err := (&Service{}).Run(host, dir, Request{Op: "abort", Operation: "rebase"}); !errors.Is(err, ErrStale) {
		t.Fatalf("aborting another op than the one stopped: %v", err)
	}
	if aborted := do(t, dir, Request{Op: "abort", Operation: "merge"}); !aborted.OK || aborted.Operation != "" || read(t, dir, "a.txt") != "one\ntwo\n" {
		t.Fatalf("the abort: %+v", aborted)
	}
}

func TestARequestFromAnOlderLookOrUnderAnAgentsFeetIsRefused(t *testing.T) {
	dir := repository(t)
	old := seen(t, dir, Request{Op: "reset", Sha: run(t, dir, "rev-parse", "HEAD~1"), Mode: "hard"})
	write(t, dir, "a.txt", "moved on\n")
	run(t, dir, "commit", "-qam", "moved on")
	if _, err := (&Service{}).Run(host, dir, old); !errors.Is(err, ErrStale) {
		t.Fatalf("a reset drawn before a commit landed: %v", err)
	}
	if read(t, dir, "a.txt") != "moved on\n" {
		t.Fatal("the stale reset ran")
	}
	var asked struct{ top string }
	busy := &Service{Busy: func(machineID, top string, worktrees []string) string {
		asked.top = top
		return "Claude Code is at work"
	}}
	_, err := busy.Run(host, dir, seen(t, dir, Request{Op: "checkout", Ref: "side"}))
	var refused *BusyError
	if !errors.As(err, &refused) || asked.top != dir || !strings.Contains(err.Error(), "Claude Code") {
		t.Fatalf("an op while an agent works: %v (asked for %q)", err, asked.top)
	}
	if run(t, dir, "symbolic-ref", "--short", "HEAD") != "main" {
		t.Fatal("the refused checkout ran")
	}
}

func TestNamesAndHashesThatGitWouldReadOtherwiseAreRefused(t *testing.T) {
	for _, req := range []Request{
		{Op: "checkout", Ref: "-f"},
		{Op: "checkout", Ref: "a b"},
		{Op: "branch-create", Name: "x..y", Sha: "abcdef1"},
		{Op: "branch-create", Name: "x", Sha: "HEAD"},
		{Op: "branch-rename", Ref: "main", Name: "bad~1"},
		{Op: "tag-add", Name: "v1.lock", Sha: "abcdef1"},
		{Op: "reset", Sha: "abcdef1", Mode: "keep"},
		{Op: "abort", Operation: "bisect"},
		{Op: "push", Ref: "main"},
		{Op: "fetch"},
		{Op: "checkout", Ref: "main", Head: "HEAD"},
	} {
		if err := Validate(req); err == nil {
			t.Errorf("%+v passed", req)
		}
	}
	if Validate(Request{Op: "merge", Sha: "abcdef1234"}) != nil || Validate(Request{Op: "checkout-remote", Ref: "origin/feature/x", Name: "feature/x"}) != nil {
		t.Fatal("a sound merge or remote checkout was refused")
	}
	if command := Command(Request{Op: "branch-delete", Ref: "x"}); !strings.Contains(command, "-d 'x'") || strings.Contains(command, "-D") {
		t.Fatalf("a delete without force runs %q", command)
	}
	for _, op := range []string{"checkout", "merge", "rebase", "reset", "cherry-pick", "revert"} {
		if strings.Contains(Command(Request{Op: op, Ref: "x", Sha: "abcdef1", Mode: "hard"}), "force") {
			t.Errorf("%s forces", op)
		}
	}
}
