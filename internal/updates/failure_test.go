package updates

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

// standIns puts scripts first on the PATH the update scripts set, in a home of the test's own.
func standIns(t *testing.T, scripts map[string]string) string {
	t.Helper()
	home := t.TempDir()
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func runIn(home, command string) error {
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
	return cmd.Run()
}

func TestAReleaseThatDoesNotRunIsNotInstalled(t *testing.T) {
	home := standIns(t, map[string]string{
		"curl":      `while [ $# -gt 0 ]; do [ "$1" = -o ] && out=$2; shift; done; printf '#!/bin/sh\nexit 3\n' > "$out"`,
		"launchctl": `: > "$HOME/restarted"`,
		"systemctl": `: > "$HOME/restarted"`,
		"remuda":    `echo old`,
	})
	if err := runIn(home, ReleaseScript("https://example.invalid/remuda", "", "remuda-linux-amd64", false)); err == nil {
		t.Error("a binary that does not run counted as installed")
	}
	if _, err := os.Stat(filepath.Join(home, "restarted")); err == nil {
		t.Error("the service was restarted on the old binary")
	}
	if body, _ := os.ReadFile(filepath.Join(home, ".local", "bin", "remuda")); string(body) != "#!/bin/sh\necho old\n" {
		t.Errorf("the old binary was replaced: %q", body)
	}
	if left, _ := filepath.Glob(filepath.Join(home, ".local", "bin", "remuda.new*")); len(left) != 0 {
		t.Errorf("the download stayed behind: %v", left)
	}
}

func TestAReleaseStoppedPartWayLeavesNoDownloadBehind(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no signals to a shell here")
	}
	for _, name := range []string{"sh", "dash", "bash"} {
		shell, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		home := standIns(t, nil)
		cmd := exec.Command(shell, "-c", stagingScript("remuda.new.$$")+`: > "$HOME/.local/bin/remuda.new.$$"; echo started; sleep 30 & wait`)
		cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
		out, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		bufio.NewReader(out).ReadString('\n')
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		cmd.Wait()
		if left, _ := filepath.Glob(filepath.Join(home, ".local", "bin", "remuda.new*")); len(left) != 0 {
			t.Errorf("%s: the download stayed behind: %v", name, left)
		}
	}
}
