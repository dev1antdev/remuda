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

func TestACheckoutAFailingHookLeftIsNamedAndLeftAsItIs(t *testing.T) {
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
	left := fieldsOf(out)["left"]
	if err == nil || left == "" || !strings.Contains(text.LastLine(out, ""), "the hook says no") {
		t.Fatalf("a checkout a failing hook left is not named: %v\n%s", err, out)
	}
	if _, err := os.Stat(left); err != nil {
		t.Errorf("the checkout left was touched: %v", err)
	}
}
