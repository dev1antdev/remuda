// Package updates is what a machine can update and how: remuda-core, the
// agents and remuda itself, each a command the person sees before it runs
// as a job, with the chats brought onto a new agent afterwards. Nothing
// asks for sudo.
package updates

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/restart"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/setup"
)

// Update is a command the page may run on a machine, shown to the person before they say yes.
type Update struct {
	What    string `json:"what"`
	Title   string `json:"title"`
	Command string `json:"command"`
	Note    string `json:"note,omitempty"`

	script string // what runs when the command shown only names it: a script this remuda carries, too long to travel back on every Run
}

// naming is a command shown in place of the script it names, the script's digest in it: one changed since it was
// shown makes the command stale.
func naming(words, script string) string {
	sum := sha256.Sum256([]byte(script))
	return words + "\n# the script's sha256: " + hex.EncodeToString(sum[:8])
}

// run is the script the update runs: its own, else the command shown.
func (u Update) run() string {
	if u.script != "" {
		return u.script
	}
	return u.Command
}

// brewUpdate goes before every `brew upgrade`: Homebrew's own auto-update runs
// once a day at most, so without it an upgrade can miss a version that is out.
const brewUpdate = "brew update && "

// arrow is " → want", or nothing when the version is unknown.
func arrow(want string) string {
	if want == "" {
		return ""
	}
	return " → " + want
}

// remudaThere sets $r to the machine's own remuda: ~/.local/bin/remuda, or remuda.exe there on Windows.
const remudaThere = `r="$HOME/.local/bin/remuda"; [ -x "$r" ] || r="$r.exe"; `

// windowsReport says the vitals are a Windows machine's.
func windowsReport(r *health.Report) bool { return r != nil && strings.HasPrefix(r.OS, "Windows") }

