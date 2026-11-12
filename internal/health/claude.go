package health

// Claude Code's sign-in and its user settings, as the vitals read them: when the stored sign-in ends (`claude auth
// status`, Claude Code 2.1.318 and up), and the default model and fast mode its settings.json keeps. The status answer
// names the account; the probe passes on the end of the sign-in alone, so no e-mail or organization leaves the machine.

import (
	"log"
	"maps"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/state"
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

// LoginRetry is how soon a read that failed is tried again: a `claude auth status` that outran its 20 s is not started
// once more at every probe.
var LoginRetry = time.Hour

// LoginsFile keeps every machine's sign-in as last read, so a restart of remuda does not ask Claude Code again.
const LoginsFile = "claude-logins.json"

// loginRead is a machine's sign-in as last read, when it was read, and when it was last asked for, answered or not.
type loginRead struct {
	Login *Login    `json:"login,omitempty"`
	At    time.Time `json:"at"`
	Tried time.Time `json:"tried"`
}

// due says whether the sign-in is asked for again at now: the person asked, the last answer is LoginEvery old, and
// no ask failed in the last LoginRetry; a moment ahead of the clock (it was set back) is old.
func (r loginRead) due(now time.Time, force bool) bool {
	if force {
		return true
	}
	fresh := func(at time.Time, within time.Duration) bool {
		age := now.Sub(at)
		return !at.IsZero() && age >= 0 && age < within
	}
	return !fresh(r.At, LoginEvery) && !fresh(r.Tried, LoginRetry)
}

// loadLogins reads the sign-ins kept from before; a damaged file is none, said once.
func loadLogins(dir string) map[string]loginRead {
	logins := map[string]loginRead{}
	if dir == "" {
		return logins
	}
	if err := state.LoadJSON(dir, LoginsFile, &logins); err != nil || logins == nil {
		if err != nil {
			log.Printf("health: %s is not readable (%v); the sign-ins are read afresh", LoginsFile, err)
		}
		return map[string]loginRead{}
	}
	return logins
}

// lastLogin is a machine's sign-in as last read, nil before the first read.
func (c *Collector) lastLogin(id string) *Login {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.logins[id].Login
}

// readLogin asks Claude Code on the machine when its sign-in ends when the last answer is LoginEvery old, or now when
// forced; a read that fails keeps the last answer. Never on the host of a remuda serving the page only: it runs no agents.
func (c *Collector) readLogin(mc *machines.Machine, force bool) *Login {
	if mc.Local() && c.machines.Server() {
		return nil
	}
	return c.signIn(mc.ID(), force, func() (string, error) {
		return shell.Text(mc, script.PATH+"\n"+LoginScript, 20*time.Second)
	})
}

// signIn is a machine's sign-in: the last answer while it is not due, else what ask brings back, kept on disk; an ask
// that fails or is cut short keeps the last answer and waits LoginRetry.
func (c *Collector) signIn(id string, force bool, ask func() (string, error)) *Login {
	c.mu.Lock()
	last := c.logins[id]
	c.mu.Unlock()
	if !last.due(time.Now(), force) {
		return last.Login
	}
	out, err := ask()
	now := time.Now()
	login, read := ParseLogin(out, now)
	if err == nil && read {
		last = loginRead{Login: login, At: now}
	}
	last.Tried = now
	c.keepLogin(id, last)
	return last.Login
}

// keepLogin notes a machine's read and writes every machine's out, one write at a time so an older copy never lands last.
func (c *Collector) keepLogin(id string, read loginRead) {
	c.loginsSaving.Lock()
	defer c.loginsSaving.Unlock()
	c.mu.Lock()
	c.logins[id] = read
	snapshot := maps.Clone(c.logins)
	c.mu.Unlock()
	if c.dir == "" {
		return
	}
	if err := state.SaveJSON(c.dir, LoginsFile, snapshot); err != nil {
		log.Printf("health: %s not written: %v", LoginsFile, err)
	}
}
