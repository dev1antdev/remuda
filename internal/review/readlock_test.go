package review

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// staleStat moves a file's modification time and leaves its content: git's diff then finds the index's stat of it out
// of date and, unless told not to, refreshes it and writes the index back — under .git/index.lock, which an agent's own
// git add or commit at that moment stops on.
func staleStat(t *testing.T, path string, ago time.Duration) {
	t.Helper()
	at := time.Now().Add(-ago)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

func TestTheLensReadsLeaveTheIndexAlone(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir, git, write := scratchCheckout(t)
	write("touched.txt", "the same\n")
	write("edited.txt", "one\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	head := git("rev-parse", "HEAD")
	write("edited.txt", "one\ntwo\n")
	touched, index := filepath.Join(dir, "touched.txt"), filepath.Join(dir, ".git", "index")
	read := func() []byte {
		b, err := os.ReadFile(index)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}

	staleStat(t, touched, time.Hour)
	before := read()
	plain := exec.Command("git", "--no-optional-locks", "diff", "--numstat")
	plain.Dir = dir
	if out, err := plain.CombinedOutput(); err != nil {
		t.Fatalf("git diff: %v %s", err, out)
	}
	if bytes.Equal(before, read()) {
		t.Fatal("the test proves nothing: a plain diff of a touched file left the index as it was")
	}

	staleStat(t, touched, 2*time.Hour)
	before = read()
	local := shell.Local{Name: "local"}
	store := New(t.TempDir())
	summary := store.Summary(local, dir)
	if len(summary.Files) != 1 || summary.Files[0].Path != "edited.txt" {
		t.Fatalf("the summary lists what changed and not what was only touched: %+v", summary.Files)
	}
	if _, blobs, err := Snapshot(local, dir); err != nil || len(blobs) != 1 || blobs["edited.txt"] == "" {
		t.Fatalf("the uncommitted blobs: %v %v", err, blobs)
	}
	store.Badges(local, []string{dir})
	if _, err := shell.Text(local, script.InDir(dir)+statusScript, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	for _, side := range []string{SideUnstaged, SideStaged} {
		if _, err := store.DiffSide(local, dir, "edited.txt", side); err != nil {
			t.Fatal(err)
		}
	}
	if diff, err := BaseDiff(local, dir, head, "edited.txt"); err != nil || !bytes.Contains([]byte(diff), []byte("+two")) {
		t.Fatalf("the branch's diff: %v %q", err, diff)
	}
	if !bytes.Equal(before, read()) {
		t.Fatal("a read of the Changes lens wrote the index")
	}
}
