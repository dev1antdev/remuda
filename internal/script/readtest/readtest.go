// Package readtest checks that a script reading a checkout leaves it alone.
package readtest

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// LeavesIndex gives read a checkout on a branch "work" beside "main", whose file touched.txt has a stale stat and whose
// fsmonitor hook leaves a mark, and fails if read wrote the index or ran the hook. It first shows that a plain git does
// both, so the check cannot pass for nothing.
func LeavesIndex(t *testing.T, read func(dir string)) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	empty := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(empty, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(path, body string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	touched, index := filepath.Join(dir, "touched.txt"), filepath.Join(dir, ".git", "index")
	git("init", "-q")
	write(touched, "the same\n", 0o600)
	write(filepath.Join(dir, "edited.txt"), "one\n", 0o600)
	git("add", "-A")
	git("commit", "-qm", "init")
	git("branch", "-M", "main")
	git("checkout", "-qb", "work")
	write(filepath.Join(dir, "edited.txt"), "one\ntwo\n", 0o600)
	write(filepath.Join(dir, "new.txt"), "new\n", 0o600)

	mark, hook := filepath.Join(empty, "fsmonitor-ran"), filepath.Join(empty, "fsmonitor")
	write(hook, "#!/bin/sh\n: > '"+mark+"'\nexit 1\n", 0o700)
	git("config", "core.fsmonitor", hook)

	stale := func(ago time.Duration) []byte {
		t.Helper()
		at := time.Now().Add(-ago)
		if err := os.Chtimes(touched, at, at); err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(index)
		if err != nil {
			t.Fatal(err)
		}
		return before
	}
	before := stale(time.Hour)
	git("--no-optional-locks", "diff", "--numstat")
	after, _ := os.ReadFile(index)
	if bytes.Equal(before, after) {
		t.Fatal("the check proves nothing: a plain diff of a touched file left the index as it was")
	}
	if _, err := os.Stat(mark); err != nil {
		t.Fatal("the check proves nothing: a plain diff ran no fsmonitor hook")
	}
	os.Remove(mark)

	before = stale(2 * time.Hour)
	read(dir)
	if after, _ := os.ReadFile(index); !bytes.Equal(before, after) {
		t.Error("the read wrote the index")
	}
	if _, err := os.Stat(mark); err == nil {
		t.Error("the read ran the fsmonitor hook")
	}
}
