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

// TestOnWindowsANativeClaudeIsFoundByItsCreationWhileItRuns looks a native process up as remuda-core reports it, by its
// Windows pid, through the runner's OpenSSH and Git Bash, a HOME of the test's own holding its session file: Git
// Bash's ps and kill do not see a native process, so PowerShell tells its creation as a FILETIME. A file recording
// that creation is its own however long ago it began, one recording another's is not though it began since, one
// recording none is its own when it began since; once the process is killed, the file names nothing.
func TestOnWindowsANativeClaudeIsFoundByItsCreationWhileItRuns(t *testing.T) {
	wintest.Live(t)
	remote := wintest.Remote(t)
	home, folder := t.TempDir(), t.TempDir()
	cmd := exec.Command("ping", "-n", "120", "127.0.0.1")
	cmd.Dir = folder
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	pid := cmd.Process.Pid
	sessions := filepath.Join(home, ".claude", "sessions")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(startedAt time.Time, creation string) {
		body := fmt.Sprintf(`{"pid":%d,"sessionId":"4c9d6f0e-1111-4222-8333-944455556666","cwd":%q,"startedAt":%d`, pid, folder, startedAt.UnixMilli())
		if creation != "" {
			body += fmt.Sprintf(`,"procStartFt":%q`, creation)
		}
		if err := os.WriteFile(filepath.Join(sessions, strconv.Itoa(pid)+".json"), []byte(body+"}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	procs := []Running{{PID: pid, Agent: "claude", CWD: folder}}
	lookup := `HOME="$(cygpath -u ` + script.Quote(home) + `)"; export HOME; ` + sessionsByPIDScript(procs)
	found := func(what string) bool {
		t.Helper()
		out := wintest.Text(t, remote, lookup)
		got, _ := parseSessionsByPID(out, procs, rpath.For(home), machineClock{})
		if len(got) > 1 || len(got) == 1 && got[pid] != "4c9d6f0e-1111-4222-8333-944455556666" {
			t.Fatalf("%s: got %v from %q", what, got, out)
		}
		return len(got) == 1
	}

	write(time.Now().Add(-time.Hour), "") // left by an earlier process the pid belonged to
	if found("a file older than the process") {
		t.Fatal("a file older than the process was taken")
	}
	write(time.Now(), "")
	out := wintest.Text(t, remote, lookup)
	wintest.Record(t, "session-by-pid.txt", out)
	_, rest, _ := strings.Cut(out, "\037ft:")
	creation, _, _ := strings.Cut(rest, "\037")
	if _, ok := filetimeOf(creation); !ok || !found("a file begun since") {
		t.Fatalf("a live native process: no creation or no session in %q", out)
	}
	write(time.Now().Add(-time.Hour), creation)
	if !found("its own creation recorded") {
		t.Fatal("a file recording the process's creation was not taken")
	}
	earlier, _ := filetimeOf(creation)
	write(time.Now(), strconv.FormatInt(earlier-10_000_000, 10))
	if found("another's creation recorded") {
		t.Fatal("a file recording another process's creation was taken")
	}

	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	write(time.Now(), creation)
	out = wintest.Text(t, remote, lookup)
	if got, _ := parseSessionsByPID(out, procs, rpath.For(home), machineClock{}); len(got) != 0 || !strings.Contains(out, "\037-\037") {
		t.Fatalf("a process gone: got %v from %q", got, out)
	}
}
