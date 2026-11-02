package updates

import (
	"os/exec"
	"strings"
	"testing"
)

// TestRestartScriptSaysWhenNoServiceManagerIsThere: a Linux without systemd (a container, WSL) has neither launchctl nor
// systemctl, and the new binary installed there keeps the old one running until the person restarts it — so the job
// must say so instead of ending in silence.
func TestRestartScriptSaysWhenNoServiceManagerIsThere(t *testing.T) {
	empty := t.TempDir()
	cmd := exec.Command("/bin/sh", "-c", RestartScript)
	cmd.Env = []string{"PATH=" + empty, "HOME=" + empty}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the script failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "start the new remuda yourself") {
		t.Fatalf("nothing said that remuda was not restarted: %q", out)
	}
}
