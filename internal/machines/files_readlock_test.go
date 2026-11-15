package machines

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// A file's changes in the file viewer are a read: a file only touched since the index last saw it — a formatter that
// rewrote it as it was, a checkout — must not have git's diff refresh its stat and write the index back, under the
// index.lock an agent's own git add or commit may want then.
func TestTheFileViewersDiffLeavesTheIndexAlone(t *testing.T) {
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
	touched, edited := filepath.Join(dir, "touched.txt"), filepath.Join(dir, "edited.txt")
	for path, body := range map[string]string{touched: "the same\n", edited: "one\n"} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	git("add", "-A")
	git("commit", "-q", "-m", "init")
	if err := os.WriteFile(edited, []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ago := time.Now().Add(-time.Hour)
	if err := os.Chtimes(touched, ago, ago); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(dir, ".git", "index")
	before, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	out, err := shell.Text(shell.Local{}, gitDiffScript(dir, script.Quote(edited)), 20*time.Second)
	if err != nil || !strings.Contains(out, "+two") {
		t.Fatalf("the diff: %v %q", err, out)
	}
	out, err = shell.Text(shell.Local{}, gitDiffScript(dir, script.Quote(touched)), 20*time.Second)
	if _, rest, _ := strings.Cut(out, "\n"); err != nil || strings.TrimSpace(rest) != "tracked" {
		t.Fatalf("the diff of a file only touched: %v %q", err, out)
	}
	if after, _ := os.ReadFile(index); !bytes.Equal(before, after) {
		t.Fatal("the file viewer's diff wrote the index")
	}
}
