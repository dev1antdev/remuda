package machines

import (
	"github.com/dev1antdev/remuda/internal/state"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAWindowsProbeIsReadInWindowsSpelling reads the probe's answer as GitHub's Windows Server 2025 runner gave it
// through its OpenSSH, Git Bash its shell, herdr 0.9.5 and remuda.exe installed (CI's windows job records it).
func TestAWindowsProbeIsReadInWindowsSpelling(t *testing.T) {
	recorded, err := os.ReadFile("testdata/windows-probe.txt")
	if err != nil {
		t.Fatal(err)
	}
	found := parseProbe(string(recorded))
	if found.os != "windows" || found.home != `C:\Users\runneradmin` || found.appData != `C:\Users\runneradmin\AppData\Roaming` {
		t.Fatalf("read %+v", found)
	}
	if found.herdr != "/c/Users/runneradmin/AppData/Local/Programs/Herdr/bin/herdr.exe" {
		t.Fatalf("herdr's stable alias goes first, the release on PATH moves with an update: %q", found.herdr)
	}
	if got := windowsSocket(found.appData); got != `C:\Users\runneradmin\AppData\Roaming\herdr\herdr.sock` {
		t.Fatalf("the socket herdr names, and its pipe after it: %q", got)
	}
	if got := BridgeCommand(found.remuda, windowsSocket(found.appData)); !strings.HasPrefix(got, "sh -c ") || !strings.Contains(got, `/c/Users/runneradmin/.local/bin/remuda.exe'\'' bridge '\''C:\Users\runneradmin\AppData\Roaming\herdr\herdr.sock`) {
		t.Fatalf("the bridge's exec: %s", got)
	}
}

func TestAMacOrLinuxProbeIsReadAsBefore(t *testing.T) {
	for answer, system := range map[string]string{
		"home=/Users/me\nos=Darwin\nherdr=/opt/homebrew/bin/herdr\n": "darwin",
		"home=/home/me\nos=Linux\nherdr=/home/me/.local/bin/herdr\n": "linux",
		"home=/home/me\nos=FreeBSD\n":                                "freebsd",
	} {
		found := parseProbe(answer)
		if found.os != system || !strings.HasPrefix(found.home, "/") || found.remuda != "" {
			t.Errorf("%q read as %+v", answer, found)
		}
	}
}

func TestTheProbeNamesTheFolderRemudaKeepsThere(t *testing.T) {
	if script := probeScript(false); !strings.Contains(script, `XDG_CONFIG_HOME`) || !strings.Contains(script, `state=`) {
		t.Error("the probe does not reckon the state dir as the machine's remuda does")
	}
	if !strings.Contains(probeScript(false), `${REMUDA_STATE_DIR:-$d}`) {
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

func TestTheProbeLooksForHerdrOnlyWhenNotConfigured(t *testing.T) {
	if strings.Contains(probeScript(false), "command -v herdr") || !strings.Contains(probeScript(true), "command -v herdr") {
		t.Fatal("a configured herdr_bin is not looked for")
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
}

// TestClosedFindsDoNotCrowdOutOpenOnes: the search's cap counts what the caller may read.
func TestClosedFindsDoNotCrowdOutOpenOnes(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "z-open"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, dir, "same.txt"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	info, err := (&Machine{local: true}).ResolvePathAllowed(root, "same.txt", func(p string) bool { return strings.Contains(p, "/z-open/") })
	if err != nil || info.Path != filepath.Join(root, "z-open", "same.txt") {
		t.Fatalf("%+v, %v", info, err)
	}
}

// TestASearchLooksOnlyAtWhatMayBeRead: a file the caller may not read is neither looked at nor named.
func TestASearchLooksOnlyAtWhatMayBeRead(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"closed/prefs.json", "open/notes.txt", "closed/two.txt", "open/two.txt"} {
		full := filepath.Join(root, file)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mc := &Machine{local: true}
	allow := func(p string) bool { return !strings.Contains(p, "/closed/") }
	if info, err := mc.ResolvePathAllowed(root, "prefs.json", allow); err != nil || info.Kind != "missing" || len(info.Candidates) != 0 {
		t.Errorf("a closed file: %+v, %v", info, err)
	}
	if info, err := mc.ResolvePathAllowed(root, "two.txt", allow); err != nil || info.Path != filepath.Join(root, "open", "two.txt") {
		t.Errorf("one of two left: %+v, %v", info, err)
	}
	if info, err := mc.ResolvePath(root, "two.txt"); err != nil || len(info.Candidates) != 2 {
		t.Errorf("the owner's search: %+v, %v", info, err)
	}
}
