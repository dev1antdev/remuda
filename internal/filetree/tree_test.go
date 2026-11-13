package filetree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

// repo makes a checkout in a temp folder with git's own config and the person's kept out of it.
func repo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := filepath.Join(t.TempDir(), "app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "init", "-q", "-b", "main")
	return dir
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@remuda.invalid", "-c", "user.name=t"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func byName(entries []Entry) map[string]Entry {
	out := map[string]Entry{}
	for _, e := range entries {
		out[e.Name] = e
	}
	return out
}

func names(entries []Entry) string {
	var out []string
	for _, e := range entries {
		out = append(out, e.Name)
	}
	return strings.Join(out, " ")
}

func TestAFolderListsFoldersFirstWithGitsWordOnEachEntry(t *testing.T) {
	dir := repo(t)
	write(t, dir+"/README.md", "# app\n")
	write(t, dir+"/src/app.go", "package main\n")
	write(t, dir+"/src/keep.go", "package main\n")
	write(t, dir+"/docs/notes.md", "notes\n")
	write(t, dir+"/old.txt", "old\n")
	write(t, dir+"/.gitignore", "build/\n*.log\n")
	write(t, dir+"/Zeta.txt", "z\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "init")

	write(t, dir+"/src/app.go", "package main\n\nfunc main() {}\n")
	write(t, dir+"/docs/notes.md", "notes\nmore\n")
	gitIn(t, dir, "add", "docs/notes.md")
	gitIn(t, dir, "rm", "-q", "old.txt")
	write(t, dir+"/src/new.ts", "export {}\n")
	write(t, dir+"/fresh/a.txt", "a\n")
	write(t, dir+"/build/out.bin", "x\n")
	write(t, dir+"/debug.log", "log\n")
	write(t, dir+"/.env", "A=1\n")
	if err := os.Symlink("README.md", dir+"/latest.md"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("src", dir+"/code"); err != nil {
		t.Fatal(err)
	}

	index, _ := os.ReadFile(dir + "/.git/index")
	var svc Service
	host := shell.Local{Name: "local"}
	root, err := svc.List(host, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(dir + "/.git/index"); string(after) != string(index) {
		t.Error("reading the tree rewrote the index")
	}
	if !root.Repo || root.Git != "" {
		t.Fatalf("the root reads as %+v", root)
	}
	want := "build code docs fresh src .env .gitignore debug.log latest.md old.txt README.md Zeta.txt"
	if got := names(root.Entries); got != want {
		t.Fatalf("the root lists %q, want %q", got, want)
	}
	entries := byName(root.Entries)
	checks := map[string]Entry{
		"build":     {Name: "build", Dir: true, Ignored: true},
		"code":      {Name: "code", Dir: true, Link: true, Mark: "?"},
		"docs":      {Name: "docs", Dir: true, Changes: true},
		"fresh":     {Name: "fresh", Dir: true, Mark: "?"},
		"src":       {Name: "src", Dir: true, Changes: true},
		"debug.log": {Name: "debug.log", Ignored: true},
		"latest.md": {Name: "latest.md", Link: true, Mark: "?"},
		"old.txt":   {Name: "old.txt", Mark: "D", Gone: true},
		"README.md": {Name: "README.md"},
		".env":      {Name: ".env", Mark: "?"},
	}
	for name, want := range checks {
		if got := entries[name]; got != want {
			t.Errorf("%s reads %+v, want %+v", name, got, want)
		}
	}

	src, err := svc.List(host, dir, "src")
	if err != nil {
		t.Fatal(err)
	}
	inSrc := byName(src.Entries)
	if inSrc["app.go"].Mark != "M" || inSrc["new.ts"].Mark != "?" || inSrc["keep.go"].Mark != "" {
		t.Errorf("src reads %+v", src.Entries)
	}
	docs, _ := svc.List(host, dir, "docs")
	if byName(docs.Entries)["notes.md"].Mark != "M" {
		t.Errorf("a staged edit reads %+v", docs.Entries)
	}
	fresh, _ := svc.List(host, dir, "fresh")
	if byName(fresh.Entries)["a.txt"].Mark != "?" {
		t.Errorf("a file in a new folder reads %+v", fresh.Entries)
	}
	build, _ := svc.List(host, dir, "build")
	if !byName(build.Entries)["out.bin"].Ignored {
		t.Errorf("a file in an ignored folder reads %+v", build.Entries)
	}
}

func TestATreeRootedInsideItsCheckoutReadsItsOwnPaths(t *testing.T) {
	dir := repo(t)
	write(t, dir+"/web/src/a.ts", "a\n")
	write(t, dir+"/web/gone/b.ts", "b\n")
	write(t, dir+"/web/gone/c.ts", "c\n")
	write(t, dir+"/other.go", "x\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "init")
	write(t, dir+"/web/src/a.ts", "a2\n")
	write(t, dir+"/other.go", "x2\n")
	if err := os.RemoveAll(dir + "/web/gone"); err != nil {
		t.Fatal(err)
	}

	var svc Service
	web, err := svc.List(shell.Local{Name: "local"}, dir+"/web", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(web.Entries); got != "gone src" {
		t.Fatalf("the tree's root lists %q", got)
	}
	gone := byName(web.Entries)["gone"]
	if !gone.Gone || !gone.Dir || !gone.Changes {
		t.Errorf("a deleted folder reads %+v", gone)
	}
	if !byName(web.Entries)["src"].Changes {
		t.Errorf("src reads %+v", web.Entries)
	}
	inSrc, _ := svc.List(shell.Local{Name: "local"}, dir+"/web", "src")
	if byName(inSrc.Entries)["a.ts"].Mark != "M" {
		t.Errorf("a file under the root reads %+v", inSrc.Entries)
	}
}

func TestAConflictReadsAsU(t *testing.T) {
	dir := repo(t)
	write(t, dir+"/a.txt", "base\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "base")
	gitIn(t, dir, "checkout", "-q", "-b", "side")
	write(t, dir+"/a.txt", "side\n")
	gitIn(t, dir, "commit", "-q", "-am", "side")
	gitIn(t, dir, "checkout", "-q", "main")
	write(t, dir+"/a.txt", "main\n")
	gitIn(t, dir, "commit", "-q", "-am", "main")
	cmd := exec.Command("git", "-c", "user.email=t@remuda.invalid", "-c", "user.name=t", "merge", "-q", "side")
	cmd.Dir = dir
	_ = cmd.Run() // the merge stops on the conflict
	var svc Service
	root, err := svc.List(shell.Local{Name: "local"}, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := byName(root.Entries)["a.txt"].Mark; got != "U" {
		t.Errorf("a file in conflict reads %q", got)
	}
}

func TestAFolderOutsideGitHasNoMarks(t *testing.T) {
	repo(t) // for the HOME and config it sets
	dir := t.TempDir()
	write(t, dir+"/a.txt", "a\n")
	var svc Service
	listing, err := svc.List(shell.Local{Name: "local"}, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if listing.Repo || len(listing.Entries) != 1 || listing.Entries[0].Mark != "" {
		t.Errorf("a plain folder reads %+v", listing)
	}
}

func TestAFolderShowsItsFirstFiveHundredFoldersFirst(t *testing.T) {
	repo(t)
	dir := t.TempDir()
	for i := range PerFolder + 20 {
		write(t, fmt.Sprintf("%s/f%04d.txt", dir, i), "")
	}
	if err := os.Mkdir(dir+"/zz-last-folder", 0o755); err != nil {
		t.Fatal(err)
	}
	var svc Service
	listing, err := svc.List(shell.Local{Name: "local"}, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if !listing.More || len(listing.Entries) != PerFolder || listing.Entries[0].Name != "zz-last-folder" {
		t.Errorf("a full folder reads more=%v, %d entries, first %q", listing.More, len(listing.Entries), listing.Entries[0].Name)
	}
}

func TestAnyNameIsListedWhole(t *testing.T) {
	repo(t)
	dir := t.TempDir()
	odd := []string{"a b.txt", "line\nbreak.txt", "-dash", "'quote'", "ünïcödé.md"}
	for _, name := range odd {
		write(t, dir+"/"+name, "")
	}
	write(t, dir+"/.git/HEAD", "")
	var svc Service
	listing, err := svc.List(shell.Local{Name: "local"}, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	got := byName(listing.Entries)
	for _, name := range odd {
		if _, ok := got[name]; !ok {
			t.Errorf("%q is not listed: %q", name, names(listing.Entries))
		}
	}
	if _, ok := got[".git"]; ok {
		t.Error(".git is listed")
	}
}

func TestAPathOutOfTheRootIsRefused(t *testing.T) {
	var svc Service
	host := &shell.Fake{Name: "m", Dir: "/home/me"}
	for _, rel := range []string{"..", "src/../..", "./src", "a//b", "/etc"} {
		if _, err := svc.List(host, "/home/me/app", rel); err == nil {
			t.Errorf("%q was listed", rel)
		}
	}
	if len(host.Scripts()) != 0 {
		t.Errorf("a refused path ran %v", host.Scripts())
	}
	if _, err := svc.List(host, "app", ""); err == nil {
		t.Error("a relative root was listed")
	}
}

func TestAWindowsFolderIsReadInGitBashsSpelling(t *testing.T) {
	var svc Service
	host := &shell.Fake{Name: "pc", Dir: `C:\Users\me`, Remote: true, Answer: func(s string) (string, error) {
		if strings.Contains(s, "status --porcelain") {
			return "repo\nsrc/\n\x1e\n M src/app.cs\x00?? src/new.xaml\x00\x1e\n", nil
		}
		return "f\x1fapp.cs\x00f\x1fnew.xaml\x00", nil
	}}
	listing, err := svc.List(host, `C:\Users\me\proj\src`, "")
	if err != nil {
		t.Fatal(err)
	}
	scripts := host.Scripts()
	if len(scripts) != 2 || !strings.Contains(scripts[0], "cd 'C:/Users/me/proj/src'") || !strings.Contains(scripts[1], "cd 'C:/Users/me/proj/src'") {
		t.Fatalf("the scripts read %q", scripts)
	}
	got := byName(listing.Entries)
	if got["app.cs"].Mark != "M" || got["new.xaml"].Mark != "?" {
		t.Errorf("the marks read %+v", listing.Entries)
	}
	if _, err := svc.List(host, `C:\Users\me\proj\src`, `a\..\..`); err == nil {
		t.Error("a backslash climbed out")
	}
}

func TestOneStatusServesTheFoldersReadWithinItsWindow(t *testing.T) {
	now := time.Unix(1000, 0)
	svc := Service{Now: func() time.Time { return now }}
	statuses := 0
	host := &shell.Fake{Name: "m", Dir: "/home/me", Answer: func(s string) (string, error) {
		if strings.Contains(s, "status --porcelain") {
			statuses++
			return "repo\n\n\x1e\n", nil
		}
		return "", nil
	}}
	for _, rel := range []string{"", "src", "docs"} {
		if _, err := svc.List(host, "/home/me/app", rel); err != nil {
			t.Fatal(err)
		}
	}
	if statuses != 1 {
		t.Fatalf("three folders read %d statuses", statuses)
	}
	now = now.Add(StatusFor + time.Second)
	svc.List(host, "/home/me/app", "")
	if statuses != 2 {
		t.Fatalf("a status past its window was not read again: %d", statuses)
	}
	svc.List(host, "/home/me/other", "")
	if statuses != 3 {
		t.Fatalf("another tree shares the first one's status: %d", statuses)
	}
}

func TestFoldersAskedAtOnceShareOneStatus(t *testing.T) {
	var svc Service
	var mu sync.Mutex
	statuses := 0
	host := &shell.Fake{Name: "m", Dir: "/home/me", Answer: func(s string) (string, error) {
		if strings.Contains(s, "status --porcelain") {
			mu.Lock()
			statuses++
			mu.Unlock()
			time.Sleep(50 * time.Millisecond)
			return "repo\n\n\x1e\n M a.go\x00", nil
		}
		return "f\x1fa.go\x00", nil
	}}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			listing, err := svc.List(host, "/home/me/app", "")
			if err != nil || listing.Entries[0].Mark != "M" {
				t.Errorf("a folder read at once reads %+v, %v", listing, err)
			}
		}()
	}
	wg.Wait()
	if statuses != 1 {
		t.Errorf("eight folders at once read %d statuses", statuses)
	}
}

func TestAStatusCutShortDropsItsLastRecord(t *testing.T) {
	st := ParseStatus("repo\n\n\x1e\n M a.go\x00 M b.g")
	if st.marks["a.go"] != "M" || st.marks["b.g"] != "" {
		t.Errorf("a cut status reads %v", st.marks)
	}
	if st := ParseStatus("none\n"); st.Repo {
		t.Error("a folder outside git reads as a checkout")
	}
	if st := ParseStatus("repo\n\n\x1e\n\x1e\nfatal: index file corrupt"); st.Err != "fatal: index file corrupt" {
		t.Errorf("git's complaint reads %q", st.Err)
	}
}

func TestATreeInsideAnIgnoredFolderIsIgnoredWhole(t *testing.T) {
	st := ParseStatus("repo\nbuild/out/\n\x1e\n!! build/\x00")
	entries := st.Apply("", []Entry{{Name: "x.bin"}}, false)
	if !entries[0].Ignored {
		t.Errorf("a file under an ignored root reads %+v", entries[0])
	}
}
