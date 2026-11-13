package machines

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

func TestTheDiffOfAFileGoneWithItsFolderIsReadFromTheFolderStillThere(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@remuda.invalid", "-c", "user.name=t"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "old", "gone.txt"), []byte("an old line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("init", "-q")
	git("add", "-A")
	git("commit", "-q", "-m", "init")
	git("rm", "-q", "old/gone.txt")
	gone := filepath.Join(dir, "old", "gone.txt")
	out, err := shell.Text(shell.Local{}, gitDiffScript(filepath.Join(dir, "old"), script.Quote(gone)), 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	root, rest, _ := strings.Cut(out, "\n")
	state, diff, _ := strings.Cut(rest, "\n")
	real, _ := filepath.EvalSymlinks(dir)
	if root != real || state != "tracked" || !strings.Contains(diff, "-an old line") {
		t.Fatalf("a file git rm took reads %q", out)
	}
}
