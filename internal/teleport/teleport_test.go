package teleport

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

// machine is this computer standing in for a machine of its own: its own home, a file put on it written here.
type machine struct {
	sh          shell.Local
	label, home string
}

func (m machine) Run(script string, timeout time.Duration) ([]byte, error) {
	return m.sh.Run(script, timeout)
}
func (m machine) Stream(script string, timeout time.Duration, sink func([]byte)) error {
	return m.sh.Stream(script, timeout, sink)
}
func (m machine) ID() string    { return m.sh.Name }
func (m machine) Label() string { return m.label }
func (m machine) Home() string  { return m.home }
func (m machine) Local() bool   { return true }
func (m machine) PutFile(path string, data []byte, _ time.Duration) error {
	return os.WriteFile(path, data, 0o600)
}

func newMachine(t *testing.T, id, label string) machine {
	return machine{sh: shell.Local{Name: id}, label: label, home: t.TempDir()}
}

func needGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@t")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@t")
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// clones makes a bare origin with one commit and two clones of it, the source and the target.
func clones(t *testing.T) (origin, source, target string) {
	base := t.TempDir()
	seed := filepath.Join(base, "seed")
	os.Mkdir(seed, 0o755)
	git(t, seed, "init", "-q")
	write(t, seed, "app.txt", "one\n")
	write(t, seed, ".gitignore", ".env\n")
	git(t, seed, "add", "-A")
	git(t, seed, "commit", "-qm", "seed")
	origin = filepath.Join(base, "origin.git")
	git(t, base, "clone", "-q", "--bare", seed, origin)
	source = filepath.Join(base, "source", "app")
	target = filepath.Join(base, "target", "app")
	git(t, base, "clone", "-q", origin, source)
	git(t, base, "clone", "-q", origin, target)
	return origin, source, target
}

// working leaves the source on feature/x with a commit on no remote, a staged change, an unstaged one, a new file
// and an ignored one.
func working(t *testing.T, source string) {
	git(t, source, "checkout", "-qb", "feature/x")
	write(t, source, "app.txt", "one\ntwo\n")
	git(t, source, "commit", "-qam", "two")
	write(t, source, "staged.txt", "staged\n")
	git(t, source, "add", "staged.txt")
	write(t, source, "app.txt", "one\ntwo\nthree\n")
	write(t, source, "notes/new.md", "new\n")
	write(t, source, ".env", "SECRET=1\n")
}

func TestMoveCarriesTheWorkWhole(t *testing.T) {
	needGit(t)
	_, source, target := clones(t)
	working(t, source)
	before := git(t, source, "status", "--porcelain")
	head := git(t, source, "rev-parse", "HEAD")

	from, to := newMachine(t, "mac", "This Mac"), newMachine(t, "gog", "GOG")
	var started Landed
	var lines []string
	landed, err := Move{Source: from, SourceDir: source, Target: to, TargetDir: target, Start: func(l Landed, src Source) error {
		started = l
		return nil
	}}.Run(func(line string) { lines = append(lines, line) })
	if err != nil {
		t.Fatalf("move: %v\n%s", err, strings.Join(lines, "\n"))
	}
	if started != landed || landed.Branch != "feature/x" {
		t.Fatalf("landed %+v, started %+v", landed, started)
	}
	if want := filepath.Join(to.home, ".herdr", "worktrees", "app", "feature-x"); landed.Dir != want {
		t.Fatalf("worktree at %s, want %s", landed.Dir, want)
	}
	if got := git(t, landed.Dir, "rev-parse", "HEAD"); got != head {
		t.Fatalf("worktree HEAD %s, want the source's %s", got, head)
	}
	if got := git(t, landed.Dir, "diff", "--cached", "--name-only"); got != "staged.txt" {
		t.Fatalf("staged in the worktree: %q", got)
	}
	if got := git(t, landed.Dir, "diff", "--name-only"); got != "app.txt" {
		t.Fatalf("unstaged in the worktree: %q", got)
	}
	if read(t, filepath.Join(landed.Dir, "app.txt")) != "one\ntwo\nthree\n" || read(t, filepath.Join(landed.Dir, "notes/new.md")) != "new\n" {
		t.Fatal("the files did not arrive as they were")
	}
	if _, err := os.Stat(filepath.Join(landed.Dir, ".env")); !os.IsNotExist(err) {
		t.Fatal("an ignored file travelled")
	}
	if git(t, source, "status", "--porcelain") != before || git(t, source, "rev-parse", "HEAD") != head {
		t.Fatal("the source changed")
	}
	if git(t, target, "status", "--porcelain") != "" {
		t.Fatal("the target's own checkout changed")
	}
	if matches, _ := filepath.Glob(filepath.Join(os.TempDir(), "remuda-teleport.*")); len(matches) > 0 {
		t.Logf("left in the temp dir: %v", matches)
	}
}

