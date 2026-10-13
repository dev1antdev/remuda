package review

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/machines"
)

func TestStaleBadgeDoesNotOverwriteAMovedBaseline(t *testing.T) {
	store := New(t.TempDir())
	storeKey := key("local", "/x")
	store.setBadge(storeKey, Baseline{}, Badge{Changed: 3, Fresh: 2})
	if badge, _ := store.Badge("local", "/x"); badge.Fresh != 2 {
		t.Fatalf("got %+v", badge)
	}
	store.mu.Lock()
	store.baselines[storeKey] = Baseline{At: time.Now(), Blobs: map[string]string{}}
	store.badges[storeKey] = Badge{}
	store.mu.Unlock()
	store.setBadge(storeKey, Baseline{}, Badge{Changed: 3, Fresh: 2})
	if badge, _ := store.Badge("local", "/x"); badge.Fresh != 0 || badge.Changed != 0 {
		t.Fatalf("a stale count came back: %+v", badge)
	}
}

func scratchCheckout(t *testing.T) (dir string, git func(args ...string) string, write func(name, body string)) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir = t.TempDir()
	git = func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	write = func(name, body string) { _ = os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644) }
	git("init", "-q")
	return dir, git, write
}

func TestSeenCoversEveryKindOfChange(t *testing.T) {
	dir, git, write := scratchCheckout(t)
	write("a.txt", "a")
	write("b.txt", "b")
	write("c d.txt", "c")
	write("r.txt", "r")
	git("add", "-A")
	git("commit", "-qm", "init")
	write("a.txt", "a2")
	_ = os.Remove(filepath.Join(dir, "b.txt"))
	write("c d.txt", "c2")
	git("mv", "r.txt", "renamed.txt")
	write("new.txt", "n")

	machine := &machines.Machine{Cfg: machines.Config{ID: "local"}, Local: true}
	store := New(t.TempDir())
	summary := store.Summary(machine, dir)
	if summary.Error != "" || len(summary.Files) != 6 || summary.Fresh != 6 {
		t.Fatalf("before: %+v", summary)
	}
	if _, err := store.MarkSeen(machine, dir, ""); err != nil {
		t.Fatal(err)
	}
	summary = store.Summary(machine, dir)
	if len(summary.Files) != 6 || summary.Fresh != 0 {
		t.Fatalf("after seen: fresh %d of %d: %+v", summary.Fresh, len(summary.Files), summary.Files)
	}
	if badge, _ := store.Badge("local", dir); badge.Fresh != 0 || badge.Changed != 6 {
		t.Fatalf("badge %+v", badge)
	}
	write("c d.txt", "c3")
	summary = store.Summary(machine, dir)
	if summary.Fresh != 1 || !summary.Files[0].Fresh || summary.Files[0].Path != "c d.txt" {
		t.Fatalf("after an edit: %+v", summary.Files)
	}
	store.Badges(machine, []string{dir})
	if badge, _ := store.Badge("local", dir); badge.Fresh != 1 || badge.Changed != 6 {
		t.Fatalf("badge after an edit %+v", badge)
	}
}

func TestAMergeBringsNoFilesOfItsOwn(t *testing.T) {
	dir, git, write := scratchCheckout(t)
	write("a.txt", "a\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	machine := &machines.Machine{Cfg: machines.Config{ID: "local"}, Local: true}
	store := New(t.TempDir())
	if _, err := store.MarkSeen(machine, dir, VerdictApproved); err != nil {
		t.Fatal(err)
	}
	trunk := git("rev-parse", "--abbrev-ref", "HEAD")
	git("checkout", "-q", "-b", "staging")
	for i := 0; i < 100; i++ {
		write(fmt.Sprintf("s%03d.txt", i), "s\n")
	}
	git("add", "-A")
	git("commit", "-qm", "a hundred files")
	write("s000.txt", "s2\n")
	git("commit", "-qam", "one more on staging")
	git("checkout", "-q", trunk)
	write("own.txt", "o\n")
	git("add", "-A")
	git("commit", "-qm", "own work")
	git("merge", "-q", "--no-edit", "staging")
	write("own.txt", "o2\n")

	summary := store.Summary(machine, dir)
	if summary.Error != "" {
		t.Fatal(summary.Error)
	}
	if len(summary.Files) != 1 || summary.Files[0].Path != "own.txt" || !summary.Files[0].Fresh || summary.Fresh != 1 {
		t.Fatalf("files: %+v", summary.Files)
	}
	if len(summary.Commits) != 2 || !strings.HasPrefix(summary.Commits[0].Subject, "Merge branch 'staging'") || summary.Commits[1].Subject != "own work" || summary.Commits[0].Seen || summary.Commits[1].Seen {
		t.Fatalf("commits: %+v", summary.Commits)
	}
	if summary.Baseline == nil || summary.Baseline.Verdict != VerdictApproved {
		t.Fatalf("baseline: %+v", summary.Baseline)
	}
	diff, err := store.Diff(machine, dir, "own.txt")
	if err != nil || !strings.Contains(diff, "-o\n+o2\n") {
		t.Fatalf("diff: %v\n%s", err, diff)
	}
	store.Badges(machine, []string{dir})
	if badge, _ := store.Badge("local", dir); badge.Changed != 1 || badge.Fresh != 1 {
		t.Fatalf("badge %+v", badge)
	}

	if _, err := store.MarkSeen(machine, dir, VerdictChanges); err != nil {
		t.Fatal(err)
	}
	git("commit", "-qam", "committed as reviewed")
	summary = store.Summary(machine, dir)
	if len(summary.Files) != 0 || len(summary.Commits) != 1 || !summary.Commits[0].Seen || summary.Baseline.Verdict != VerdictChanges {
		t.Fatalf("after the commit: files %+v commits %+v baseline %+v", summary.Files, summary.Commits, summary.Baseline)
	}
	if _, err := store.MarkSeen(machine, dir, "lgtm"); err == nil {
		t.Fatal("an unknown verdict was taken")
	}
}
