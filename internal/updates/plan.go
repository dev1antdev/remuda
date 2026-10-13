// Package updates is what a machine can update and how: herdr, the agents
// and remuda itself, each a command the person sees before it runs as a
// job, with the chats brought onto a new agent afterwards. Nothing asks
// for sudo.
package updates

import (
	"fmt"
	"strings"

	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/script"
)

// Update is a command the page may run on a machine, shown to the person before they say yes.
type Update struct {
	What    string `json:"what"`
	Title   string `json:"title"`
	Command string `json:"command"`
	Note    string `json:"note,omitempty"`
}

// brewUpdate goes before every `brew upgrade`: Homebrew's own auto-update runs
// once a day at most, so without it an upgrade can miss a version that is out.
const brewUpdate = "brew update && "

// serverStep follows an install of herdr: a comment for the shell, and the promise the herdr restart keeps.
const serverStep = "\n# then the running server onto it: kept when herdr's handoff already did that, else restarted with every chat started again on its session"

const serverNote = "When the running server is still on the old version afterwards (Homebrew, or a handoff that did not happen) it is restarted: herdr restores the workspaces, tabs and panes, and remuda starts every chat again with its session resumed; shells and other pane processes come back as fresh shells."

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
	case "herdr":
		if r == nil || r.Herdr == "" {
			return Update{}, fmt.Errorf("herdr is not installed there")
		}
		if r.HerdrVia == "brew" {
			return Update{What: what, Title: "Update herdr", Command: path + brewUpdate + "brew upgrade herdr && herdr --version" + serverStep,
				Note: "Installed with Homebrew: the formulae are refreshed first, then the upgrade. " + serverNote}, nil
		}
		return Update{What: what, Title: "Update herdr", Command: path + "herdr update --handoff && herdr --version" + serverStep,
			Note: "herdr's own updater, with a live handoff: the new server takes the running panes over. " + serverNote}, nil
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
	case "agents":
		var parts []string
		brew := false // one refresh of the formulae for all of them
		for _, w := range []string{"claude", "codex", "gemini"} {
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
	for _, c := range []cand{{"herdr", r.Herdr, l.Herdr}, {"claude", r.Claude, l.Claude}, {"codex", r.Codex, l.Codex}, {"gemini", r.Gemini, l.Gemini}} {
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
		if c.what == "herdr" && r.HerdrServer != "" && r.HerdrServer != r.Herdr {
			u.Note = "The server runs " + r.HerdrServer + " while " + r.Herdr + " is installed. " + u.Note
		}
		out = append(out, u)
	}
	return out
}
