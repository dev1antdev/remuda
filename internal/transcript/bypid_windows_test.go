//go:build windows

package transcript

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/wintest"
)

// TestOnWindowsANativeClaudeIsFoundByItsFolderWhileItRuns looks a native process up as remuda-core reports it, by its
// Windows pid, through the runner's OpenSSH and Git Bash, a HOME of the test's own holding its session file: Git
// Bash's ps and kill do not see a native process, so PowerShell tells when it was created, and a file of its folder
// started since is its own, one started before it a leftover; once it is killed, the file names nothing.
func TestOnWindowsANativeClaudeIsFoundByItsFolderWhileItRuns(t *testing.T) {
	wintest.Live(t)
	remote := wintest.Remote(t)
	home, folder := t.TempDir(), t.TempDir()
	cmd := exec.Command("ping", "-n", "120", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	pid := cmd.Process.Pid
	sessions := filepath.Join(home, ".claude", "sessions")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(startedAt time.Time) {
		body := fmt.Sprintf(`{"pid":%d,"sessionId":"4c9d6f0e-1111-4222-8333-944455556666","cwd":%q,"startedAt":%d,"procStartFt":"134080000000000000"}`, pid, folder, startedAt.UnixMilli())
		if err := os.WriteFile(filepath.Join(sessions, strconv.Itoa(pid)+".json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	procs := []Running{{PID: pid, Agent: "claude", CWD: folder}}
	lookup := `HOME="$(cygpath -u ` + script.Quote(home) + `)"; export HOME; ` + sessionsByPIDScript(procs)

	write(time.Now().Add(-time.Hour)) // left by an earlier process the pid belonged to
	out := wintest.Text(t, remote, lookup)
	if got := parseSessionsByPID(out, procs, rpath.For(home)); len(got) != 0 {
		t.Fatalf("a file older than the process: got %v from %q", got, out)
	}

	write(time.Now())
	out = wintest.Text(t, remote, lookup)
	wintest.Record(t, "session-by-pid.txt", out)
	if got := parseSessionsByPID(out, procs, rpath.For(home)); got[pid] != "4c9d6f0e-1111-4222-8333-944455556666" || !strings.Contains(out, "\037ms:") {
		t.Fatalf("a live native process: got %v from %q", got, out)
	}

	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	out = wintest.Text(t, remote, lookup)
	if got := parseSessionsByPID(out, procs, rpath.For(home)); len(got) != 0 || !strings.Contains(out, "\037-\037") {
		t.Fatalf("a process gone: got %v from %q", got, out)
	}
}
