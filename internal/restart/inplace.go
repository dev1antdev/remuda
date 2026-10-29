package restart

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// echo is how Claude Code shows the command it was given, on the pane's screen.
const echo = "❯ /restart"

// inPlaceWait is how long the screen is watched for what the in-place restart came to.
const inPlaceWait = 20 * time.Second

// InPlace is the agent's own command that restarts it in place with the session kept, "" for a kind without one.
func InPlace(kind string) string {
	if a, ok := agents[kind]; ok {
		return a.restart
	}
	return ""
}

// Verdict is what the screen says an in-place restart came to.
type Verdict int

const (
	Pending Verdict = iota // nothing yet
	Resumed                // the agent is back on its session
	Unknown                // the agent does not have the command here: an older build, or the feature off
	Refused                // the agent said why it will not restart this session
)

var bannerVersion = regexp.MustCompile(`Claude Code v(\d+\.\d+\.\d+)`)

// Echoes counts the command's echoes on the screen, so Outcome reads only what came after the one just sent.
func Echoes(screen string) int {
	return strings.Count(screen, echo)
}

// Outcome reads the screen past the command's newest echo — pending while no echo beyond the `before` already
// there has appeared — and answers the verdict with the agent's own line for an unknown command ("Unknown
// command: /restart" on 2.1.312) or a refusal.
func Outcome(screen string, before int) (Verdict, string) {
	if strings.Count(screen, echo) <= before {
		return Pending, ""
	}
	tail := screen[strings.LastIndex(screen, echo)+len(echo):]
	for _, line := range strings.Split(tail, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "⎿⏺"))
		switch {
		case strings.Contains(line, "Session resumed"):
			return Resumed, ""
		case strings.Contains(line, "Unknown command"), strings.Contains(line, "Unknown slash command"), strings.Contains(line, "isn't available"):
			return Unknown, line
		case strings.HasPrefix(line, "Can't restart"):
			return Refused, line
		}
	}
	return Pending, ""
}

// Version is the version the agent's banner shows on the screen, "" when the banner is not on it.
func Version(screen string) string {
	m := bannerVersion.FindStringSubmatch(screen)
	if m == nil {
		return ""
	}
	return m[1]
}

// inPlace asks the agent to restart itself with the session kept and reads the outcome off the screen, since herdr
// sees nothing of it: true when that settled the matter, false when the agent does not have the command, so the
// exit and resume follow.
func (r *Restarter) inPlace(key string, client *herdr.Client, p herdr.PaneInfo, a agent, session string, say func(string)) (bool, error) {
	paneID, title := p.PaneID, p.DisplayTitle()
	before, _ := client.PaneRead(paneID)
	echoes := Echoes(before)
	say(fmt.Sprintf("· %s (%s): %s %s", title, paneID, a.Title, a.restart))
	if err := client.AgentPrompt(paneID, a.restart); err != nil {
		return true, fmt.Errorf("%s: %s: %v", title, a.restart, err)
	}
	r.typed(key, paneID, a.restart)
	deadline := time.Now().Add(inPlaceWait)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		screen, err := client.PaneRead(paneID)
		if err != nil {
			continue
		}
		switch verdict, line := Outcome(screen, echoes); verdict {
		case Resumed:
			on := ""
			if v := Version(screen); v != "" {
				on = " on v" + v
			}
			say(fmt.Sprintf("  ✓ %s restarted in place%s, session %s kept", a.Title, on, short(session)))
			return true, nil
		case Unknown:
			say(fmt.Sprintf("  %s has no %s here (%s) — leaving and resuming instead", a.Title, a.restart, line))
			return false, nil
		case Refused:
			return true, fmt.Errorf("%s: %s — left as it is", title, line)
		}
	}
	return true, fmt.Errorf("%s: nothing on the screen said what %s did within %s — left as it is", title, a.restart, inPlaceWait)
}
