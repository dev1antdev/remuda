package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/wintest"
)

// TestOnWindowsSetupOfThisPCRunsAsFromACockpit is `remuda setup -host pc -no-service` and `remuda mcp install -host
// pc` against the runner itself over its OpenSSH, Git Bash its shell: herdr there already, its server running, a
// stand-in claude whose calls are kept, so the registration's words are what Claude Code would be told.
func TestOnWindowsSetupOfThisPCRunsAsFromACockpit(t *testing.T) {
	wintest.Live(t)
	home, _ := os.UserHomeDir()
	bin := filepath.Join(home, ".local", "bin")
	calls := filepath.Join(t.TempDir(), "claude-calls.log")
	standIn := "#!/bin/sh\nprintf 'claude %s\\n' \"$*\" >> '" + filepath.ToSlash(calls) + "'\ncase \"$1 $2\" in \"mcp --help\") exit 0 ;; \"--version \") echo '2.1.330 (Claude Code)' ;; esac\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(standIn), 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(filepath.Join(bin, "claude"))

	var out strings.Builder
	if err := Run(Options{Host: os.Getenv("REMUDA_SSH_HOST"), NoService: true}, &out); err != nil {
		t.Fatalf("setup -host: %v\n%s", err, out.String())
	}
	wintest.Record(t, "setup.txt", out.String())
	for _, want := range []string{"herdr: herdr 0.9.", "herdr server: running", "setup: done"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("setup did not say %q:\n%s", want, out.String())
		}
	}

	out.Reset()
	if err := MCP(os.Getenv("REMUDA_SSH_HOST"), "install", &out); err != nil {
		t.Fatalf("mcp install -host: %v\n%s", err, out.String())
	}
	log, _ := os.ReadFile(calls)
	exe := filepath.Join(bin, "remuda.exe")
	if !strings.Contains(string(log), "claude mcp add --scope user remuda -- "+exe+" mcp") {
		t.Fatalf("Claude Code was not told remuda.exe by its Windows name (%s):\n%s\n%s", exe, log, out.String())
	}
}
