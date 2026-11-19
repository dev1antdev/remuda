package updates

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dev1antdev/remuda/internal/health"
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

func TestAHerdrUpdateThatFailsFailsItsJob(t *testing.T) {
	const herdr = `case "$1" in update) [ -e "$HOME/fails" ] && exit 7 ;; --version) echo 0.9.9 ;; esac; exit 0`
	const brew = `[ "$1" = upgrade ] && [ -e "$HOME/fails" ] && exit 7; exit 0`
	for name, report := range map[string]*health.Report{
		"its own updater": {Herdr: "0.9.3", HerdrVia: "native"},
		"Homebrew":        {Herdr: "0.9.3", HerdrVia: "brew"},
	} {
		update, err := Plan("herdr", report)
		if err != nil {
			t.Fatal(err)
		}
		home := standIns(t, map[string]string{"herdr": herdr, "brew": brew})
		if err := runIn(home, update.Command); err != nil {
			t.Errorf("%s: an update that went through: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(home, "fails"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		var exit *exec.ExitError
		if err := runIn(home, update.Command); !errors.As(err, &exit) || exit.ExitCode() != 7 {
			t.Errorf("%s: a failed update ended with %v", name, err)
		}
	}
}

func TestAFailedHerdrUpdateLeavesTheServerRunning(t *testing.T) {
	var said []string
	then := (&Service{}).herdrThen(nil) // no machine: going on to the versions or the restart would panic
	if err := then(func(line string) { said = append(said, line) }, errors.New("exit status 7")); err != nil {
		t.Fatal(err)
	}
	if len(said) != 1 {
		t.Errorf("said %q", said)
	}
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
