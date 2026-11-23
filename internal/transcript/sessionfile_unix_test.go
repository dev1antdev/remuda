//go:build !windows

package transcript

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestOpenSessionRefusesLinksAndPipes: a learned folder may hold anything with a session file's name; a reader refuses a
// link and a named pipe rather than following one or waiting for the other.
func TestOpenSessionRefusesLinksAndPipes(t *testing.T) {
	dir := t.TempDir()
	session := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(session, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := OpenSession(session)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	link := filepath.Join(dir, "link.jsonl")
	if err := os.Symlink(session, link); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSession(link); err == nil {
		t.Fatal("a link to a session file was opened")
	}

	pipe := filepath.Join(dir, "pipe.jsonl")
	if err := syscall.Mkfifo(pipe, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSession(pipe); err == nil {
		t.Fatal("a named pipe was opened as a session file")
	}

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "s.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "project")); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenInRoot(root, filepath.Join(root, "project", "s.jsonl")); err == nil {
		t.Fatal("a session under a linked project folder left its root")
	}
	own := filepath.Join(root, "own")
	if err := os.Mkdir(own, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(own, "s.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err = OpenInRoot(root, filepath.Join(own, "s.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
}