func TestMoveRenamesABranchTheTargetHas(t *testing.T) {
	needGit(t)
	_, source, target := clones(t)
	working(t, source)
	git(t, target, "branch", "feature/x")
	landed, err := Move{Source: newMachine(t, "mac", "This Mac · MacBook"), SourceDir: source, Target: newMachine(t, "gog", "GOG"), TargetDir: target,
		Start: func(Landed, Source) error { return nil }}.Run(func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if landed.Branch != "feature/x-from-this-mac-macbook" {
		t.Fatalf("branch %q", landed.Branch)
	}
	if git(t, target, "rev-parse", "feature/x") == git(t, target, "rev-parse", landed.Branch) {
		t.Fatal("the target's own branch moved")
	}
}

func TestMoveSendsTheWholeBranchWhenTheTargetLacksItsBase(t *testing.T) {
	needGit(t)
	_, source, target := clones(t)
	mirror := filepath.Join(t.TempDir(), "mirror.git")
	git(t, filepath.Dir(mirror), "init", "-q", "--bare", mirror)
	git(t, source, "checkout", "-qb", "feature/y")
	write(t, source, "a.txt", "a\n")
	git(t, source, "add", "a.txt")
	git(t, source, "commit", "-qm", "a")
	git(t, source, "remote", "add", "mirror", mirror)
	git(t, source, "push", "-q", "mirror", "feature/y")
	write(t, source, "b.txt", "b\n")
	git(t, source, "add", "b.txt")
	git(t, source, "commit", "-qm", "b")

	var lines []string
	landed, err := Move{Source: newMachine(t, "mac", "Mac"), SourceDir: source, Target: newMachine(t, "gog", "GOG"), TargetDir: target,
		Start: func(Landed, Source) error { return nil }}.Run(func(line string) { lines = append(lines, line) })
	if err != nil {
		t.Fatalf("%v\n%s", err, strings.Join(lines, "\n"))
	}
	if !strings.Contains(strings.Join(lines, "\n"), "sending the whole branch") {
		t.Fatalf("no second, whole send:\n%s", strings.Join(lines, "\n"))
	}
	if read(t, filepath.Join(landed.Dir, "a.txt")) != "a\n" || read(t, filepath.Join(landed.Dir, "b.txt")) != "b\n" {
		t.Fatal("the commits did not all arrive")
	}
}

func TestMoveWithoutARemoteNeedsTheSharedHistory(t *testing.T) {
	needGit(t)
	base := t.TempDir()
	source := filepath.Join(base, "source")
	os.Mkdir(source, 0o755)
	git(t, source, "init", "-q")
	write(t, source, "x.txt", "x\n")
	git(t, source, "add", "x.txt")
	git(t, source, "commit", "-qm", "x")
	target := filepath.Join(base, "target")
	git(t, base, "clone", "-q", source, target)
	git(t, source, "checkout", "-qb", "work")
	write(t, source, "y.txt", "y\n")
	git(t, source, "add", "y.txt")
	git(t, source, "commit", "-qm", "y")
	landed, err := Move{Source: newMachine(t, "mac", "Mac"), SourceDir: source, Target: newMachine(t, "gog", "GOG"), TargetDir: target,
		Start: func(Landed, Source) error { return nil }}.Run(func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(landed.Dir, "y.txt")) != "y\n" {
		t.Fatal("the commit did not arrive")
	}

	stranger := filepath.Join(base, "stranger")
	os.Mkdir(stranger, 0o755)
	git(t, stranger, "init", "-q")
	write(t, stranger, "z.txt", "z\n")
	git(t, stranger, "add", "z.txt")
	git(t, stranger, "commit", "-qm", "z")
	if _, err := (Move{Source: newMachine(t, "mac", "Mac"), SourceDir: source, Target: newMachine(t, "gog", "GOG"), TargetDir: stranger,
		Start: func(Landed, Source) error { return nil }}).Run(func(string) {}); err == nil || !strings.Contains(err.Error(), "does not share") {
		t.Fatalf("an unrelated repository was taken: %v", err)
	}
}

func TestMoveRefusesWhatCannotTravel(t *testing.T) {
	needGit(t)
	origin, source, target := clones(t)
	other := filepath.Join(t.TempDir(), "other")
	git(t, filepath.Dir(other), "clone", "-q", origin, other)
	git(t, other, "remote", "set-url", "origin", "git@github.com:someone/else.git")
	mac := newMachine(t, "mac", "Mac")
	run := func(dir, to string) error {
		_, err := Move{Source: mac, SourceDir: dir, Target: newMachine(t, "gog", "GOG"), TargetDir: to, Start: func(Landed, Source) error { return nil }}.Run(func(string) {})
		return err
	}
	if err := run(source, other); err == nil || !strings.Contains(err.Error(), "another repository") {
		t.Fatalf("another repository: %v", err)
	}
	if err := run(t.TempDir(), target); err == nil || !strings.Contains(err.Error(), "not a git checkout") {
		t.Fatalf("outside git: %v", err)
	}
	git(t, source, "checkout", "-q", "--detach")
	if err := run(source, target); err == nil || !strings.Contains(err.Error(), "no branch") {
		t.Fatalf("detached: %v", err)
	}
	git(t, source, "checkout", "-q", "main")
	if _, err := (Move{Source: mac, SourceDir: source, Target: mac, TargetDir: source, Start: func(Landed, Source) error { return nil }}).Run(func(string) {}); err == nil || !strings.Contains(err.Error(), "very checkout") {
		t.Fatalf("onto itself: %v", err)
	}
}

func TestCheckoutsPutTheSameRepositoryFirst(t *testing.T) {
	needGit(t)
	origin, source, target := clones(t)
	other := filepath.Join(t.TempDir(), "other")
	git(t, filepath.Dir(other), "clone", "-q", origin, other)
	git(t, other, "remote", "set-url", "origin", "https://example.com/else.git")
	git(t, target, "remote", "set-url", "origin", "git@example.com:team/app.git")
	git(t, source, "remote", "set-url", "origin", "https://example.com/team/app")
	src, err := Inspect(shell.Local{Name: "mac"}, source)
	if err != nil {
		t.Fatal(err)
	}
	list, err := Checkouts(shell.Local{Name: "gog"}, []string{other, filepath.Join(target, "."), target, t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || !list[0].Same || list[0].Dir == other || list[1].Same {
		t.Fatalf("checkouts: %+v", list)
	}
}

func TestRemoteKey(t *testing.T) {
	for _, tc := range []struct{ a, b string }{
		{"git@gitlab.gog.com:backend/games-db.git", "https://gitlab.gog.com/backend/games-db"},
		{"ssh://git@github.com/dev1antdev/remuda.git", "https://github.com/dev1antdev/remuda/"},
		{"https://user:token@GitHub.com/a/b.git", "git@github.com:a/b"},
	} {
		if !SameRepository(tc.a, tc.b) {
			t.Errorf("%s and %s should be one repository (%q, %q)", tc.a, tc.b, RemoteKey(tc.a), RemoteKey(tc.b))
		}
	}
	if SameRepository("", "") || SameRepository("git@github.com:a/b", "git@github.com:a/c") {
		t.Error("different or empty remotes matched")
	}
}

func TestWords(t *testing.T) {
	if got := Slug("This Mac · Sergiis-MacBook-Pro"); got != "this-mac-sergiis-macbook" {
		t.Errorf("slug %q", got)
	}
	if got := Slug("··"); got != "elsewhere" {
		t.Errorf("empty slug %q", got)
	}
	if got := (Source{Branch: "feature/x", Ahead: 1, Staged: 2, Untracked: 1}).Words(); got != "feature/x · 1 commit on no remote · 2 staged files · 1 new file" {
		t.Errorf("words %q", got)
	}
	if Size(800) != "800 B" || Size(14*1024) != "14 KB" || Size(3<<20) != "3.0 MB" {
		t.Error("sizes")
	}
	p := Preamble("GOG", "/w/app", Source{Branch: "x", Unstaged: 1}, Landed{Branch: "x-from-gog", Dir: "/h/w"}, "This Mac")
	if !strings.Contains(p, "moved here from GOG") || !strings.Contains(p, "branch x-from-gog") || !strings.Contains(p, "the unstaged changes") || strings.Contains(p, "staged changes (still") {
		t.Errorf("preamble:\n%s", p)
	}
}

func TestLandReadsTheRemoteScriptsLines(t *testing.T) {
	answer := ""
	var fail error
	host := &shell.Fake{Name: "gog", Remote: true, Answer: func(string) (string, error) { return answer, fail }}
	var said []string
	say := func(line string) { said = append(said, line) }

	answer = "fetching what this checkout lacks from its remotes\n" + needFull + "\n"
	fail = &shell.ExitError{Code: 3}
	if _, err := Land(host, "/w/app", "/tmp/p", "/h/.herdr/worktrees/app", "from-mac", say); err != errNeedFull {
		t.Fatalf("need full: %v", err)
	}

	answer = "branch x\nworktree /h/.herdr/worktrees/app/x\nerror: the staged changes did not apply\n"
	fail = &shell.ExitError{Code: 1}
	landed, err := Land(host, "/w/app", "/tmp/p", "/h/.herdr/worktrees/app", "from-mac", say)
	if err == nil || !strings.Contains(err.Error(), "stays at /h/.herdr/worktrees/app/x") || landed.Dir == "" {
		t.Fatalf("a partial landing must name its worktree: %+v %v", landed, err)
	}

	answer, fail = "branch x\nworktree /h/w/x\nthe unstaged changes applied", nil
	landed, err = Land(host, "/w/app", "/tmp/p", "/h/w", "from-mac", say)
	if err != nil || landed != (Landed{Dir: "/h/w/x", Branch: "x"}) {
		t.Fatalf("landed %+v %v", landed, err)
	}
	scripts := host.Scripts()
	last := scripts[len(scripts)-1]
	for _, want := range []string{"cd '/w/app'", "pk='/tmp/p'; trap 'rm -f \"$pk\"' EXIT", "'/h/w'/", `"$branch-"'from-mac'`, "git apply --index --binary"} {
		if !strings.Contains(last, want) {
			t.Errorf("the landing script lacks %q", want)
		}
	}
	if !strings.Contains(strings.Join(said, "|"), "the unstaged changes applied") {
		t.Errorf("said: %v", said)
	}
}
