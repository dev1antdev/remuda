package sshx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/script"
)

// fakeSSH puts an `ssh` first on PATH that records each call's arguments, one call a line, and answers yes to all.
func fakeSSH(t *testing.T) (calls string) {
	t.Helper()
	dir := t.TempDir()
	calls = filepath.Join(dir, "calls")
	stub := "#!/bin/sh\ncase \"$*\" in *\" -G \"*) exit 1 ;; esac\nprintf '%s\\n' \"$*\" >> " + script.Quote(calls) + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return calls
}

// recorded is the lines the fake ssh was called with.
func recorded(t *testing.T, calls string) []string {
	t.Helper()
	b, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}
