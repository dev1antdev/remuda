package gitsync

import (
	"strconv"
	"strings"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// probe is how a checkout stands before an action: where its top is, its branch and what that tracks, how far apart
// they are, and the remotes it has.
type probe struct {
	missing  bool // the folder is not there
	notRepo  bool // it is no git checkout
	top      string
	branch   string // "" for a detached HEAD
	remote   string // branch.<b>.remote, "." for a local upstream
	merge    string // branch.<b>.merge, refs/heads/main
	upstream string // the tracking branch as git names it, origin/main; "" when none is set or it is gone
	counted  bool   // ahead and behind were read
	ahead    int
	behind   int
	remotes  []string
}

// probeScript prints the checkout's facts as key=value lines.
func probeScript(style rpath.Style, dir string) string {
	return `cd ` + script.Quote(style.Slash(dir)) + ` 2>/dev/null || { printf 'missing=1\n'; exit 0; }; ` +
		`top=$(` + read + ` rev-parse --show-toplevel 2>/dev/null) || { printf 'norepo=1\n'; exit 0; }; cd "$top" || exit 0; printf 'top=%s\n' "$top"; ` +
		`b=$(git symbolic-ref --short -q HEAD 2>/dev/null); printf 'branch=%s\n' "$b"; ` +
		`if [ -n "$b" ]; then printf 'remote=%s\n' "$(git config --get "branch.$b.remote" 2>/dev/null)"; printf 'merge=%s\n' "$(git config --get "branch.$b.merge" 2>/dev/null)"; fi; ` +
		`if u=$(git rev-parse --abbrev-ref --symbolic-full-name '@{u}' 2>/dev/null) && [ -n "$u" ]; then printf 'upstream=%s\n' "$u"; ` +
		`printf 'counts=%s\n' "$(` + read + ` rev-list --left-right --count 'HEAD...@{u}' 2>/dev/null)"; fi; ` +
		`printf 'remotes=%s\n' "$(git remote 2>/dev/null | tr '\n' ' ')"`
}

// readProbe runs the probe on the machine.
func readProbe(host shell.Host, style rpath.Style, dir string) (probe, error) {
	output, err := shell.Text(host, probeScript(style, dir), Timeout)
	if err != nil {
		return probe{}, err
	}
	return parseProbe(style, output), nil
}

func parseProbe(style rpath.Style, output string) probe {
	var p probe
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch key {
		case "missing":
			p.missing = true
		case "norepo":
			p.notRepo = true
		case "top":
			p.top = style.Clean(value)
		case "branch":
			p.branch = value
		case "remote":
			p.remote = value
		case "merge":
			p.merge = value
		case "upstream":
			p.upstream = value
		case "counts":
			fields := strings.Fields(value)
			if len(fields) == 2 {
				ahead, errAhead := strconv.Atoi(fields[0])
				behind, errBehind := strconv.Atoi(fields[1])
				p.ahead, p.behind, p.counted = ahead, behind, errAhead == nil && errBehind == nil
			}
		case "remotes":
			p.remotes = strings.Fields(value)
		}
	}
	return p
}

// tracks says the branch has an upstream set, whether or not it is still there.
func (p probe) tracks() bool { return p.remote != "" && p.merge != "" }

// hasRemote says the repository knows the remote.
func (p probe) hasRemote(name string) bool {
	for _, remote := range p.remotes {
		if remote == name {
			return true
		}
	}
	return false
}

// fetchRemote is the remote a fetch or a pull reads: the upstream's, else origin, else the only one.
func (p probe) fetchRemote() string {
	switch {
	case p.remote != "" && p.remote != "." && p.hasRemote(p.remote):
		return p.remote
	case p.hasRemote("origin"):
		return "origin"
	case len(p.remotes) == 1:
		return p.remotes[0]
	}
	return ""
}
