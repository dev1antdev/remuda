package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/text"
)

func TestAWorktreeWhoseWorkspaceDidNotOpenIsTakenDownWithTheBranchItMade(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "no-config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	repo := filepath.Join(home, "app")
	git := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", repo, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	git("init", "-q", "-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "init")
	git("branch", "theirs")
	sh := shell.Local{Name: "local"}

	for _, c := range []struct{ branch, base string }{{"mine", "main"}, {"theirs", ""}} {
		out, err := shell.Text(sh, addScript(repo, c.branch, c.base, ""), worktreeWithin)
		if err != nil {
			t.Fatalf("%s: %v %s", c.branch, err, out)
		}
		path, _ := parseCheckout(out)
		if out, err := shell.Text(sh, undoScript(repo, path, c.branch, fieldsOf(out)["made"]), checkoutWithin); err != nil {
			t.Fatalf("%s: undo %v %s", c.branch, err, out)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s: the checkout stays at %s", c.branch, path)
		}
	}
	if branches := git("branch", "--format=%(refname:short)"); branches != "main\ntheirs" {
		t.Errorf("the branch made is not gone, or the one there before went: %q", branches)
	}
}

func TestAWorktreeAFailingHookLeftIsReportedForTakingDownAndABranchInUseIsKept(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "no-config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	repo := filepath.Join(home, "app")
	git := func(dir string, args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if err := os.MkdirAll(filepath.Join(repo, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	git(repo, "init", "-q", "-b", "main")
	git(repo, "commit", "-q", "--allow-empty", "-m", "init")
	hook := filepath.Join(repo, ".git", "hooks", "post-checkout")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho 'the hook says no'\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sh := shell.Local{Name: "local"}

	out, err := shell.Text(sh, addScript(repo, "hooked", "main", ""), worktreeWithin)
	path, _ := parseCheckout(out)
	if err == nil || path == "" || fieldsOf(out)["made"] == "" || !strings.Contains(text.LastLine(out, ""), "the hook says no") {
		t.Fatalf("a checkout a failing hook left is not reported for taking down: %v\n%s", err, out)
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}

	git(path, "checkout", "-q", "--detach")
	other := filepath.Join(home, "other")
	git(repo, "worktree", "add", "-q", other, "hooked")
	if out, err := shell.Text(sh, undoScript(repo, path, "hooked", fieldsOf(out)["made"]), checkoutWithin); err == nil || !strings.Contains(out, "kept") {
		t.Fatalf("a branch another worktree uses was not kept: %v %s", err, out)
	}
	if got := git(other, "symbolic-ref", "--short", "HEAD"); got != "hooked" {
		t.Errorf("the other worktree's branch is %q", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the checkout stays at %s", path)
	}
}
