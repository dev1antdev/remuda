package review

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/wintest"
)

// gitRepo makes a repository with one commit and one change on top, as an agent's folder on the PC would be.
func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "check@example.invalid")
	run("config", "user.name", "check")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "first")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestOnWindowsTheChangesLensReadsAWindowsRepository runs the review's summary, a file's diff, the sweep's badges and
// the secret check over the runner's OpenSSH on a repository named in Windows' spelling, as herdr names it.
func TestOnWindowsTheChangesLensReadsAWindowsRepository(t *testing.T) {
	wintest.Live(t)
	host := wintest.Host(t)
	dir := gitRepo(t)
	store := New(t.TempDir())
	summary := store.Summary(host, dir)
	if summary.Error != "" || summary.Branch != "main" || len(summary.Files) != 2 {
		t.Fatalf("the summary of %s: %+v", dir, summary)
	}
	diff, err := store.Diff(host, dir, "main.go")
	if err != nil || !strings.Contains(diff, "+func main() {}") {
		t.Fatalf("the diff: %v\n%s", err, diff)
	}
	store.Badges(host, []string{dir})
	if badge, ok := store.Badge(host.ID(), dir); !ok || badge.Changed != 2 {
		t.Fatalf("the sweep's badge: %+v, %v", badge, ok)
	}
	root := store.Root(host.ID(), dir)
	if !strings.Contains(root, `\`) || strings.HasPrefix(root, "/") {
		t.Fatalf("the repository's folder in Windows' spelling, as the sidebar groups by: %q", root)
	}
	scan, err := ScanWork(host, dir, false)
	if err != nil || len(scan.Findings) != 0 || scan.Files != 2 {
		t.Fatalf("the secret check: %v %+v", err, scan)
	}
}