// Plan is the update for one thing on a machine with this report, run as the user and never with sudo.
func Plan(what string, r *health.Report) (Update, error) {
	path := script.PATH + "; "
	via := func(cmdVia, brew, npm, native string) (string, error) {
		switch cmdVia {
		case "brew":
			return path + brewUpdate + brew, nil
		case "npm":
			return path + npm, nil
		case "native", "":
			return path + native, nil
		}
		return "", fmt.Errorf("unknown install")
	}
	switch what {
	case "code":
		if r == nil || r.CodeServer == "" {
			return Update{}, fmt.Errorf("code-server is not installed there")
		}
		if r.Remuda == "" {
			return Update{}, fmt.Errorf("remuda is not installed there to run its installer")
		}
		return Update{What: what, Title: "Update code-server", Command: path + remudaThere + `"$r" code install && readlink "$HOME/.local/opt/code-server"`, Note: "remuda's own installer: the pinned release, its checksum verified, the service restarted; an editor open on it reconnects (Reload the editor if it does not)."}, nil
	case "core":
		if r == nil || AssetName(r.OS, r.Arch) == "" || windowsReport(r) && r.Arch == "arm64" { // remuda's own builds, less Windows on arm
			return Update{}, fmt.Errorf("remuda-core has no build for this machine, or its vitals are not read yet")
		}
		if r.RemudaCore != "" && !health.Newer(r.RemudaCore, health.PinnedCore) {
			return Update{}, fmt.Errorf("remuda-core %s is there already", r.RemudaCore)
		}
		u := Update{What: what, Title: "Install remuda-core", Note: "The terminal runtime remuda's machines move to, the pinned release with its checksum verified, put in ~/.local/bin by this remuda's own installer over your SSH connection, so no remuda need run there. Nothing starts until remuda connects to it; herdr and its agents carry on as they are."}
		if r.RemudaCore != "" {
			u.Title, u.Note = "Update remuda-core", r.RemudaCore+arrow(health.PinnedCore)+". Swapped by its own update: its terminals keep running."
		}
		u.script = path + "\n" + setup.CoreInstaller()
		u.Command = naming("# this remuda's own remuda-core installer, the one `remuda core -host H install` runs, over the SSH connection:\n# remuda-core "+health.PinnedCore+", its checksum verified, into ~/.local/bin", u.script)
		return u, nil
	case "mcp":
		if r == nil || r.Remuda == "" {
			return Update{}, fmt.Errorf("remuda is not installed there to serve the tasks")
		}
		return Update{What: what, Title: "Connect the agents to your tasks (MCP)", Command: path + remudaThere + `"$r" mcp install`,
			Note: "Registers remuda's MCP server with every coding agent on the machine through the agent's own command (claude mcp add, codex mcp add, gemini mcp add; Kimi when its CLI can): the agents can then list, create, move and comment on your tasks. Safe to run again; an agent already running sees the server once restarted."}, nil
	case "statusline":
		if r == nil || r.Claude == "" {
			return Update{}, fmt.Errorf("Claude Code is not installed there")
		}
		if r.Remuda == "" {
			return Update{}, fmt.Errorf("remuda is not installed there to keep the limits")
		}
		if r.Statusline {
			return Update{}, fmt.Errorf("Claude Code's status line already goes through remuda there")
		}
		return Update{What: what, Title: "Show Claude Code's plan limits", Command: path + remudaThere + `"$r" statusline status >/dev/null 2>&1 || { echo "this remuda is older than the status line wrapper: update remuda here first"; exit 1; }; "$r" statusline install`,
			Note: "Claude Code hands its status line the 5-hour and weekly limits of a Pro, Max or Ultra plan. This puts remuda in front of your status line in ~/.claude/settings.json (the file and the setting it replaces kept in remuda's state dir first): it notes the limits for the meter and the warnings, then runs your own status line as before. `remuda statusline uninstall` there puts it back."}, nil
	case "claude":
		if r == nil || r.Claude == "" {
			return Update{}, fmt.Errorf("Claude Code is not installed there")
		}
		c, err := via(r.ClaudeVia, "brew upgrade claude-code", "npm install -g @anthropic-ai/claude-code@latest", "claude update")
		if err != nil {
			return Update{}, err
		}
		return Update{What: what, Title: "Update Claude Code", Command: c + " && claude --version", Note: "Running agents keep the old version until they are restarted."}, nil
	case "codex":
		if r == nil || r.Codex == "" {
			return Update{}, fmt.Errorf("Codex is not installed there")
		}
		c, err := via(r.CodexVia, "brew upgrade codex", "npm install -g @openai/codex@latest", "npm install -g @openai/codex@latest")
		if err != nil {
			return Update{}, err
		}
		return Update{What: what, Title: "Update Codex", Command: c + " && codex --version", Note: "Running agents keep the old version until they are restarted."}, nil
	case "gemini":
		if r == nil || r.Gemini == "" {
			return Update{}, fmt.Errorf("Gemini CLI is not installed there")
		}
		c, err := via(r.GeminiVia, "brew upgrade gemini-cli", "npm install -g @google/gemini-cli@latest", "npm install -g @google/gemini-cli@latest")
		if err != nil {
			return Update{}, err
		}
		return Update{What: what, Title: "Update Gemini CLI", Command: c + " && gemini --version", Note: "Running agents keep the old version until they are restarted."}, nil
	case "kimi":
		if r == nil || r.Kimi == "" {
			return Update{}, fmt.Errorf("Kimi Code is not installed there")
		}
		c, err := via(r.KimiVia, "brew upgrade kimi-code", "npm install -g @moonshot-ai/kimi-code@latest", "kimi upgrade -y")
		if err != nil {
			return Update{}, err
		}
		return Update{What: what, Title: "Update Kimi Code", Command: c + " && kimi --version", Note: "Running agents keep the old version until they are restarted."}, nil
	case "agents":
		var parts []string
		brew := false // one refresh of the formulae for all of them
		for _, w := range restart.Kinds("agents") {
			if u, err := Plan(w, r); err == nil {
				c := strings.TrimPrefix(u.Command, path)
				if strings.HasPrefix(c, brewUpdate) {
					brew = true
					c = strings.TrimPrefix(c, brewUpdate)
				}
				parts = append(parts, `echo "== `+u.Title+`"; `+c+` || { echo "!! failed"; rc=1; }`)
			}
		}
		if len(parts) == 0 {
			return Update{}, fmt.Errorf("no agent is installed there")
		}
		head := "rc=0; "
		if brew {
			head += `echo "== brew update"; brew update || { echo "!! brew update failed"; rc=1; }; `
		}
		return Update{What: what, Title: "Update every agent", Command: path + head + strings.Join(parts, "; ") + "; exit $rc", Note: "Running agents keep the old version until they are restarted."}, nil
	}
	return Update{}, fmt.Errorf("nothing to update called %q", what)
}

// Offered lists what the machine can update right now, each noting whether a newer version is out.
func Offered(r *health.Report, l health.Latest) []Update {
	var out []Update
	type cand struct {
		what, have, latest string
	}
	for _, c := range []cand{{"claude", r.Claude, l.Claude}, {"codex", r.Codex, l.Codex}, {"gemini", r.Gemini, l.Gemini}, {"kimi", r.Kimi, l.Kimi}} {
		if c.have == "" {
			continue
		}
		u, err := Plan(c.what, r)
		if err != nil {
			continue
		}
		if c.latest != "" && health.Newer(c.have, c.latest) {
			u.Note = c.have + " → " + c.latest + ". " + u.Note
		} else {
			u.Note = c.have + " is the latest known. " + u.Note
		}
		out = append(out, u)
	}
	if u, err := Plan("code", r); err == nil {
		if l.Code != "" && health.Newer(r.CodeServer, l.Code) {
			u.Note = r.CodeServer + " → " + l.Code + ". " + u.Note
		} else {
			u.Note = r.CodeServer + " is what this remuda installs. " + u.Note
		}
		out = append(out, u)
	}
	if u, err := Plan("core", r); err == nil {
		out = append(out, u)
	}
	if u, err := Plan("mcp", r); err == nil {
		out = append(out, u)
	}
	if u, err := Plan("statusline", r); err == nil {
		out = append(out, u)
	}
	return out
}
