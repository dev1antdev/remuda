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
	stub := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + script.Quote(calls) + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return calls
}

func recorded(t *testing.T, calls string) []string {
	t.Helper()
	b, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

// TestForwardSocketCancelsTheForwardTheMasterKeeps: a master already holding the forward answers a second ask with
// "found existing forwarding" and binds nothing, so the listener's file, unlinked here, would be gone for good —
// herdr started by connect, or a remuda restarted over a living master, fell back to one ssh exec per request.
func TestForwardSocketCancelsTheForwardTheMasterKeeps(t *testing.T) {
	calls := fakeSSH(t)
	c := &Conn{Host: "gog.remote", ctl: "/tmp/remuda-ssh-test.ctl"}
	local, err := c.ForwardSocket("/home/me/.config/herdr/herdr.sock")
	if err != nil {
		t.Fatal(err)
	}
	spec := local + ":/home/me/.config/herdr/herdr.sock"
	lines := recorded(t, calls)
	if len(lines) != 2 {
		t.Fatalf("ssh was asked %d times, want the cancel then the forward:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	if !strings.HasSuffix(lines[0], "-O cancel -L "+spec+" gog.remote") || !strings.Contains(lines[0], "-S /tmp/remuda-ssh-test.ctl") {
		t.Errorf("the first ask is not the cancel on the master: %s", lines[0])
	}
	if !strings.HasSuffix(lines[1], "-O forward -L "+spec+" gog.remote") {
		t.Errorf("the second ask is not the forward: %s", lines[1])
	}
}
