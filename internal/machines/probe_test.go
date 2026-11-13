package machines

import (
	"os"
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
