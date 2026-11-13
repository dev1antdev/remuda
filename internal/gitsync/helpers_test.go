package gitsync

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/shell"
)

// sandbox makes git in the test see nobody's config: a HOME of its own, no system file, a fixed author.
func sandbox(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	for _, name := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(name, "test")
	}
	for _, name := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(name, "test@remuda.invalid")
	}
	return home
}

// gitIn runs git in a folder, failing the test when it fails.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "init.defaultBranch=main", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// commit writes a file and commits it.
func commit(t *testing.T, dir, file, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, file)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "--", file)
	gitIn(t, dir, "commit", "-q", "-m", "change "+file)
}

// remoteAndClones is a bare remote with a first commit and n clones of it, each tracking main.
func remoteAndClones(t *testing.T, home string, n int) (bare string, clones []string) {
	t.Helper()
	bare = filepath.Join(home, "remote.git")
	gitIn(t, home, "init", "-q", "--bare", bare)
	seed := filepath.Join(home, "seed")
	gitIn(t, home, "init", "-q", seed)
	commit(t, seed, "README.md", "# hello\n")
	gitIn(t, seed, "remote", "add", "origin", bare)
	gitIn(t, seed, "push", "-q", "-u", "origin", "main")
	for i := range n {
		clone := filepath.Join(home, "code", "clone"+string(rune('a'+i)))
		gitIn(t, home, "clone", "-q", bare, clone)
		clones = append(clones, clone)
	}
	return bare, clones
}

// service is the place over this machine alone, with the busy answer and the secret check given.
func service(t *testing.T, busy func(string, string) (string, string), check func(shell.Host, string) (review.SecretScan, error)) *Service {
	t.Helper()
	host := shell.Local{Name: "local"}
	return &Service{
		Fleet: func() []Machine { return []Machine{{Host: host}} },
		Host:  func(string) (shell.Host, error) { return host, nil },
		Busy:  busy,
		Check: check,
		Scans: OpenScans(t.TempDir()),
	}
}
