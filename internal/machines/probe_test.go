package machines

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/state"
)

// TestAWindowsProbeIsReadInWindowsSpelling reads the probe's answer as GitHub's Windows Server 2025 runner gave it
// through its OpenSSH, Git Bash its shell, herdr 0.9.5 and remuda.exe installed (CI's windows job records it).
func TestAWindowsProbeIsReadInWindowsSpelling(t *testing.T) {
	recorded, err := os.ReadFile("testdata/windows-probe.txt")
	if err != nil {
		t.Fatal(err)
	}
	found := parseProbe(string(recorded))
	if found.os != "windows" || found.home != `C:\Users\runneradmin` {
		t.Fatalf("read %+v", found)
	}
}

func TestAMacOrLinuxProbeIsReadAsBefore(t *testing.T) {
	for answer, system := range map[string]string{
		"home=/Users/me\nos=Darwin\ncore=/Users/me/.local/bin/remuda-core\n": "darwin",
		"home=/home/me\nos=Linux\ncore=/home/me/.local/bin/remuda-core\n":    "linux",
		"home=/home/me\nos=FreeBSD\n":                                        "freebsd",
	} {
		found := parseProbe(answer)
		if found.os != system || !strings.HasPrefix(found.home, "/") {
			t.Errorf("%q read as %+v", answer, found)
		}
	}
}

func TestTheProbeNamesTheFolderRemudaKeepsThere(t *testing.T) {
	if script := probeScript(); !strings.Contains(script, `XDG_CONFIG_HOME`) || !strings.Contains(script, `state=`) {
		t.Error("the probe does not reckon the state dir as the machine's remuda does")
	}
	if !strings.Contains(probeScript(), `${REMUDA_STATE_DIR:-$d}`) {
		t.Error("a state dir set for the machine's remuda is not looked at")
	}
	if found := parseProbe("home=/home/me\nos=Linux\nstate=/home/me/dotfiles/remuda\n"); found.state != "/home/me/dotfiles/remuda" {
		t.Errorf("read %+v", found)
	}
	windows := parseProbe("state=/c/Users/me/AppData/Roaming/remuda\nhome=/c/Users/me\nos=MINGW64_NT-10.0\nwinhome=C:\\\\Users\\\\me\n")
	if windows.state != `C:\Users\me\AppData\Roaming\remuda` {
		t.Errorf("on Windows: %q", windows.state)
	}
}

func TestAWindowsShellThatIsNotGitBashIsNamed(t *testing.T) {
	for _, answer := range []string{
		"'sh' is not recognized as an internal or external command,\r\noperable program or batch file.",
		"sh: The term 'sh' is not recognized as a name of a cmdlet, function, script file, or executable program.",
	} {
		if !windowsShell(answer) {
			t.Errorf("not seen as Windows' own shell: %q", answer)
		}
	}
	if windowsShell("sh: 1: herdr: not found") {
		t.Error("a POSIX shell's own complaint is no Windows shell")
	}
	if err := errWindowsShell("pc"); !strings.Contains(err.Error(), DefaultShellLine) {
		t.Fatalf("the error says the line to run: %v", err)
	}
}

func TestTheProbeLooksForNoHerdrAnyMore(t *testing.T) {
	if strings.Contains(probeScript(), "herdr") {
		t.Fatal("the probe still looks for herdr: herdr is not this remuda's to find (Core 26i)")
	}
}

// TestTheEffectivePathIsTheOneLookedAt: what a check is made on is the file a look then opens, a space at its end gone.
func TestTheEffectivePathIsTheOneLookedAt(t *testing.T) {
	mc := &Machine{home: "/home/me", os: "linux"}
	for raw, want := range map[string]string{
		"/w/app/x ":       "/w/app/x",
		" /w/app/../y":    "/w/y",
		"~/notes":         "/home/me/notes",
		"src/main.go":     "/w/app/src/main.go",
		"../../etc/hosts": "/etc/hosts",
		"   ":             "",
	} {
		if got := mc.EffectivePath("/w/app", raw); got != want {
			t.Errorf("%q: %q, want %q", raw, got, want)
		}
	}
}

// TestAStateDirIsKnownByItsLockWhereverItIs: here and through the script a remote machine runs.
func TestAStateDirIsKnownByItsLockWhereverItIs(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(root, "app", "run state")
	if err := os.MkdirAll(filepath.Join(stateDir, "tailscale"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, state.LockFile), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{
		stateDir:                            true,
		filepath.Join(stateDir, "tasks.db"): true,
		filepath.Join(stateDir, "tailscale", "node"): true,
		filepath.Join(root, "app", "main.go"):        false,
		root:                                         false,
	} {
		if got := (&Machine{local: true}).InStateDir(path); got != want {
			t.Errorf("here, %s: %v, want %v", path, got, want)
		}
		out, err := exec.Command("sh", "-c", stateDirScript(path)).Output()
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(string(out)) == "yes"; got != want {
			t.Errorf("by the script, %s: %v, want %v", path, got, want)
		}
	}
}

func TestACheckoutHoldingAStateDirIsKnown(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, "dotfiles", "run")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	mc := &Machine{local: true}
	if mc.HoldsStateDir(root) {
		t.Error("no state dir here yet")
	}
	if err := os.WriteFile(filepath.Join(stateDir, state.LockFile), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if !mc.HoldsStateDir(root) {
		t.Error("the state dir under the checkout")
	}
	if !mc.HoldsStateDir(filepath.Join(root, "gone")) {
		t.Error("a folder that could not be looked through")
	}
	if err := os.MkdirAll(filepath.Join(root, "web", "node_modules", ".cache"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(stateDir, state.LockFile), filepath.Join(root, "web", "node_modules", ".cache", state.LockFile)); err != nil {
		t.Fatal(err)
	}
	if !mc.HoldsStateDir(root) {
		t.Error("a state dir under node_modules")
	}
}

// TestLookingAtAPathTakesItAsGiven: a name ending in a space is that name, and never the file without the space.
func TestLookingAtAPathTakesItAsGiven(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "secret")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	mc := &Machine{local: true}
	if info := mc.LookAt(file + " "); info.Kind != "missing" || info.Path != file+" " {
		t.Errorf("with a space: %+v", info)
	}
	if info := mc.LookAt(file); info.Kind == "missing" {
		t.Errorf("the file: %+v", info)
	}
}
