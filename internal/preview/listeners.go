// Package preview opens what an agent serves — a dev server on GOG's :3000 — on the cockpit, for the person's own
// browser: the machine's listening ports found and matched to its workspaces (this file), and on Open a listener of
// the cockpit's own, the same port on its tailnet name, that forwards over the machine's ssh master to the server
// (service.go, proxy.go). A port on its own origin keeps a dev server's absolute asset paths working, and keeps the
// app's scripts off remuda's origin.
package preview

import (
	"net"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// NotOnWindows is why a Windows machine offers no previews yet: Windows says which process listens on a port, but not
// in which folder it runs, and a preview is matched to its workspace by that folder.
const NotOnWindows = "previews are not offered for a Windows machine yet: Windows does not say which folder a listening process runs in, so no port can be matched to its workspace"

// FindTimeout is how long the look over a machine's ports may take.
var FindTimeout = 20 * time.Second

// Find is the machine's listeners worth a preview, skip left out.
func Find(host shell.Shell, skip map[int]bool) ([]Listener, error) {
	out, err := shell.Text(host, Script(), FindTimeout)
	if err != nil && strings.TrimSpace(out) == "" {
		return nil, err
	}
	return Parse(out, skip), nil
}

// Listener is one TCP port a process on the machine listens on, with where that process runs.
type Listener struct {
	Port      int    `json:"port"`
	Address   string `json:"address"` // what it is bound to: 127.0.0.1, ::1, * …
	Dial      string `json:"-"`       // the host the machine reaches it at
	PID       int    `json:"pid"`
	Command   string `json:"command"` // the process's name: node, python3 …
	Label     string `json:"label,omitempty"`
	Dir       string `json:"dir,omitempty"`       // its working folder, else its parent's
	Workspace string `json:"workspace,omitempty"` // the workspace whose folder holds it
}

// Script prints a line per listening TCP port above 1023: port, address, pid, command, the process's working folder,
// its parent's, and its arguments; ss and /proc on Linux, lsof and ps elsewhere. Only the person's own processes
// answer with a folder, so another user's server is never named.
func Script() string {
	return script.PATH + `
emit() {
  [ "$1" -ge 1024 ] 2>/dev/null || return 0
  if [ -r "/proc/$3/cwd" ] || [ -L "/proc/$3/cwd" ]; then
    cwd=$(readlink "/proc/$3/cwd" 2>/dev/null)
    ppid=$(awk '{print $4}' "/proc/$3/stat" 2>/dev/null)
    pcwd=$(readlink "/proc/$ppid/cwd" 2>/dev/null)
  else
    cwd=$(lsof -a -p "$3" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -1)
    ppid=$(ps -o ppid= -p "$3" 2>/dev/null | tr -d ' ')
    pcwd=$(lsof -a -p "$ppid" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -1)
  fi
  args=$(ps -o args= -p "$3" 2>/dev/null | cut -c1-240)
  printf '%s\037%s\037%s\037%s\037%s\037%s\037%s\n' "$1" "$2" "$3" "$4" "$cwd" "$pcwd" "$args"
}
if [ "$(uname)" = Linux ] && command -v ss >/dev/null 2>&1; then
  ss -ltnpH 2>/dev/null | while read -r state recv send local peer users; do
    pid=$(printf '%s' "$users" | sed -n 's/.*pid=\([0-9]*\).*/\1/p')
    comm=$(printf '%s' "$users" | sed -n 's/^users:(("\([^"]*\)".*/\1/p')
    [ -n "$pid" ] && emit "${local##*:}" "${local%:*}" "$pid" "$comm"
  done
else
  lsof -nP -iTCP -sTCP:LISTEN -Fpcn 2>/dev/null | while read -r line; do
    case $line in
      p*) pid=${line#p} ;;
      c*) comm=${line#c} ;;
      n*) n=${line#n}; emit "${n##*:}" "${n%:*}" "$pid" "$comm" ;;
    esac
  done
fi
`
}

// infrastructure is the processes whose ports are never anyone's app: remuda, herdr, ssh and Tailscale's own
var infrastructure = map[string]bool{"remuda": true, "herdr": true, "sshd": true, "sshd-sess": true, "ssh": true, "tailscaled": true, "tailscale": true, "Tailscale": true, "IPNExtension": true}

// Parse reads the script's lines into the listeners worth a preview: one per port, never one in skip (the cockpit's
// own, the door, code-server), nor remuda's, herdr's, ssh's or Tailscale's, nor a process with no folder of the
// person's own (a system daemon runs in /, another user's answers nothing).
func Parse(out string, skip map[int]bool) []Listener {
	byPort := map[int]Listener{}
	for _, line := range script.Lines(out) {
		f := script.Fields(line)
		if len(f) != 7 {
			continue
		}
		port, err := strconv.Atoi(strings.TrimSpace(f[0]))
		if err != nil || port < 1024 || port > 65535 || skip[port] {
			continue
		}
		pid, _ := strconv.Atoi(strings.TrimSpace(f[2]))
		command := strings.TrimSpace(f[3])
		if infrastructure[command] {
			continue
		}
		dir := strings.TrimSpace(f[4])
		if dir == "" || dir == "/" {
			dir = strings.TrimSpace(f[5])
		}
		if dir == "" || dir == "/" {
			continue
		}
		address := bound(f[1])
		l := Listener{Port: port, Address: address, Dial: dialHost(address), PID: pid, Command: command, Label: labelOf(command, f[6]), Dir: path.Clean(dir)}
		if had, ok := byPort[port]; ok && !better(l, had) {
			continue
		}
		byPort[port] = l
	}
	list := make([]Listener, 0, len(byPort))
	for _, l := range byPort {
		list = append(list, l)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Port < list[j].Port })
	return list
}

