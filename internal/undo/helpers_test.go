package undo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

// local is this machine as an undo sees it: its shell, and a file written here directly.
type local struct{ shell.Host }

func (local) PutFile(path string, data []byte, _ time.Duration) error {
	return os.WriteFile(path, data, 0o600)
}

func needGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	// the person's own git settings (a signing key, hooks, a template) never reach the tests' repositories
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
}

// git runs git in dir and answers its output, failing the test on an error.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// repo is a fresh repository with a first commit of the files given.
func repo(t *testing.T, files map[string]string) string {
	t.Helper()
	needGit(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-q", "-b", "main")
	for path, text := range files {
		write(t, dir, path, text)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "first")
	return dir
}

func write(t *testing.T, dir, path, text string) {
	t.Helper()
	full := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, dir, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil {
		return "<gone>"
	}
	return string(data)
}

// fixture is an undo service over a store in a temp state dir and this machine's shell.
type fixture struct {
	store   *Store
	service *Service
	working string
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{store: OpenStore(t.TempDir())}
	f.service = &Service{
		Store:   f.store,
		Host:    func(string) (Host, error) { return local{shell.Local{Name: "local"}}, nil },
		Working: func(string, string) string { return f.working },
	}
	return f
}

// turn takes the two checkpoints around work, as the tracker does, and keeps the turn.
func (f *fixture) turn(t *testing.T, dir string, work func()) Turn {
	t.Helper()
	host := local{shell.Local{Name: "local"}}
	start, err := read(host, dir, readOptions{patch: forwardPatch})
	if err != nil {
		t.Fatal(err)
	}
	work()
	end, err := read(host, dir, readOptions{base: start.Head, patch: reversePatch})
	if err != nil {
		t.Fatal(err)
	}
	turn, startPatch, endPatch := Delta(start, end)
	turn.ID, turn.Machine, turn.Dir, turn.Pane, turn.Agent = NewID(time.Now()), "local", dir, "w1:p1", "Claude Code"
	turn.Started, turn.Ended = time.Now().Add(-time.Minute), time.Now()
	if err := f.store.Add(turn, startPatch, endPatch); err != nil {
		t.Fatal(err)
	}
	return turn
}

// status is git's short status, the porcelain v1 lines sorted as git prints them.
func status(t *testing.T, dir string) string {
	return git(t, dir, "status", "--porcelain=v1", "--untracked-files=all")
}
