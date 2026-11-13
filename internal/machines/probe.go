package machines

import (
	"fmt"
	"strings"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
)

// What a remote says about itself when its connection is made, in one round trip.

// probe is the machine as the connection first reads it.
type probe struct {
	home    string // as scripts read paths there: /home/me, or C:\Users\me on Windows
	os      string // darwin | linux | windows, or the system's own name lowercased
	herdr   string // herdr's binary, "" when none was found
	remuda  string // remuda's own binary on a Windows machine, the bridge to herdr there
	appData string // %APPDATA% on Windows, where herdr keeps its socket
}

// probeScript prints the machine's home, its system and, when asked to find it, herdr's binary, a key=value a line; on
// Windows, where Git Bash is the ssh shell, Windows' own spelling of the home, %APPDATA% and remuda's own binary too.
// herdr's stable alias comes first there: an update moves the release on PATH, the alias follows it. Every lookup is
// quiet and the script ends true, so a missing binary is an empty value, not a failed exec.
func probeScript(findHerdr bool) string {
	s := `printf 'home=%s\n' "$HOME"; o=$(uname -s 2>/dev/null); printf 'os=%s\n' "$o"
case "$o" in MINGW*|MSYS*|CYGWIN*)
  printf 'winhome=%s\n' "$(cygpath -w "$HOME" 2>/dev/null)"; printf 'appdata=%s\n' "$APPDATA"
  r="$HOME/.local/bin/remuda.exe"; [ -x "$r" ] && printf 'remuda=%s\n' "$r"
  a="$(cygpath -u "$LOCALAPPDATA" 2>/dev/null)/Programs/Herdr/bin/herdr.exe"; [ -x "$a" ] && printf 'herdr=%s\n' "$a" ;;
esac
`
	if !findHerdr {
		return s + "true\n"
	}
	return s + `h=$(command -v herdr 2>/dev/null) || for p in "$HOME/.local/bin/herdr" /opt/homebrew/bin/herdr /usr/local/bin/herdr /usr/bin/herdr; do [ -x "$p" ] && { h=$p; break; }; done
[ -n "$h" ] && printf 'herdr=%s\n' "$h"
true
`
}

// parseProbe reads the probe's answer; the first herdr named wins.
func parseProbe(out string) probe {
	var p probe
	var winHome string
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(strings.TrimRight(line, "\r"), "=")
		if !ok {
			continue
		}
		switch key {
		case "home":
			p.home = strings.TrimSpace(value)
		case "os":
			p.os = osOf(strings.TrimSpace(value))
		case "winhome":
			winHome = strings.TrimSpace(value)
		case "appdata":
			p.appData = strings.TrimSpace(value)
		case "remuda":
			p.remuda = strings.TrimSpace(value)
		case "herdr":
			if p.herdr == "" {
				p.herdr = strings.TrimSpace(value)
			}
		}
	}
	if p.os == "windows" && winHome != "" {
		p.home = rpath.For(winHome).Clean(winHome)
	}
	return p
}

// osOf names a system by what uname says: Git Bash on Windows says MINGW64_NT-10.0-26100.
func osOf(uname string) string {
	switch {
	case uname == "Darwin":
		return "darwin"
	case uname == "Linux":
		return "linux"
	case strings.HasPrefix(uname, "MINGW"), strings.HasPrefix(uname, "MSYS"), strings.HasPrefix(uname, "CYGWIN"):
		return "windows"
	}
	return strings.ToLower(uname)
}

// DefaultShellLine is what makes Git Bash the shell Windows' OpenSSH runs a command in, once, in an administrator PowerShell.
const DefaultShellLine = script.WindowsDefaultShell

// windowsShell says a probe failed because the machine is Windows whose ssh runs commands in cmd.exe or PowerShell:
// both answer that sh "is not recognized".
func windowsShell(answer string) bool {
	return strings.Contains(answer, "is not recognized as")
}

// errWindowsShell is the connection's word for a Windows machine whose ssh shell is not Git Bash.
func errWindowsShell(host string) error {
	return fmt.Errorf("%s is Windows and its ssh runs commands in cmd.exe or PowerShell; remuda needs Git Bash there. Install Git for Windows, then in an administrator PowerShell on %s run: %s", host, host, DefaultShellLine)
}

// errNoRemuda is the connection's word for a Windows machine without remuda.exe, herdr's only door there.
func errNoRemuda(host string) error {
	return fmt.Errorf("remuda.exe is not on %s: it is how this cockpit reaches herdr's named pipe there. Run `remuda setup -host %s` from here", host, host)
}

// windowsSocket is herdr's default socket on a Windows machine: %APPDATA%\herdr\herdr.sock, a named pipe of that name.
func windowsSocket(appData string) string {
	return rpath.Style{Windows: true}.Join(appData, "herdr", "herdr.sock")
}
