package review

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/dev1antdev/herdr-ui/internal/machines"
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

// a scratch checkout with a modified file, a deleted one, a renamed one, a
// new one and a name with a space: all count, all can be marked seen
func TestSeenCoversEveryKindOfChange(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, body string) { _ = os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644) }
	git("init", "-q")
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
	if _, err := st.MarkSeen(mc, dir); err != nil {
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
