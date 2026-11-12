package placement

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

func checkout(t *testing.T, remote string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "app with a space")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init", "-q")
	if remote != "" {
		run("remote", "add", "origin", remote)
	}
	return dir
}

func TestRemotesReadEachFoldersRepositoryOnceAndKeepIt(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	state := t.TempDir()
	ssh := checkout(t, "git@github.com:team/app.git")
	https := checkout(t, "https://github.com/team/app")
	none := checkout(t, "")
	plain := t.TempDir()
	now := time.Date(2026, 11, 12, 14, 0, 0, 0, time.UTC)
	r := OpenRemotes(state)
	r.now = func() time.Time { return now }
	r.Refresh(shell.Local{Name: "gog"}, []string{ssh, https, none, plain, "/no/such/folder"})
	if r.Of("gog", ssh) != "github.com/team/app" || r.Of("gog", https) != r.Of("gog", ssh) {
		t.Fatalf("%q %q", r.Of("gog", ssh), r.Of("gog", https))
	}
	if r.Of("gog", none) != "" || r.Of("gog", plain) != "" || r.Of("mac", ssh) != "" {
		t.Fatal("a folder with no remote, or another machine's, named a repository")
	}

	calls := 0
	counted := &shell.Fake{Name: "gog", Answer: func(string) (string, error) { calls++; return "", nil }}
	r.Refresh(counted, []string{ssh, https})
	if calls != 0 {
		t.Fatal("read again before RemotesEvery")
	}
	again := OpenRemotes(state) // a restart knows what was read
	if again.Of("gog", ssh) != "github.com/team/app" {
		t.Fatal("not kept across a restart")
	}
	again.Forget("gog")
	if again.Of("gog", ssh) != "" {
		t.Fatal("a removed machine's folders were kept")
	}
}
