package machines

import (
	"fmt"
	"strings"

	"github.com/dev1antdev/remuda/internal/coreapi"
	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
)

// What a remote says about itself when its connection is made, in one round trip.

// probe is the machine as the connection first reads it.
type probe struct {
	home    string // as scripts read paths there: /home/me, or C:\Users\me on Windows
	os      string // darwin | linux | windows, or the system's own name lowercased
	appData string // %APPDATA% on Windows, where herdr keeps its socket
	state   string // the folder remuda keeps there
	core    string // remuda-core's binary, "" when none was found
}

// coreLookup prints remuda-core's binary as `core=`, where the vitals and the Updates row look for it.
var coreLookup = script.CoreBin("c") + `
[ -n "$c" ] && printf 'core=%s\n' "$c"
true
`

// probeScript prints the machine's home and its system, a key=value a line; on Windows, where Git Bash is the ssh shell,
// Windows' own spelling of the home and %APPDATA% too, where herdr keeps its named pipe. Every lookup is quiet and the
// script ends true, so a missing value is an empty line, not a failed exec.
func probeScript() string {
	return script.StateDir("d") + `
printf 'state=%s\n' "${REMUDA_STATE_DIR:-$d}"
printf 'home=%s\n' "$HOME"; o=$(uname -s 2>/dev/null); printf 'os=%s\n' "$o"
case "$o" in MINGW*|MSYS*|CYGWIN*)
  printf 'winhome=%s\n' "$(cygpath -w "$HOME" 2>/dev/null)"; printf 'appdata=%s\n' "$APPDATA" ;;
esac
true
`
}

// parseProbe reads the probe's answer.
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
		case "state":
			p.state = strings.TrimSpace(value)
		case "core":
			p.core = strings.TrimSpace(value)
		}
	}
	if p.os == "windows" && winHome != "" {
		p.home = rpath.For(winHome).Clean(winHome)
		p.state = rpath.For(winHome).Clean(p.state)
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

// NoCoreError is Ready's answer on a remote connected without a remuda-core it can follow, its master still up: none
// installed there (errNoCore), or none answering.
type NoCoreError struct{ Reason error }

func (e *NoCoreError) Error() string { return e.Reason.Error() }
func (e *NoCoreError) Unwrap() error { return e.Reason }

// errNoCore is the connection's word for a machine without remuda-core, to a remuda that speaks it (-core).
func errNoCore(host string) error {
	return fmt.Errorf("remuda-core is not installed on %s: Install remuda-core in its Updates, or run `remuda core -host %s install` from here", host, host)
}

// herdrSocket is the herdr remuda reached on a machine before the move, as MovedHerdr names it: the machine's
// remote_socket, else herdr's default — under %APPDATA% on Windows, wherever the profile keeps it, under the home
// elsewhere. It is named here, not left to herdr, so a HERDR_SESSION the ssh environment exports cannot point the check
// at another session's server.
func herdrSocket(cfg Config, found probe) string {
	switch {
	case cfg.RemoteSocket != "":
		return cfg.RemoteSocket
	case found.os == "windows":
		return windowsSocket(found.appData)
	}
	return coreapi.DefaultSocket(found.home)
}

// windowsSocket is herdr's default socket on a Windows machine: %APPDATA%\herdr\herdr.sock, a named pipe of that name.
func windowsSocket(appData string) string {
	return rpath.Style{Windows: true}.Join(appData, "herdr", "herdr.sock")
}
