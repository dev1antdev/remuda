package transcript

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// TestALearnedFolderAndItsSessionOutliveARestart: what an accepted process read is kept in the state dir, so the next
// run of remuda reads that conversation under the folder its agent named — while an id nobody read there is not found
// under a folder whose process reads lapsed long ago.
func TestALearnedFolderAndItsSessionOutliveARestart(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store := OpenRootStore(t.TempDir())
	host := shell.Local{Name: "local"}
	root := filepath.Join(t.TempDir(), "profile", "projects")
	const read, unknown = "4c9d6f0e-1111-4222-8333-944455556666", "5d0e7f1f-2222-4333-8444-a55566667777"
	path := filepath.Join(root, "-work", read+".jsonl")
	for _, name := range []string{read, unknown} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "-work", name+".jsonl"), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	first := NewReader(host).Keeping(store)
	first.addRoot(Claude, root)
	first.rememberSession(read, path)

	// the agent stops and remuda restarts: nothing is fresh, nothing is in memory
	next := NewReader(host).Keeping(store)
	if got, err := next.ResolveSession("claude", "/work", "id", read); err != nil || got != path {
		t.Fatalf("the conversation read there: %q, %v; want %s", got, err, path)
	}
	if _, err := next.ResolveSession("claude", "/work", "id", unknown); err == nil {
		t.Fatal("an id nobody read there is found under a lapsed folder")
	}
	if !slices.Contains(next.Roots()[Claude], root) {
		t.Fatalf("the folder is dropped from the machine's history: %v", next.Roots())
	}
}

// TestAFolderThatIsGoneIsForgotten: a kept folder checked and found gone leaves the machine's roots and the file; one
// the machine could not be asked about stays.
func TestAFolderThatIsGoneIsForgotten(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	store := OpenRootStore(dir)
	host := shell.Local{Name: "local"}
	root := filepath.Join(t.TempDir(), "profile", "projects")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	first := NewReader(host).Keeping(store)
	first.addRoot(Claude, root)
	if !slices.Contains(first.Roots()[Claude], root) {
		t.Fatalf("learned folder: %v", first.Roots())
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	next := NewReader(host).Keeping(store)
	if slices.Contains(next.Roots()[Claude], root) {
		t.Fatalf("a folder that is gone is still offered: %v", next.Roots())
	}
	if slices.Contains(store.Loaded(host.ID()).Roots[Claude], root) {
		t.Fatal("a folder that is gone is still kept")
	}

	unasked := &shell.Fake{Name: "remote", Dir: t.TempDir(), Remote: true, Answer: func(string) (string, error) {
		return "", errors.New("the connection went away")
	}}
	kept := NewReader(unasked).Keeping(store)
	kept.addRoot(Claude, root)
	if !slices.Contains(kept.Roots()[Claude], root) {
		t.Fatalf("a folder the machine could not be asked about is dropped: %v", kept.Roots())
	}
}

// TestSessionsOfLearnsOnlyAcceptedRoots: the folder an agent named for itself is kept when the process read is one
// SessionsOf answers for, never from a record the pid check rejected.
func TestSessionsOfLearnsOnlyAcceptedRoots(t *testing.T) {
	const id = "4c9d6f0e-1111-4222-8333-944455556666"
	for _, c := range []struct {
		name     string
		start    string
		accepted bool
	}{
		{"accepted", "Thu Nov 19 00:36:10 2026", true},
		{"rejected", "-", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			config := filepath.Join(home, "profile")
			pidFile := filepath.Join(config, "sessions", "7.json")
			payload := pidFile + script.FS + `{"pid":7,"sessionId":"` + id + `","cwd":"/work","procStart":"Thu Nov 19 00:36:10 2026"}`
			out := "7" + script.US + "claude" + script.US + c.start + script.US + "/work" + script.US +
				payload + script.US + c.start + script.RS + "\n"
			host := &shell.Fake{Name: "remote", Dir: home, Remote: true, Answer: func(string) (string, error) { return out, nil }}
			reader := NewReader(host)
			got, err := reader.SessionsOf([]Running{{PID: 7, Agent: "claude", CWD: "/work"}})
			if err != nil {
				t.Fatal(err)
			}
			projects := filepath.Join(config, "projects")
			if c.accepted {
				if got[7] != id || !slices.Contains(reader.Roots()[Claude], projects) {
					t.Fatalf("session %q, roots %v: want %s learned", got[7], reader.Roots(), projects)
				}
			} else if got[7] != "" || slices.Contains(reader.Roots()[Claude], projects) {
				t.Fatalf("a rejected pid taught %v: %v", projects, reader.Roots())
			}
		})
	}
}

