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
	st := New(t.TempDir())
	k := key("local", "/x")
	st.setBadge(k, Baseline{}, Badge{Changed: 3, Fresh: 2}) // no baseline yet: kept
	if b, _ := st.Badge("local", "/x"); b.Fresh != 2 {
		t.Fatalf("got %+v", b)
	}
	st.mu.Lock()
	st.baselines[k] = Baseline{At: time.Now(), Files: map[string]string{}}
	st.badges[k] = Badge{}
	st.mu.Unlock()
	st.setBadge(k, Baseline{}, Badge{Changed: 3, Fresh: 2}) // computed before "seen": dropped
	if b, _ := st.Badge("local", "/x"); b.Fresh != 0 || b.Changed != 0 {
		t.Fatalf("a stale count came back: %+v", b)
	}
}

// a scratch checkout: git run in it with a throwaway identity, files
// written into it
func scratch(t *testing.T) (dir string, git func(args ...string) string, write func(name, body string)) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir = t.TempDir()
	git = func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write = func(name, body string) { _ = os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644) }
	git("init", "-q")
	return dir, git, write
}

// a scratch checkout with a modified file, a deleted one, a renamed one, a
// new one and a name with a space: all count, all can be marked seen
func TestSeenCoversEveryKindOfChange(t *testing.T) {
	dir, git, write := scratch(t)
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

	mc := &machines.Machine{Cfg: machines.Config{ID: "local"}, Local: true}
	st := New(t.TempDir())
	sum := st.Summary(mc, dir)
	if sum.Error != "" || len(sum.Files) != 6 || sum.Fresh != 6 {
		t.Fatalf("before: %+v", sum)
	}
	if _, err := st.MarkSeen(mc, dir, ""); err != nil {
		t.Fatal(err)
	}
	sum = st.Summary(mc, dir)
	if len(sum.Files) != 6 || sum.Fresh != 0 {
		t.Fatalf("after seen: fresh %d of %d: %+v", sum.Fresh, len(sum.Files), sum.Files)
	}
	if b, _ := st.Badge("local", dir); b.Fresh != 0 || b.Changed != 6 {
		t.Fatalf("badge %+v", b)
	}
	write("c d.txt", "c3")
	sum = st.Summary(mc, dir)
	if sum.Fresh != 1 || !sum.Files[0].Fresh || sum.Files[0].Path != "c d.txt" {
		t.Fatalf("after an edit: %+v", sum.Files)
	}
	st.Badges(mc, []string{dir})
	if b, _ := st.Badge("local", dir); b.Fresh != 1 || b.Changed != 6 {
		t.Fatalf("badge after an edit %+v", b)
	}
}

// a baseline set, a branch of a hundred files merged in, one more edit: the
// list is that edit, never what the merge brought, and the commits since
// the look follow the first parent — the merge is one line of them
func TestAMergeBringsNoFilesOfItsOwn(t *testing.T) {
	dir, git, write := scratch(t)
	write("a.txt", "a\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	mc := &machines.Machine{Cfg: machines.Config{ID: "local"}, Local: true}
	st := New(t.TempDir())
	if _, err := st.MarkSeen(mc, dir, VerdictApproved); err != nil {
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

	sum := st.Summary(mc, dir)
	if sum.Error != "" {
		t.Fatal(sum.Error)
	}
	if len(sum.Files) != 1 || sum.Files[0].Path != "own.txt" || !sum.Files[0].Fresh || sum.Fresh != 1 {
		t.Fatalf("files: %+v", sum.Files)
	}
	if len(sum.Commits) != 2 || !strings.HasPrefix(sum.Commits[0].Subject, "Merge branch 'staging'") || sum.Commits[1].Subject != "own work" || sum.Commits[0].Seen || sum.Commits[1].Seen {
		t.Fatalf("commits: %+v", sum.Commits)
	}
	if sum.Baseline == nil || sum.Baseline.Verdict != VerdictApproved {
		t.Fatalf("baseline: %+v", sum.Baseline)
	}
	d, err := st.Diff(mc, dir, "own.txt")
	if err != nil || !strings.Contains(d, "-o\n+o2\n") {
		t.Fatalf("diff: %v\n%s", err, d)
	}
	st.Badges(mc, []string{dir})
	if b, _ := st.Badge("local", dir); b.Changed != 1 || b.Fresh != 1 {
		t.Fatalf("badge %+v", b)
	}
	// reviewed, then committed: a commit of reviewed work, nothing left
	if _, err := st.MarkSeen(mc, dir, VerdictChanges); err != nil {
		t.Fatal(err)
	}
	git("commit", "-qam", "committed as reviewed")
	sum = st.Summary(mc, dir)
	if len(sum.Files) != 0 || len(sum.Commits) != 1 || !sum.Commits[0].Seen || sum.Baseline.Verdict != VerdictChanges {
		t.Fatalf("after the commit: files %+v commits %+v baseline %+v", sum.Files, sum.Commits, sum.Baseline)
	}
	if _, err := st.MarkSeen(mc, dir, "lgtm"); err == nil {
		t.Fatal("an unknown verdict was taken")
	}
}
