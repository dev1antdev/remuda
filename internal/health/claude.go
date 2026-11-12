package health

// Claude Code's sign-in and its user settings, as the vitals read them: when the stored sign-in ends (`claude auth
// status`, Claude Code 2.1.318 and up), and the default model and fast mode its settings.json keeps. The status answer
// names the account; the probe passes on the end of the sign-in alone, so no e-mail or organization leaves the machine.

import (
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// Login is when Claude Code's stored sign-in on a machine ends, and when that was read.
type Login struct {
	Expires time.Time `json:"expires"`
	At      time.Time `json:"at"`
}

// LoginEvery is how often the sign-in is read: `claude auth status` starts Claude Code itself, which counts the call
// in its own analytics, and the end of a sign-in moves only when the person signs in again.
var LoginEvery = 6 * time.Hour

// LoginScript prints login_expires=<RFC 3339> when Claude Code says when its stored sign-in ends, and login_read once
// it asked; the answer's other fields never leave the sed.
const LoginScript = `if command -v claude >/dev/null 2>&1; then claude auth status --json 2>/dev/null | tr ',{}' '\n\n\n' | sed -n -E 's/^[[:space:]]*"loginExpiresAt"[[:space:]]*:[[:space:]]*"([^"]*)".*$/login_expires=\1/p' | head -1; fi
echo login_read=1
`

// SettingsScript prints the default model and the saved fast mode of Claude Code's user settings: the top-level
// "model", and claude_fast=on when fastMode is saved on and not reset each session (fastModePerSessionOptIn).
const SettingsScript = `cs="$HOME/.claude/settings.json"; if [ -r "$cs" ]; then sed -n 's/^  "model"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/claude_model=\1/p' "$cs" | head -1; if grep -q '"fastMode"[[:space:]]*:[[:space:]]*true' "$cs" && ! grep -q '"fastModePerSessionOptIn"[[:space:]]*:[[:space:]]*true' "$cs"; then echo "claude_fast=on"; fi; fi
`

// parseClaude reads the settings' lines into the report; false for a line that is none of them
func parseClaude(r *Report, k, v string) bool {
	switch k {
	case "claude_model":
		r.ClaudeModel = v
	case "claude_fast":
		r.ClaudeFast = v == "on"
	default:
		return false
	}
	return true
}

// ParseLogin reads LoginScript's answer: the sign-in's end, nil when Claude Code names none (an API key, no sign-in,
// a version before 2.1.318), and whether Claude Code was asked at all.
func ParseLogin(out string, at time.Time) (*Login, bool) {
	var login *Login
	read := false
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "login_read":
			read = true
		case "login_expires":
			if expires, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(v)); err == nil {
				login = &Login{Expires: expires.UTC(), At: at}
			}
		}
	}
	return login, read
}

// loginRead is a machine's sign-in as last read, and when.
type loginRead struct {
	login *Login
	at    time.Time
}

// lastLogin is a machine's sign-in as last read, nil before the first read.
func (c *Collector) lastLogin(id string) *Login {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.logins[id].login
}

// readLogin asks Claude Code on the machine when its sign-in ends when the last answer is LoginEvery old, or now when
// forced; a read that fails keeps the last answer. Never on the host of a remuda serving the page only: it runs no agents.
func (c *Collector) readLogin(mc *machines.Machine, force bool) *Login {
	if mc.Local() && c.machines.Server() {
		return nil
	}
	c.mu.Lock()
	last, seen := c.logins[mc.ID()]
	c.mu.Unlock()
	if seen && !force && time.Since(last.at) < LoginEvery {
		return last.login
	}
	out, err := shell.Text(mc, script.PATH+"\n"+LoginScript, 20*time.Second)
	if err != nil {
		return last.login
	}
	login, read := ParseLogin(out, time.Now())
	if !read {
		return last.login
	}
	c.mu.Lock()
	c.logins[mc.ID()] = loginRead{login: login, at: time.Now()}
	c.mu.Unlock()
	return login
}