// TestResolvedSessionFollowsTheRoots: a miss kept for one set of session folders is read again once another root is
// learned, so an id under a folder an agent named later is still found.
func TestResolvedSessionFollowsTheRoots(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	reader := NewReader(shell.Local{Name: "local"})
	const id = "4c9d6f0e-1111-4222-8333-944455556666"
	cwd := "/work/app"

	first := filepath.Join(t.TempDir(), "projects")
	reader.addRoot(Claude, first)
	if _, err := reader.ResolveSession("claude", cwd, "id", id); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("first resolve: %v", err)
	}

	second := filepath.Join(t.TempDir(), "projects")
	path := filepath.Join(second, EncodePath(cwd), id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader.addRoot(Claude, second)
	got, err := reader.ResolveSession("claude", cwd, "id", id)
	if err != nil || got != path {
		t.Fatalf("after another root was learned: %q, %v; want %s", got, err, path)
	}
}

// TestALearnedRootExpiresForANewResume: an id is found under a folder an agent named while that read is fresh, and not
// after it lapses: the reader keeps the folder for the sessions it already read, not as a place to look for a
// conversation nobody saw there.
func TestALearnedRootExpiresForANewResume(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	reader := NewReader(shell.Local{Name: "local"})
	root := filepath.Join(t.TempDir(), "projects")
	const id = "4c9d6f0e-1111-4222-8333-944455556666"
	path := filepath.Join(root, "-work", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader.addRoot(Claude, root)
	if got, err := reader.ResolveSession("claude", "/work", "id", id); err != nil || got != path {
		t.Fatalf("fresh root: %q, %v", got, err)
	}
	reader.mu.Lock()
	for dir := range reader.learnedRoots[Claude] {
		reader.learnedRoots[Claude][dir] = time.Now().Add(-time.Second)
	}
	reader.mu.Unlock()
	if _, err := reader.ResolveSession("claude", "/work", "id", id); err == nil {
		t.Fatal("an id was found under an expired root")
	}
	if !slices.Contains(reader.Roots()[Claude], root) {
		t.Fatalf("the folder an agent left is dropped from the machine's history: %v", reader.Roots())
	}
}

// TestTheLiveSessionWinsOverACopyUnderAnotherRoot: the same id may sit in the default root and a root an agent named;
// the accepted live file opens, and a remote lookup names one file rather than both.
func TestTheLiveSessionWinsOverACopyUnderAnotherRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	const id = "4c9d6f0e-1111-4222-8333-944455556666"
	cwd := "/work/app"
	write := func(root string) string {
		path := filepath.Join(root, EncodePath(cwd), id+".jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	defaultPath := write(RootsOf(home)[Claude][0])
	ownRoot := filepath.Join(home, "profile", "projects")
	ownPath := write(ownRoot)
	reader := NewReader(shell.Local{Name: "local"})
	reader.addRoot(Claude, ownRoot)
	reader.rememberSession(id, ownPath)
	if got, err := reader.ResolveSession("claude", cwd, "id", id); err != nil || got != ownPath {
		t.Fatalf("the live session = %q, %v; want %s", got, err, ownPath)
	}
	roots := reader.Roots()
	out, err := shell.Text(shell.Local{Name: "local"}, findScript(roots, Claude, home, cwd, id), 5*time.Second)
	if err != nil || strings.TrimSpace(out) != defaultPath {
		t.Fatalf("a remote lookup = %q, %v; want one file %s", out, err, defaultPath)
	}
	missing := "00000000-0000-4000-8000-000000000000"
	if out, err := shell.Text(shell.Local{Name: "local"}, findScript(roots, Claude, home, cwd, missing), 5*time.Second); err != nil || out != "" {
		t.Fatalf("a miss = %q, %v; want a successful empty answer", out, err)
	}
}

// TestARootIsAnAbsoluteFolder: what a process says is cleaned as the machine spells it and a relative folder is no root.
func TestARootIsAnAbsoluteFolder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	reader := NewReader(shell.Local{Name: "local"})
	relative := "profile/projects"
	reader.addRoot(Claude, relative)
	if slices.Contains(reader.Roots()[Claude], relative) {
		t.Fatalf("a relative folder was learned: %v", reader.Roots())
	}
	windows := `C:/Users/me/profile/projects`
	host := &shell.Fake{Name: "pc", Dir: `C:\Users\me`, Remote: true}
	pc := NewReader(host)
	pc.addRoot(Claude, windows)
	clean := `C:\Users\me\profile\projects`
	if !slices.Contains(pc.Roots()[Claude], clean) {
		t.Fatalf("a Windows root was not cleaned: %v", pc.Roots())
	}
}
