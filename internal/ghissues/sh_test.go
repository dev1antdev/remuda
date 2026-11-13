package ghissues

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

// The scripts run in a real sh, a stand-in gh first on the PATH they set up (~/.local/bin in a temp HOME), and git on
// repositories made here: a 304 ends a script before the list is asked for, a comment's file goes whatever happens.

// standInGh answers as gh would, every call noted in ~/gh.log.
const standInGh = `#!/bin/sh
printf '%s\n' "$*" >> "$HOME/gh.log"
case "$*" in
  "api user --jq .login") echo alice ;;
  *If-None-Match*) printf 'HTTP/2.0 304 Not Modified\r\nEtag: W/"aaa111"\r\n\r\n'; exit 1 ;;
  "api -i repos/acme/widgets/issues?state=all"*) printf 'HTTP/2.0 200 OK\r\nEtag: W/"aaa111"\r\n\r\n[{"number": 41}]' ;;
  "api -i repos/acme/widgets/issues/41") printf 'HTTP/2.0 200 OK\r\nEtag: W/"bbb"\r\n\r\n'; cat "$HOME/issue.json" ;;
  "api --paginate repos/acme/widgets/issues/41/timeline?per_page=100") cat "$HOME/timeline.json" ;;
  "api graphql"*) cat "$HOME/closedby.json" ;;
  "issue list"*) cat "$HOME/list.json" ;;
  "issue comment 41 -R acme/widgets --body-file "*) cp "$7" "$HOME/posted.txt" ;;
  "issue close 41 -R acme/widgets --reason not planned") echo closed ;;
  "issue reopen 41"*) echo 'GraphQL: Could not reopen (reopenIssue)' >&2; exit 1 ;;
  *) echo "unexpected: $*" >&2; exit 2 ;;
esac
`

// localHost is this machine with files put here.
type localHost struct{ here shell.Local }

func (h localHost) ID() string    { return h.here.ID() }
func (h localHost) Label() string { return h.here.Label() }
func (h localHost) Home() string  { return h.here.Home() }
func (h localHost) Local() bool   { return true }
func (h localHost) Run(script string, timeout time.Duration) ([]byte, error) {
	return h.here.Run(script, timeout)
}
func (h localHost) Stream(script string, timeout time.Duration, sink func([]byte)) error {
	return h.here.Stream(script, timeout, sink)
}
func (localHost) PutFile(path string, data []byte, timeout time.Duration) error {
	return os.WriteFile(path, data, 0o600)
}

func standIn(t *testing.T) (home string, s *Service) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TMPDIR", home)
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"list.json": listJSON, "issue.json": issueJSON, "timeline.json": timelineJSON, "closedby.json": closedByJSON}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(home, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(standInGh), 0o755); err != nil {
		t.Fatal(err)
	}
	if found, err := exec.Command("sh", "-c", `PATH="$HOME/.local/bin:$PATH"; command -v gh`).Output(); err != nil || strings.TrimSpace(string(found)) != filepath.Join(bin, "gh") {
		t.Fatalf("the stand-in is not the gh the scripts find: %q %v", found, err)
	}
	return home, New(func(string) (Host, error) { return localHost{here: shell.Local{Name: "here"}}, nil })
}

func ghLog(t *testing.T, home string) []string {
	t.Helper()
	data, _ := os.ReadFile(filepath.Join(home, "gh.log"))
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func TestTheScriptsRunInShAndA304AsksNothingMore(t *testing.T) {
	home, s := standIn(t)
	clock := time.Now()
	s.now = func() time.Time { return clock }
	list, err := s.List("here", "acme/widgets", DefaultList, false)
	if err != nil || len(list.Items) != 3 || list.Me != "alice" {
		t.Fatalf("%+v %v", list, err)
	}
	clock = clock.Add(2 * time.Minute)
	if probed, err := s.List("here", "acme/widgets", DefaultList, false); err != nil || probed.Source != FromProbe {
		t.Fatalf("%+v %v", probed.Source, err)
	}
	calls := ghLog(t, home)
	if last := calls[len(calls)-1]; !strings.Contains(last, "If-None-Match") {
		t.Fatalf("the list was asked for after a 304: %v", calls)
	}
	d, err := s.Issue("here", "acme/widgets", 41, false)
	if err != nil || d.Title != "Crash on start" || len(d.Timeline) == 0 || len(d.Pulls) != 1 {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestAWriteInShPostsTheFileAndRemovesIt(t *testing.T) {
	home, s := standIn(t)
	if err := s.Close("here", "acme/widgets", 41, "not planned", "Won't fix: it's by design."); err != nil {
		t.Fatal(err)
	}
	posted, _ := os.ReadFile(filepath.Join(home, "posted.txt"))
	if string(posted) != "Won't fix: it's by design." {
		t.Fatalf("posted %q", posted)
	}
	if left, _ := filepath.Glob(filepath.Join(home, "remuda-issue.*")); len(left) != 0 {
		t.Fatalf("the comment's file stayed: %v", left)
	}
	err := s.Reopen("here", "acme/widgets", 41, "Back again.")
	if err == nil || !strings.Contains(err.Error(), "Could not reopen") {
		t.Fatalf("gh's refusal: %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(home, "remuda-issue.*")); len(left) != 0 {
		t.Fatalf("a refused write left its file: %v", left)
	}
}

func TestOriginsAreReadOffRealCheckouts(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	home, s := standIn(t)
	repo := filepath.Join(home, "widgets")
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@example.invalid", "-c", "user.name=t"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	run(repo, "init", "-q", "-b", "main")
	run(repo, "commit", "-q", "--allow-empty", "-m", "init")
	run(repo, "remote", "add", "origin", "git@github.com:acme/widgets.git")
	tree := filepath.Join(home, "trees", "dev-feat-41")
	run(repo, "worktree", "add", "-q", "-b", "dev/feat/41", tree)
	run(repo, "config", "branch.dev/feat/41.remuda-issue", "acme/widgets#41")
	plain := filepath.Join(home, "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	answer, err := s.Repos("here", []string{repo, tree, plain}, tree)
	if err != nil || len(answer.Repos) != 1 {
		t.Fatalf("%+v %v", answer, err)
	}
	r := answer.Repos[0]
	real, _ := filepath.EvalSymlinks(repo)
	if r.Repo != "acme/widgets" || r.Folder != real || len(r.Dirs) != 2 || len(r.Links) != 1 || r.Links[0] != (Link{"dev/feat/41", 41}) {
		t.Fatalf("%+v (want main folder %s)", r, real)
	}
	if answer.Here.Repo != "acme/widgets" {
		t.Fatalf("the worktree asked about: %+v", answer.Here)
	}
	if again, _ := s.Repos("here", []string{repo, tree, plain}, plain); again.Here.Why != "no-git" {
		t.Fatalf("a plain folder: %+v", again.Here)
	}
}