// bound is a listener's address as written for a person: brackets and a zone dropped, a wildcard as *
func bound(address string) string {
	address = strings.TrimSpace(address)
	address = strings.TrimSuffix(strings.TrimPrefix(address, "["), "]")
	if i := strings.IndexByte(address, '%'); i >= 0 {
		address = address[:i]
	}
	switch address {
	case "", "*", "0.0.0.0":
		return "*"
	case "::":
		return "::"
	}
	return address
}

// dialHost is where the machine reaches a listener: its loopback for a wildcard, the IPv6 one for ::, else the address itself
func dialHost(address string) string {
	switch address {
	case "*":
		return "127.0.0.1"
	case "::", "::1":
		return "::1"
	}
	if ip := net.ParseIP(address); ip != nil {
		return ip.String()
	}
	return "127.0.0.1"
}

// better says which of two sockets on one port to keep: one the IPv4 loopback reaches first
func better(l, had Listener) bool {
	return had.Dial != "127.0.0.1" && l.Dial == "127.0.0.1"
}

// labels name what a dev server is by a word of its arguments, the first that matches
var labels = []struct{ word, label string }{
	{"vite", "vite"}, {"next", "next"}, {"nuxt", "nuxt"}, {"astro", "astro"}, {"remix", "remix"}, {"storybook", "storybook"},
	{"webpack", "webpack"}, {"parcel", "parcel"}, {"svelte-kit", "sveltekit"}, {"react-scripts", "react-scripts"}, {"expo", "expo"},
	{"http.server", "http.server"}, {"uvicorn", "uvicorn"}, {"gunicorn", "gunicorn"}, {"flask", "flask"}, {"runserver", "django"},
	{"streamlit", "streamlit"}, {"jupyter", "jupyter"}, {"rails", "rails"}, {"hugo", "hugo"}, {"jekyll", "jekyll"}, {"php -S", "php"},
	{"wrangler", "wrangler"}, {"docusaurus", "docusaurus"},
}

// labelOf is what the person knows the server as — vite, next, http.server — from its arguments, "" when nothing says
func labelOf(command, args string) string {
	lower := strings.ToLower(args)
	for _, l := range labels {
		if strings.Contains(lower, l.word) {
			return l.label
		}
	}
	return ""
}

// Match names each listener's workspace: the one whose folder holds the listener's, the deepest when folders nest
// (a worktree under its repository's folder). A Mac names a process's folder by its real path, /private/tmp for /tmp.
func Match(list []Listener, folders map[string]string) []Listener {
	for i := range list {
		dir := list[i].Dir
		if rest, ok := strings.CutPrefix(dir, "/private"); ok && (strings.HasPrefix(rest, "/tmp/") || strings.HasPrefix(rest, "/var/")) {
			dir = rest
		}
		best, deepest := "", 0
		for id, folder := range folders {
			folder = path.Clean(folder)
			if folder == "/" || folder == "." {
				continue
			}
			if (dir == folder || strings.HasPrefix(dir, folder+"/")) && len(folder) > deepest {
				best, deepest = id, len(folder)
			}
		}
		list[i].Workspace = best
	}
	return list
}
