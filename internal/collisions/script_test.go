package collisions

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
)

// repository is a clone of a bare origin with a main checkout, the way a person's repository and its worktrees stand:
// origin/HEAD names main, and the worktrees branch off it.
type repository struct {
	t      *testing.T
	origin string
	main   string
}

func newRepository(t *testing.T) *repository {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root := t.TempDir()
	r := &repository{t: t, origin: filepath.Join(root, "origin.git"), main: filepath.Join(root, "app")}
	r.git(root, "init", "-q", "--bare", "-b", "main", r.origin)
	seed := filepath.Join(root, "seed")
	r.git(root, "clone", "-q", r.origin, seed)
	r.write(seed, "src/auth.ts", numbered(60))
	r.write(seed, "src/other.ts", numbered(10))
	r.write(seed, "logo.png", "\x89PNG\x00one")
	r.git(seed, "add", "-A")
	r.git(seed, "commit", "-qm", "init")
	r.git(seed, "push", "-q", "origin", "HEAD:main")
	r.git(root, "clone", "-q", r.origin, r.main)
	return r
}

func numbered(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString("line " + strings.Repeat("x", i%7) + "\n")
	}
	return b.String()
}

func (r *repository) git(dir string, args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t", "-c", "init.defaultBranch=main"}, args...)...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func (r *repository) write(dir, name, body string) {
	r.t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

// edit replaces the lines from..to (1-based, inclusive) of a file with text of its own.
func (r *repository) edit(dir, name string, from, to int, with string) {
	r.t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		r.t.Fatal(err)
	}
	lines := strings.SplitAfter(string(data), "\n")
	var out []string
	out = append(out, lines[:from-1]...)
	out = append(out, with)
	out = append(out, lines[to:]...)
	r.write(dir, name, strings.Join(out, ""))
}

func (r *repository) worktree(name string) string {
	dir := filepath.Join(filepath.Dir(r.main), name)
	r.git(r.main, "worktree", "add", "-q", "-b", name, dir)
	return dir
}

func TestTheScriptReadsEachWorkAgainstTheMainBranch(t *testing.T) {
	r := newRepository(t)
	web, api := r.worktree("web"), r.worktree("api")
	r.edit(web, "src/auth.ts", 40, 44, "web's own\n")
	r.git(web, "commit", "-qam", "web's change") // a commit of the branch counts as much as what is uncommitted
	r.edit(web, "src/other.ts", 2, 2, "web again\n")
	r.write(web, "src/new.ts", "fresh\n")
	r.write(web, "logo.png", "\x89PNG\x00two")
	r.edit(api, "src/auth.ts", 42, 43, "api's own\n")
	r.edit(api, "src/other.ts", 9, 9, "api too\n")
	r.write(api, "src/new.ts", "another\n")
	if err := os.Remove(filepath.Join(api, "logo.png")); err != nil {
		t.Fatal(err)
	}

	sub := filepath.Join(web, "src")
	output, err := shell.Text(shell.Local{Name: "local"}, Script([]string{web, api, sub, t.TempDir()}), scriptTimeout)
	if err != nil {
		t.Fatal(err)
	}
	works := Parse("local", output)
	if len(works) != 3 {
		t.Fatalf("works: %+v", works)
	}
	byDir := map[string]Work{}
	for _, work := range works {
		byDir[work.Dir] = work
	}
	webWork, apiWork, subWork := byDir[web], byDir[api], byDir[sub]
	if webWork.Main != "main" || webWork.Base == "" || webWork.Base != apiWork.Base {
		t.Fatalf("base: %q %q / %q", webWork.Main, webWork.Base, apiWork.Base)
	}
	if subWork.Top != webWork.Top || len(subWork.Files) != len(webWork.Files) {
		t.Fatalf("a subfolder is its checkout's work: %+v", subWork)
	}
	auth := webWork.Files["src/auth.ts"]
	if len(auth.Hunks) != 1 || auth.Hunks[0].Start != 40 || auth.Hunks[0].Count != 5 || auth.Blob == "" {
		t.Fatalf("web's auth.ts: %+v", auth)
	}
	if !webWork.Files["src/new.ts"].Whole || !webWork.Files["logo.png"].Whole || apiWork.Files["logo.png"].Blob != Gone {
		t.Fatalf("new, binary and deleted files: %+v / %+v", webWork.Files, apiWork.Files)
	}
	overlaps := Compare(webWork, apiWork)
	got := map[string]Overlap{}
	for _, o := range overlaps {
		got[o.Path] = o
	}
	if o := got["src/auth.ts"]; !o.Lines || o.From != 40 || o.To != 44 {
		t.Fatalf("auth.ts: %+v", o)
	}
	if o := got["src/other.ts"]; o.Lines {
		t.Fatalf("other.ts is touched far apart: %+v", o)
	}
	if !got["src/new.ts"].Lines || !got["logo.png"].Lines || len(overlaps) != 4 {
		t.Fatalf("overlaps: %+v", overlaps)
	}
}

func TestAMainAheadOfOriginIsTheBase(t *testing.T) {
	r := newRepository(t)
	r.edit(r.main, "src/auth.ts", 1, 1, "not pushed yet\n")
	r.git(r.main, "commit", "-qam", "a commit on main not pushed")
	web := r.worktree("web")
	r.edit(web, "src/other.ts", 5, 5, "web\n")
	output, err := shell.Text(shell.Local{Name: "local"}, Script([]string{web, r.main}), scriptTimeout)
	if err != nil {
		t.Fatal(err)
	}
	works := Parse("local", output)
	if len(works) != 2 {
		t.Fatalf("works: %+v", works)
	}
	for _, work := range works {
		if work.Main != "main" {
			t.Fatalf("%s read against %q, want the local main that holds origin's", work.Dir, work.Main)
		}
		if _, ok := work.Files["src/auth.ts"]; ok {
			t.Fatalf("%s counts main's own commit as its work: %+v", work.Dir, work.Files)
		}
	}
}

func TestAMainBehindOriginGivesWayToOrigin(t *testing.T) {
	r := newRepository(t)
	other := filepath.Join(t.TempDir(), "other")
	r.git(filepath.Dir(other), "clone", "-q", r.origin, other)
	r.edit(other, "src/other.ts", 1, 1, "pushed by someone else\n")
	r.git(other, "commit", "-qam", "upstream moves on")
	r.git(other, "push", "-q", "origin", "HEAD:main")
	r.git(r.main, "fetch", "-q")
	web := r.worktree("web")
	r.edit(web, "src/auth.ts", 3, 3, "web\n")
	output, err := shell.Text(shell.Local{Name: "local"}, Script([]string{web}), scriptTimeout)
	if err != nil {
		t.Fatal(err)
	}
	works := Parse("local", output)
	if len(works) != 1 || works[0].Main != "origin/main" || len(works[0].Files) != 1 {
		t.Fatalf("works: %+v", works)
	}
}
