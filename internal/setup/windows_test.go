package setup

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// herdrStandIn answers as herdr 0.9.5 does: its server running once Start-Process ran, claude's hooks missing.
const herdrStandIn = `case "$1 $2" in
"--version ") echo "herdr 0.9.5" ;;
"status server") if [ -f "$HOME/herdr-running" ]; then echo "status: running"; else echo "status: not running"; fi ;;
"integration status") echo "claude: not installed (C:/x)" ;;
esac`

// setupPATH is the line setup.sh begins with: what its herdr resolves through.
const setupPATH = `PATH="$HOME/.local/bin:$HOME/.kimi-code/bin:/opt/homebrew/bin:/usr/local/bin:$HOME/AppData/Local/Programs/Herdr/bin:$PATH"`

// windowsHome is a home on a machine that says it is Windows under Git Bash: stand-ins for uname, cygpath,
// powershell.exe (herdr's installer and Start-Process) and the agents first on PATH, each logging its call, so the
// scripts take their Windows way and reach nothing real.
func windowsHome(t *testing.T) (string, func() string) {
	t.Helper()
	dir := home(t)
	bin := filepath.Join(dir, ".local", "bin")
	for name, body := range map[string]string{
		"uname":   `case "$1" in -m) echo x86_64 ;; *) echo MINGW64_NT-10.0-26100 ;; esac`,
		"cygpath": `printf '%s\n' "$2"`,
		"powershell.exe": `case "$*" in
*install.ps1*) mkdir -p "$HOME/AppData/Local/Programs/Herdr/bin"; printf '#!/bin/sh\necho "herdr $*" >> "$HOME/calls.log"\n%s\n' '` + strings.ReplaceAll(herdrStandIn, "'", `'\''`) + `' > "$HOME/AppData/Local/Programs/Herdr/bin/herdr"; chmod +x "$HOME/AppData/Local/Programs/Herdr/bin/herdr" ;;
*Start-Process*) : > "$HOME/herdr-running" ;;
esac`,
		"claude": ``,
	} {
		write(t, filepath.Join(bin, name), "#!/bin/sh\necho \""+name+" $*\" >> \"$HOME/calls.log\"\n"+body+"\n", 0o755)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	for _, name := range []string{"uname", "cygpath", "powershell.exe"} {
		out, err := exec.Command("sh", "-c", `command -v "$1"`, "sh", name).Output()
		if got := strings.TrimSpace(string(out)); err != nil || got != filepath.Join(bin, name) {
			t.Fatalf("%s resolves to %q, not to its stand-in", name, got)
		}
	}
	return dir, func() string {
		b, _ := os.ReadFile(filepath.Join(dir, "calls.log"))
		return string(b)
	}
}

// herdrOutside says a herdr resolves, through setup.sh's PATH, to a binary outside the test's home: the person's own.
func herdrOutside(t *testing.T, dir string) bool {
	t.Helper()
	if !strings.Contains(setupScript, setupPATH) {
		t.Fatal("setup.sh no longer begins with the PATH this test checks")
	}
	out, _ := exec.Command("sh", "-c", setupPATH+"; command -v herdr").Output()
	found := strings.TrimSpace(string(out))
	return found != "" && !strings.HasPrefix(found, dir)
}

func TestSetupOnWindowsStartsHerdrsServerOutsideTheSession(t *testing.T) {
	dir, calls := windowsHome(t)
	write(t, filepath.Join(dir, ".local", "bin", "herdr"), "#!/bin/sh\necho \"herdr $*\" >> \"$HOME/calls.log\"\n"+herdrStandIn+"\n", 0o755)
	if herdrOutside(t, dir) {
		t.Fatal("a herdr outside this home would be reached")
	}
	var out strings.Builder
	if err := run("", nil, setupScript, nil, &out); err != nil {
		t.Fatalf("setup: %v\n%s\n%s", err, out.String(), calls())
	}
	log := calls()
	for _, want := range []string{"Start-Process -WindowStyle Hidden", "herdr integration install claude"} {
		if !strings.Contains(log, want) {
			t.Errorf("the stand-ins never saw %q:\n%s", want, log)
		}
	}
	if !strings.Contains(out.String(), "herdr server: running") || !strings.Contains(out.String(), "setup: done") {
		t.Errorf("setup did not say it was done:\n%s", out.String())
	}
	if strings.Contains(log, "install.ps1") || strings.Contains(log, "brew") || strings.Contains(log, "nohup") {
		t.Errorf("a herdr there was installed again, or a Unix way taken:\n%s", log)
	}
}

func TestSetupOnWindowsInstallsHerdrForWindows(t *testing.T) {
	dir, calls := windowsHome(t)
	if herdrOutside(t, dir) {
		t.Skip("a herdr of this machine's own is on setup.sh's PATH; the install's way is tried where none is (CI)")
	}
	var out strings.Builder
	if err := run("", nil, setupScript, nil, &out); err != nil {
		t.Fatalf("setup: %v\n%s\n%s", err, out.String(), calls())
	}
	log := calls()
	for _, want := range []string{"irm https://herdr.dev/install.ps1 | iex", "Start-Process -WindowStyle Hidden", "herdr integration install claude"} {
		if !strings.Contains(log, want) {
			t.Errorf("the stand-ins never saw %q:\n%s", want, log)
		}
	}
}
func TestTheInstallerPutsRemudaExeInPlaceOnWindows(t *testing.T) {
	dir, _ := windowsHome(t)
	served := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served[r.URL.Path] = true
		switch r.URL.Path {
		case "/checksums.txt":
			w.Write([]byte(sumOf(fakeBinary) + "  remuda-windows-amd64.exe\n" + sumOf("other") + "  remuda-linux-amd64\n"))
		case "/remuda-windows-amd64.exe":
			w.Write([]byte(fakeBinary))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	exe := filepath.Join(dir, ".local", "bin", "remuda.exe")
	write(t, exe, "#!/bin/sh\necho remuda v0.0.1\n", 0o755)
	out, err := install(t, srv.URL)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got, _ := os.ReadFile(exe); string(got) != fakeBinary {
		t.Fatalf("remuda.exe is not the release's:\n%s", out)
	}
	if exists(filepath.Join(dir, ".local", "bin", "remuda")) || !served["/remuda-windows-amd64.exe"] {
		t.Errorf("the Windows asset was not the one installed: %v\n%s", served, out)
	}
	if matches, _ := filepath.Glob(exe + ".old.*"); len(matches) != 0 {
		t.Errorf("the one before, not running, was left: %v", matches)
	}
}

func TestAWindowsHostWhoseSSHIsNotGitBashIsToldTheLine(t *testing.T) {
	bin := filepath.Join(home(t), "bin")
	write(t, filepath.Join(bin, "ssh"), "#!/bin/sh\necho \"'uname' is not recognized as an internal or external command,\"\necho 'operable program or batch file.'\nexit 1\n", 0o755)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	_, err := remoteSystem("pc")
	if err == nil || !strings.Contains(err.Error(), `-Name DefaultShell -Value "C:\Program Files\Git\bin\bash.exe"`) {
		t.Fatalf("the refusal does not say the line to run: %v", err)
	}
	write(t, filepath.Join(bin, "ssh"), "#!/bin/sh\necho MINGW64_NT-10.0-26100\n", 0o755)
	if system, err := remoteSystem("pc"); err != nil || system != "windows" {
		t.Fatalf("Git Bash's uname read as %q, %v", system, err)
	}
}
