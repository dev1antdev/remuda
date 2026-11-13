//go:build windows

package updates

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/wintest"
)

// TestOnWindowsTheReleaseTakesThePlaceOfARunningRemudaExe runs a release's install step over the runner's OpenSSH
// while remuda.exe runs there — a bridge to herdr holding it, as the cockpit's event stream does — and the new one
// is in place after.
func TestOnWindowsTheReleaseTakesThePlaceOfARunningRemudaExe(t *testing.T) {
	wintest.Live(t)
	exe := wintest.RemudaExe(t)
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(exe), "remuda.new.exe"), data, 0o755); err != nil {
		t.Fatal(err)
	}
	running := exec.Command(exe, "bridge", herdr.OwnSocket())
	stdin, err := running.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := running.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { stdin.Close(); running.Process.Kill(); running.Wait() }()
	time.Sleep(time.Second)

	out, err := wintest.Remote(t).Run(windowsInstall, time.Minute)
	if err != nil || !strings.Contains(string(out), "no service on Windows") {
		t.Fatalf("the install beside a running remuda.exe: %v\n%s", err, out)
	}
	if version, err := exec.Command(exe, "-version").Output(); err != nil || !strings.HasPrefix(string(version), "remuda ") {
		t.Fatalf("remuda.exe in place after the install: %v %s", err, version)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "remuda.new.exe")); !os.IsNotExist(err) {
		t.Fatalf("the download was left behind: %v", err)
	}
}
