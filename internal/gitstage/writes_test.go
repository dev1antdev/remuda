package gitstage

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
)

// scratch is a temporary repository with one commit, git told nothing of the person's own config, hooks or signing.
func scratch(t *testing.T) (dir string, git func(args ...string) string, write func(name, body string)) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, name := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(name, "Test")
	}
	for _, name := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(name, "test@example.com")
	}
	dir = t.TempDir()
	git = func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
		return strings.TrimRight(string(output), "\n") // a status line begins with a blank for the index's column
	}
	write = func(name, body string) {
		path := filepath.Join(dir, name)
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	return dir, git, write
}

var host = shell.Local{Name: "local"}

func status(t *testing.T, git func(args ...string) string) string {
	t.Helper()
	return git("status", "--porcelain=v1", "--untracked-files=all")
}

func TestStageAndUnstageEveryKindOfFile(t *testing.T) {
	dir, git, write := scratch(t)
	write("a.go", "a\n")
	write("old.txt", "old\n")
	write("[a].txt", "brackets\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	write("a.go", "a2\n")
	_ = os.Remove(filepath.Join(dir, "old.txt"))
	write("new file.ts", "new\n")
	write("[a].txt", "brackets, changed\n")
	write("a.txt", "a file the pattern [a].txt would also name\n")

	s := &Service{}
	done, err := s.Stage(host, dir, []string{"a.go", "old.txt", "new file.ts", "[a].txt"}, true)
	if err != nil || len(done.Paths) != 4 {
		t.Fatalf("stage: %+v %v", done, err)
	}
	if got := status(t, git); !strings.Contains(got, "M  a.go") || !strings.Contains(got, "D  old.txt") || !strings.Contains(got, `A  "new file.ts"`) || !strings.Contains(got, "M  [a].txt") || !strings.Contains(got, "?? a.txt") {
		t.Fatalf("after staging:\n%s", got)
	}
	if _, err := s.Unstage(host, dir, []string{"a.go", "old.txt", "new file.ts", "[a].txt"}, true); err != nil {
		t.Fatal(err)
	}
	if got := status(t, git); !strings.Contains(got, " M a.go") || !strings.Contains(got, " D old.txt") || !strings.Contains(got, `?? "new file.ts"`) || !strings.Contains(got, " M [a].txt") {
		t.Fatalf("after unstaging, the working tree must stand as it was:\n%s", got)
	}
	if body, _ := os.ReadFile(filepath.Join(dir, "a.go")); string(body) != "a2\n" {
		t.Fatalf("unstaging touched the working tree: %q", body)
	}
}

func TestAFilePartlyStagedKeepsBothSides(t *testing.T) {
	dir, git, write := scratch(t)
	write("a.go", "one\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	write("a.go", "one\ntwo\n")
	s := &Service{}
	if _, err := s.Stage(host, dir, []string{"a.go"}, false); err != nil {
		t.Fatal(err)
	}
	write("a.go", "one\ntwo\nthree\n")
	if got := status(t, git); got != "MM a.go" {
		t.Fatalf("status: %q", got)
	}
	if err := s.Discard(host, dir, "a.go", DiscardUnstaged); err != nil {
		t.Fatal(err)
	}
	if body, _ := os.ReadFile(filepath.Join(dir, "a.go")); string(body) != "one\ntwo\n" || status(t, git) != "M  a.go" {
		t.Fatalf("a discard of the unstaged edits took the staged ones too: %q %q", body, status(t, git))
	}
	if err := s.Discard(host, dir, "a.go", DiscardAll); err != nil {
		t.Fatal(err)
	}
	if body, _ := os.ReadFile(filepath.Join(dir, "a.go")); string(body) != "one\n" || status(t, git) != "" {
		t.Fatalf("a discard of everything left %q %q", body, status(t, git))
	}
}

func TestDiscardDeletesOnlyThePickedNewFile(t *testing.T) {
	dir, git, write := scratch(t)
	write("keep.txt", "keep\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	write("added.go", "staged new\n")
	git("add", "added.go")
	write("loose.txt", "untracked\n")
	write("other.txt", "untracked too\n")
	write("ignored.log", "ignored\n")
	write(".gitignore", "*.log\n")
	s := &Service{}
	if err := s.Discard(host, dir, "keep.txt", DiscardUntracked); err == nil || !strings.Contains(err.Error(), "tracks") {
		t.Fatalf("a tracked file was discarded as untracked: %v", err)
	}
	if err := s.Discard(host, dir, "loose.txt", DiscardUntracked); err != nil {
		t.Fatal(err)
	}
	if err := s.Discard(host, dir, "added.go", DiscardAll); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{"loose.txt": false, "added.go": false, "other.txt": true, "ignored.log": true, "keep.txt": true} {
		if _, err := os.Stat(filepath.Join(dir, name)); (err == nil) != want {
			t.Errorf("%s there: %v, want %v", name, err == nil, want)
		}
	}
}

func TestUnstageBeforeTheFirstCommit(t *testing.T) {
	dir, git, write := scratch(t)
	write("first.go", "first\n")
	s := &Service{}
	if _, err := s.Stage(host, dir, []string{"first.go"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Unstage(host, dir, []string{"first.go"}, false); err != nil {
		t.Fatal(err)
	}
	if got := status(t, git); got != "?? first.go" {
		t.Fatalf("status: %q", got)
	}
}

func TestCommitStagedCommitAllAndAmend(t *testing.T) {
	dir, git, write := scratch(t)
	write("a.go", "a\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	s := &Service{}
	if _, err := s.Commit(host, dir, CommitRequest{Message: "nothing here"}); !IsRefusal(err) || !strings.Contains(err.Error(), "nothing is staged") {
		t.Fatalf("an empty commit: %v", err)
	}
	if _, err := s.Commit(host, dir, CommitRequest{Message: "  \n"}); !IsRefusal(err) {
		t.Fatalf("no message: %v", err)
	}
	write("a.go", "a2\n")
	write("b.go", "b\n")
	made, err := s.Commit(host, dir, CommitRequest{Message: "Both files, staged first\n\nWith a body that says why.", Stage: []string{"a.go", "b.go"}})
	if err != nil || made.Subject != "Both files, staged first" || len(made.Short) < 4 {
		t.Fatalf("commit all: %+v %v", made, err)
	}
	if got := status(t, git); got != "" {
		t.Fatalf("commit all left %q", got)
	}
	if body := git("log", "-1", "--format=%B"); !strings.Contains(body, "With a body that says why.") {
		t.Fatalf("message: %q", body)
	}
	head, message, err := LastCommit(host, dir)
	if err != nil || head != made.SHA || !strings.HasPrefix(message, "Both files") {
		t.Fatalf("last commit: %q %q %v", head, message, err)
	}
	if _, err := s.Commit(host, dir, CommitRequest{Message: "rewritten", Amend: true, Head: strings.Repeat("a", 40)}); !IsRefusal(err) || !strings.Contains(err.Error(), "changed since") {
		t.Fatalf("an amend on a HEAD that moved: %v", err)
	}
	amended, err := s.Commit(host, dir, CommitRequest{Message: "Both files, reworded", Amend: true, Head: head})
	if err != nil || amended.SHA == head || git("rev-list", "--count", "HEAD") != "2" || git("log", "-1", "--format=%s") != "Both files, reworded" {
		t.Fatalf("amend: %+v %v", amended, err)
	}
}

func TestACommitHookRefusalIsGitsOwnWords(t *testing.T) {
	dir, git, write := scratch(t)
	write("a.go", "a\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	hook := filepath.Join(dir, ".git", "hooks", "pre-commit")
	_ = os.WriteFile(hook, []byte("#!/bin/sh\necho 'lint: a.go is not formatted'\nexit 1\n"), 0o755)
	write("a.go", "a2\n")
	s := &Service{}
	_, err := s.Commit(host, dir, CommitRequest{Message: "Format later", Stage: []string{"a.go"}})
	var refused *GitError
	if !errors.As(err, &refused) || !strings.Contains(refused.Output, "lint: a.go is not formatted") {
		t.Fatalf("the hook's words: %v", err)
	}
}

func TestAConflictIsLeftToThePerson(t *testing.T) {
	dir, git, write := scratch(t)
	write("c.go", "base\n")
	write("d.go", "d\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	git("checkout", "-q", "-b", "other")
	write("c.go", "theirs\n")
	git("commit", "-qam", "theirs")
	git("checkout", "-q", "main")
	write("c.go", "ours\n")
	git("commit", "-qam", "ours")
	cmd := exec.Command("git", "merge", "-q", "other")
	cmd.Dir = dir
	_ = cmd.Run() // stops on the conflict
	write("d.go", "d2\n")

	s := &Service{}
	done, err := s.Stage(host, dir, []string{"c.go", "d.go"}, true)
	if err != nil || len(done.Paths) != 1 || done.Paths[0] != "d.go" || len(done.Skipped) != 1 {
		t.Fatalf("Stage all: %+v %v", done, err)
	}
	if _, err := s.Stage(host, dir, []string{"c.go"}, false); !IsRefusal(err) || !strings.Contains(err.Error(), "conflict markers") {
		t.Fatalf("staging a file with its markers in: %v", err)
	}
	if err := s.Discard(host, dir, "c.go", DiscardAll); !IsRefusal(err) {
		t.Fatalf("discarding a conflict: %v", err)
	}
	if _, err := s.Commit(host, dir, CommitRequest{Message: "merge"}); !IsRefusal(err) || !strings.Contains(err.Error(), "resolve the conflicts") {
		t.Fatalf("a commit over a conflict: %v", err)
	}
	write("c.go", "ours and theirs\n")
	if _, err := s.Stage(host, dir, []string{"c.go"}, false); err != nil {
		t.Fatalf("staging the resolved file: %v", err)
	}
	if _, err := s.Commit(host, dir, CommitRequest{Message: "Merge other"}); err != nil {
		t.Fatalf("the merge commit: %v", err)
	}
}

func TestAWriteWaitsForTheAgent(t *testing.T) {
	dir, git, write := scratch(t)
	write("a.go", "a\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	write("a.go", "a2\n")
	var asked []string
	s := &Service{Busy: func(machineID, folder, top string) string {
		asked = append(asked, machineID, folder, top)
		return "Claude Code"
	}}
	_, err := s.Stage(host, dir, []string{"a.go"}, false)
	if !IsRefusal(err) || !strings.Contains(err.Error(), "Claude Code is working") {
		t.Fatalf("a stage under an agent's feet: %v", err)
	}
	real, _ := filepath.EvalSymlinks(dir)
	if len(asked) != 3 || asked[0] != "local" || asked[1] != dir || (asked[2] != dir && asked[2] != real) {
		t.Fatalf("asked %q", asked)
	}
	if got := status(t, git); got != " M a.go" {
		t.Fatalf("the refused stage staged: %q", got)
	}
}

func TestPathsTravelInChunksAndStayInTheFolder(t *testing.T) {
	dir, git, write := scratch(t)
	write("seed", "seed\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	var paths []string
	for i := range 450 {
		name := fmt.Sprintf("many/file %03d.txt", i)
		write(name, "x\n")
		paths = append(paths, name)
	}
	s := &Service{}
	if done, err := s.Stage(host, dir, paths, true); err != nil || len(done.Paths) != 450 {
		t.Fatalf("%d staged, %v", len(done.Paths), err)
	}
	if got := git("diff", "--cached", "--name-only"); strings.Count(got, "\n")+1 != 450 {
		t.Fatalf("staged %d", strings.Count(got, "\n")+1)
	}
	for _, bad := range []string{"../outside", "/etc/passwd", "-rf", "a/../../b"} {
		if _, err := s.Stage(host, dir, []string{bad}, false); !IsRefusal(err) {
			t.Errorf("%q reached git: %v", bad, err)
		}
	}
	if _, err := s.Stage(host, "relative/dir", []string{"a"}, false); !IsRefusal(err) {
		t.Errorf("a relative folder: %v", err)
	}
}
