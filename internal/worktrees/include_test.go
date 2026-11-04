package worktrees

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
)

// repoWithWorktree is a repository with a commit, the given files written, and a linked worktree beside it.
func repoWithWorktree(t *testing.T, files map[string]string) (main, worktree string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root := t.TempDir()
	main = filepath.Join(root, "repo")
	worktree = filepath.Join(root, "repo-wt")
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	git(main, "init", "-q")
	for name, body := range files {
		path := filepath.Join(main, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(main, "add", "-f", ".gitignore", "tracked.env") // tracked although its name is ignored: a tracked file is never copied
	git(main, "commit", "-qm", "init")
	git(main, "worktree", "add", "-q", "-b", "task-41", worktree)
	return main, worktree
}

func TestIncludeCopiesTheIgnoredFilesTheListNames(t *testing.T) {
	main, worktree := repoWithWorktree(t, map[string]string{
		".gitignore":           "*.env\n.env*\nconfig/local/\nsecrets/\nbuild/\n",
		".worktreeinclude":     ".env\n.env.*\n!.env.example\nconfig/local/\n*.pem\nnot-ignored.txt\n",
		".env":                 "TOKEN=1\n",
		".env.development":     "DEV=1\n",
		".env.example":         "TOKEN=\n",
		"tracked.env":          "tracked\n",
		"config/local/a.json":  "{}\n",
		"config/local/b/c.yml": "x: 1\n",
		"secrets/key.pem":      "pem\n",
		"build/out.js":         "built\n",
		"not-ignored.txt":      "untracked, not ignored\n",
	})
	if err := os.Symlink("/etc/hosts", filepath.Join(main, ".env.outside")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, ".env.development"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	in, err := Include(shell.Local{Name: "local"}, worktree)
	if err != nil {
		t.Fatal(err)
	}
	if !in.Listed || in.TooMany != 0 {
		t.Fatalf("included = %+v", in)
	}
	wantMain, _ := filepath.EvalSymlinks(main)
	if in.Main != wantMain {
		t.Fatalf("main = %q, want %q", in.Main, wantMain)
	}
	copied := append([]string(nil), in.Copied...)
	sort.Strings(copied)
	if want := []string{".env", "config/local/a.json", "config/local/b/c.yml", "secrets/key.pem"}; strings.Join(copied, ",") != strings.Join(want, ",") {
		t.Fatalf("copied = %v, want %v", copied, want)
	}
	skipped := map[string]string{}
	for _, s := range in.Skipped {
		skipped[s.Path] = s.Why
	}
	if skipped[".env.development"] != "already in the worktree" || skipped[".env.outside"] != "a link out of the repository" || len(skipped) != 2 {
		t.Fatalf("skipped = %v", skipped)
	}
	for _, name := range []string{".env", "config/local/b/c.yml", "secrets/key.pem"} {
		if _, err := os.Stat(filepath.Join(worktree, name)); err != nil {
			t.Fatalf("%s not in the worktree: %v", name, err)
		}
	}
	for _, name := range []string{".env.example", "build/out.js", "not-ignored.txt", ".env.outside"} {
		if _, err := os.Lstat(filepath.Join(worktree, name)); err == nil {
			t.Fatalf("%s should not be in the worktree", name)
		}
	}
	if body, _ := os.ReadFile(filepath.Join(worktree, ".env.development")); string(body) != "mine\n" {
		t.Fatalf("an existing file was overwritten: %q", body)
	}
	if body, _ := os.ReadFile(filepath.Join(worktree, "tracked.env")); string(body) != "tracked\n" {
		t.Fatalf("the tracked file is git's own copy: %q", body)
	}
	if words := in.Words(); !strings.Contains(words, "Copied 4 files") || !strings.Contains(words, ".env.outside (a link out of the repository)") {
		t.Fatalf("words = %q", words)
	}
}

func TestIncludeKeepsALinkThatStaysInsideAndWritesNothingOutsideTheWorktree(t *testing.T) {
	main, worktree := repoWithWorktree(t, map[string]string{
		".gitignore":       "*.local\nshared/\nout/\n",
		".worktreeinclude": "*.local\nout/\n",
		"real.local":       "real\n",
		"out/x.local":      "x\n",
		"tracked.env":      "t\n",
	})
	if err := os.Symlink("real.local", filepath.Join(main, "alias.local")); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(worktree, "out")); err != nil {
		t.Fatal(err)
	}

	in, err := Include(shell.Local{Name: "local"}, worktree)
	if err != nil {
		t.Fatal(err)
	}
	if target, err := os.Readlink(filepath.Join(worktree, "alias.local")); err != nil || target != "real.local" {
		t.Fatalf("a link inside the repository is copied as a link: %q %v", target, err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("a file was written through the worktree's link out of it: %v", entries)
	}
	why := ""
	for _, s := range in.Skipped {
		if s.Path == "out/x.local" {
			why = s.Why
		}
	}
	if why != "its folder in the worktree leads out of it" {
		t.Fatalf("skipped = %+v", in.Skipped)
	}
}

func TestIncludeWithoutTheFileCopiesNothingAndSaysNothing(t *testing.T) {
	_, worktree := repoWithWorktree(t, map[string]string{".gitignore": "*.env\n", ".env": "x\n", "tracked.env": "t\n"})
	in, err := Include(shell.Local{Name: "local"}, worktree)
	if err != nil || in.Listed || in.Words() != "" {
		t.Fatalf("included = %+v, %v", in, err)
	}
	if _, err := os.Stat(filepath.Join(worktree, ".env")); err == nil {
		t.Fatal(".env was copied without a .worktreeinclude")
	}
}

func TestIncludeReadsTheAnswerAndSaysWhatHappened(t *testing.T) {
	answer := "main\x1f/home/me/repo\ncopied\x1f.env\ncopied\x1fconfig/local.json\nskipped\x1f.env.local\x1falready in the worktree\n"
	host := &shell.Fake{Name: "box", Remote: true, Answer: func(string) (string, error) { return answer, nil }}
	in, err := Include(host, "/home/me/repo-wt")
	if err != nil {
		t.Fatal(err)
	}
	if !in.Listed || in.Main != "/home/me/repo" || len(in.Copied) != 2 || in.Skipped[0].Why != "already in the worktree" {
		t.Fatalf("included = %+v", in)
	}
	if s := host.Scripts()[0]; !strings.Contains(s, "cd '/home/me/repo-wt'") || !strings.Contains(s, "--exclude-from=.worktreeinclude") || !strings.Contains(s, "check-ignore --stdin") || !strings.Contains(s, "cp -pP") {
		t.Fatalf("script = %q", s)
	}
	if words := in.Words(); words != "Copied 2 files the repository's .worktreeinclude names into the new worktree: .env, config/local.json. Left out: .env.local (already in the worktree)." {
		t.Fatalf("words = %q", words)
	}

	tooMany := ParseIncluded("main\x1f/r\ntoomany\x1f5400\n")
	if tooMany.TooMany != 5400 || !strings.Contains(tooMany.Words(), "nothing was copied") {
		t.Fatalf("too many = %+v %q", tooMany, tooMany.Words())
	}
	none := ParseIncluded("main\x1f/r\n")
	if none.Words() != "The repository's .worktreeinclude matches no ignored file: nothing to copy." {
		t.Fatalf("none = %q", none.Words())
	}

	gone := &shell.Fake{Name: "box", Answer: func(string) (string, error) { return "the new worktree is not there\n", &shell.ExitError{Code: 3} }}
	if _, err := Include(gone, "/w"); err == nil || err.Error() != "the new worktree is not there" {
		t.Fatalf("err = %v", err)
	}
	if someOf([]string{"a", "b", "c"}, 2) != "a, b and 1 more" {
		t.Fatal(someOf([]string{"a", "b", "c"}, 2))
	}
}
