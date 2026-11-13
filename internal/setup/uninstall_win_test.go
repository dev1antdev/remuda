package setup

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// windowsMachine lays a home out as setup left a Windows PC, with stand-ins for uname, cygpath, herdr, powershell.exe
// and pgrep resolving into it through the uninstall script's own PATH; every call logged, nothing real reached.
func windowsMachine(t *testing.T) (string, func() string) {
	t.Helper()
	dir := home(t)
	appData, localAppData := filepath.Join(dir, "AppData", "Roaming"), filepath.Join(dir, "AppData", "Local")
	t.Setenv("APPDATA", appData)
	t.Setenv("LOCALAPPDATA", localAppData)
	bin := filepath.Join(dir, ".local", "bin")
	for name, body := range map[string]string{
		"uname":          `case "$1" in -m) echo x86_64 ;; -n) echo PC ;; *) echo MINGW64_NT-10.0-26100 ;; esac`,
		"cygpath":        `printf '%s\n' "$2"`,
		"powershell.exe": ``,
		"pgrep":          `exit 1`,
		"herdr": `case "$1 $2" in
"status server") if [ -f "$HOME/herdr-stopped" ]; then echo "status: not running"; else echo "status: running"; fi ;;
"integration status") printf 'claude: current (v10) (C:/x)\npi: not installed\n' ;;
"server stop") : > "$HOME/herdr-stopped" ;;
esac`,
	} {
		write(t, filepath.Join(bin, name), "#!/bin/sh\necho \""+name+" $*\" >> \"$HOME/calls.log\"\n"+body+"\n", 0o755)
	}
	const pathLine = `PATH="$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:$PATH"`
	if !strings.Contains(uninstallScript, pathLine) {
		t.Fatal("the uninstall script no longer puts ~/.local/bin first on PATH: the stand-ins would not be found")
	}
	for _, name := range []string{"uname", "cygpath", "powershell.exe", "pgrep", "herdr"} {
		out, err := exec.Command("sh", "-c", pathLine+`; command -v "$1"`, "sh", name).Output()
		if got := strings.TrimSpace(string(out)); err != nil || got != filepath.Join(bin, name) {
			t.Fatalf("%s resolves to %q, not to its stand-in", name, got)
		}
	}
	write(t, filepath.Join(bin, "remuda.exe"), fakeBinary, 0o755)
	write(t, filepath.Join(bin, "remuda.exe.old.123"), "the one before", 0o755)
	write(t, filepath.Join(appData, "remuda", "agent-key"), "rk_x", 0o600) // gitleaks:allow (a stand-in)
	write(t, filepath.Join(appData, "remuda", "claude-limits.json"), "{}", 0o600)
	write(t, filepath.Join(dir, ".herdr", "packages", "standalone", "releases", "0.9.5", "herdr.exe"), "", 0o755)
	write(t, filepath.Join(dir, ".herdr", "worktrees", "app", "feature", "main.go"), "package main", 0o644)
	write(t, filepath.Join(localAppData, "Programs", "Herdr", "bin", "herdr.exe"), "", 0o755)
	write(t, filepath.Join(appData, "herdr", "herdr-server.log"), "log", 0o644)
	return dir, func() string {
		b, _ := os.ReadFile(filepath.Join(dir, "calls.log"))
		return string(b)
	}
}

func TestUninstallOnWindowsTakesWhatSetupPutThereAndLeavesTheWork(t *testing.T) {
	dir, calls := windowsMachine(t)
	removal := Removal{Data: true, Herdr: true}
	var plan strings.Builder
	some, err := removal.Plan(&plan)
	if err != nil || !some {
		t.Fatalf("plan: %v, %v\n%s", some, err, plan.String())
	}
	for _, want := range []string{"remove  ~/.local/bin/remuda.exe", "your data in ~/AppData/Roaming/remuda", "herdr for Windows", "its hooks for: claude", "~/.herdr/worktrees stays"} {
		if !strings.Contains(plan.String(), want) {
			t.Errorf("the plan does not say %q:\n%s", want, plan.String())
		}
	}
	if strings.Contains(plan.String(), "the service") || strings.Contains(calls(), "server stop") {
		t.Fatalf("the plan names a service or acted:\n%s\n%s", plan.String(), calls())
	}

	var out strings.Builder
	if err := removal.Remove(&out); err != nil {
		t.Fatalf("run: %v\n%s\n%s", err, out.String(), calls())
	}
	for _, gone := range []string{
		filepath.Join(dir, ".local", "bin", "remuda.exe"), filepath.Join(dir, ".local", "bin", "remuda.exe.old.123"),
		filepath.Join(dir, "AppData", "Roaming", "remuda"), filepath.Join(dir, ".herdr", "packages"),
		filepath.Join(dir, "AppData", "Local", "Programs", "Herdr"), filepath.Join(dir, "AppData", "Roaming", "herdr"),
	} {
		if exists(gone) {
			t.Errorf("%s is still there:\n%s", gone, out.String())
		}
	}
	if !exists(filepath.Join(dir, ".herdr", "worktrees", "app", "feature", "main.go")) {
		t.Fatalf("the worktrees under ~/.herdr went with herdr:\n%s", out.String())
	}
	log := calls()
	for _, want := range []string{"herdr integration uninstall claude", "herdr server stop", `[Environment]::SetEnvironmentVariable("Path", $n, "User")`} {
		if !strings.Contains(log, want) {
			t.Errorf("the stand-ins never saw %q:\n%s", want, log)
		}
	}
	for _, unix := range []string{"launchctl", "systemctl", "brew"} {
		if strings.Contains(log, unix) {
			t.Errorf("%s was called on Windows:\n%s", unix, log)
		}
	}
}
