package awake

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// ShellHolder takes the hold with a script on the machine — caffeinate on macOS, systemd-inhibit on Linux — the
// inhibitor's pid kept in a file under remuda's state dir there, so a reconnect finds the one still running.
type ShellHolder struct {
	Dir  string // this instance's state dir: where the pidfile sits on the local machine
	Name string // this remuda's name in a remote's pidfile, so two cockpits holding one machine keep apart
}

var scriptTimeout = 20 * time.Second

func (h ShellHolder) Hold(host shell.Host) error {
	out, err := shell.Text(host, HoldScript(h.dirLine(host), h.file(host)), scriptTimeout)
	if err != nil {
		return err
	}
	return holdResult(out, host.Label())
}

func (h ShellHolder) Release(host shell.Host) error {
	_, err := shell.Text(host, ReleaseScript(h.dirLine(host), h.file(host)), scriptTimeout)
	return err
}

// dirLine sets $d, the state dir the pidfile lives in: this instance's own here, remuda's on the remote.
func (h ShellHolder) dirLine(host shell.Host) string {
	if host.Local() {
		return "d=" + script.Quote(h.Dir)
	}
	return script.StateDir("d")
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// file names the pidfile: one here, one per holding cockpit on a remote.
func (h ShellHolder) file(host shell.Host) string {
	if host.Local() {
		return "awake.pid"
	}
	name := unsafeName.ReplaceAllString(h.Name, "-")
	if name == "" {
		name = "remuda"
	}
	return "awake-" + name + ".pid"
}

// ours defines ours, the sh test that the pidfile's pid is a sleep inhibitor and not a process that took the number
// since, and stop, which ends it: ps and kill on a Mac or Linux; on Windows, where Git Bash's ps cannot say a native
// process's arguments, tasklist's word that the pid is a PowerShell, and taskkill.
const ours = `case "$(uname -s)" in MINGW*|MSYS*|CYGWIN*) win=1 ;; *) win= ;; esac
ours() { if [ -n "$win" ]; then tasklist //FI "PID eq $pid" //NH 2>/dev/null | grep -qi '^powershell'; else ps -o args= -p "$pid" 2>/dev/null | grep -q -E 'caffeinate|systemd-inhibit'; fi; }
stop() { if [ -n "$win" ]; then taskkill //PID "$pid" //F >/dev/null 2>&1; else kill "$pid" 2>/dev/null; fi; }`

// HoldScript keeps the inhibitor the pidfile names when it still runs, else starts one detached and writes its pid;
// it prints "held <pid>", "started <pid>", or "none <os>" where no inhibitor is known for the system.
func HoldScript(dirLine, file string) string {
	return `umask 077
` + dirLine + `
f="$d/` + file + `"
` + script.PATH + `
` + ours + `
if [ -f "$f" ]; then
  pid=$(cat "$f" 2>/dev/null)
  if [ -n "$pid" ] && ours; then printf 'held %s\n' "$pid"; exit 0; fi
fi
os=$(uname -s)
case "$os" in
  Darwin) tool=caffeinate ;;
  Linux) tool=systemd-inhibit ;;
  MINGW*|MSYS*|CYGWIN*) tool=powershell.exe ;;
  *) printf 'none %s\n' "$os"; exit 0 ;;
esac
if ! command -v "$tool" >/dev/null 2>&1; then printf 'none %s\n' "$os"; exit 0; fi
mkdir -p "$d" && chmod 700 "$d"
case "$tool" in
  caffeinate) nohup caffeinate -i </dev/null >/dev/null 2>&1 & pid=$! ;;
  systemd-inhibit) nohup systemd-inhibit --what=idle:sleep --who=remuda --why='agents working' sleep infinity </dev/null >/dev/null 2>&1 & pid=$! ;;
  *) pid=$(powershell.exe -NoProfile -NonInteractive -Command "(Start-Process -WindowStyle Hidden -PassThru -FilePath powershell.exe -ArgumentList '-NoProfile','-NonInteractive','-EncodedCommand','` + windowsHold + `').Id" </dev/null 2>/dev/null | tr -d '\r')
     [ -n "$pid" ] || { printf 'none %s\n' "$os"; exit 0; } ;;
esac
printf '%s\n' "$pid" > "$f"
printf 'started %s\n' "$pid"`
}

// ReleaseScript ends the inhibitor the pidfile names, when it still is one, and removes the file.
func ReleaseScript(dirLine, file string) string {
	return dirLine + `
f="$d/` + file + `"
` + ours + `
pid=$(cat "$f" 2>/dev/null)
if [ -n "$pid" ] && ours; then stop; printf 'released %s\n' "$pid"; else printf 'none\n'; fi
rm -f "$f"`
}

// holdResult reads the hold script's answer: an inhibitor running, or none known for the machine.
func holdResult(out, label string) error {
	word, rest, _ := strings.Cut(strings.TrimSpace(out), " ")
	switch word {
	case "held", "started":
		return nil
	case "none":
		if rest == "" {
			rest = "this system"
		}
		return fmt.Errorf("no sleep inhibitor on %s: neither caffeinate nor systemd-inhibit for %s", label, rest)
	}
	return fmt.Errorf("the hold on %s answered %q", label, strings.TrimSpace(out))
}
