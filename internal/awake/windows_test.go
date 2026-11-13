//go:build windows

package awake

import (
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/wintest"
)

// TestOnWindowsTheHoldOutlivesItsSessionAndIsLetGo takes the hold over the runner's OpenSSH as a cockpit takes it on a
// Windows machine: a hidden PowerShell started outside the session, found again by the next look, ended by the release.
func TestOnWindowsTheHoldOutlivesItsSessionAndIsLetGo(t *testing.T) {
	wintest.Live(t)
	remote := wintest.Remote(t)
	dirLine, file := script.StateDir("d"), "awake-windows-check.pid"
	first := wintest.Text(t, remote, HoldScript(dirLine, file))
	wintest.Record(t, "awake-hold.txt", first)
	word, pid, _ := strings.Cut(strings.TrimSpace(first), " ")
	if word != "started" || pid == "" {
		t.Fatalf("the hold answered %q", first)
	}
	if again := strings.TrimSpace(wintest.Text(t, remote, HoldScript(dirLine, file))); again != "held "+pid {
		t.Fatalf("the next look, in a session of its own, answered %q: the hold did not outlive the first", again)
	}
	if released := strings.TrimSpace(wintest.Text(t, remote, ReleaseScript(dirLine, file))); released != "released "+pid {
		t.Fatalf("the release answered %q", released)
	}
	if after := wintest.Text(t, remote, `tasklist //FI "PID eq `+pid+`" //NH`); strings.Contains(strings.ToLower(after), "powershell") {
		t.Fatalf("the hold still runs after its release:\n%s", after)
	}
}
