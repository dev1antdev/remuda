// Package limits reads how much of each coding agent's subscription is spent — Claude Code's 5-hour and weekly
// windows from what it hands its status line, Codex's from its own rollouts — machine by machine, warns at 80 % and at
// the limit, notes an agent that hit one, and keeps the person's bookings to continue such an agent after the reset.
package limits

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
)

// Window is one of an agent's subscription windows as the agent last reported it.
type Window struct {
	Agent    string    `json:"agent"`              // claude | codex
	Name     string    `json:"name"`               // 5h | week, or the window's length in minutes for one of another size
	Used     float64   `json:"used"`               // percent of the window spent, 0..100, above when exceeded
	ResetsAt time.Time `json:"resets_at"`          // when the window starts afresh
	At       time.Time `json:"at"`                 // when the agent reported it
	Forecast *Outlook  `json:"forecast,omitempty"` // when it runs out at the pace of its recent readings (forecast.go), set by Service.Of
}

// Reading is what one machine says of the limits: the windows, and whether Claude Code's status line is wrapped to report them.
type Reading struct {
	Windows []Window `json:"windows,omitempty"`
	Wrapped bool     `json:"wrapped,omitempty"` // `remuda statusline` sits in front of Claude Code's status line there
}

// SnapshotFile is where `remuda statusline` keeps the last rate limits Claude Code handed it, in the machine's state dir.
const SnapshotFile = "claude-limits.json"

// Snapshot is that file: when it was written and Claude Code's own rate_limits object, kept as it came.
type Snapshot struct {
	At         time.Time       `json:"at"`
	RateLimits json.RawMessage `json:"rate_limits"`
}

// Script reads a machine's limits in one shell: the newest rate_limits line of Codex's recent rollouts, the snapshot
// the status line wrapper keeps in remuda's state dir there, and whether the wrapper is in Claude Code's settings.
func Script() string {
	return `d="$HOME/.codex/sessions"
if [ -d "$d" ]; then
  find "$d" -name 'rollout-*.jsonl' -type f -mtime -8 -exec ls -t {} + 2>/dev/null | head -n 5 | while IFS= read -r f; do
    line=$(tail -c 1048576 "$f" 2>/dev/null | grep '"rate_limits"' | tail -n 1)
    if [ -n "$line" ]; then printf '%s\n' "$line"; break; fi
  done
fi
printf '\036\n'
case "$(uname -s)" in
  Darwin) s="$HOME/Library/Application Support/remuda" ;;
  *) s="${XDG_CONFIG_HOME:-$HOME/.config}/remuda" ;;
esac
cat "$s/` + SnapshotFile + `" 2>/dev/null
printf '\n\036\n'
grep -q 'remuda[^"]*statusline' "$HOME/.claude/settings.json" 2>/dev/null && echo wrapped
exit 0`
}

// Parse reads Script's answer; windows already reset by now are left out, since what they say no longer holds.
func Parse(out string, now time.Time) Reading {
	sections := script.Sections(out)
	var r Reading
	if len(sections) > 0 {
		r.Windows = append(r.Windows, CodexWindows([]byte(strings.TrimSpace(sections[0])))...)
	}
	if len(sections) > 1 {
		r.Windows = append(r.Windows, ClaudeWindows([]byte(strings.TrimSpace(sections[1])))...)
	}
	if len(sections) > 2 {
		r.Wrapped = strings.TrimSpace(sections[2]) == "wrapped"
	}
	kept := r.Windows[:0]
	for _, w := range r.Windows {
		if w.ResetsAt.After(now) {
			kept = append(kept, w)
		}
	}
	r.Windows = kept
	sort.SliceStable(r.Windows, func(i, j int) bool { return rank(r.Windows[i]) < rank(r.Windows[j]) })
	return r
}

// rank orders the windows: Claude's before Codex's, the 5-hour before the week
func rank(w Window) string {
	agent := map[string]string{"claude": "0", "codex": "1"}[w.Agent]
	name := map[string]string{"5h": "0", "week": "1"}[w.Name]
	return agent + w.Agent + name + w.Name
}

// CodexWindows are the windows of a Codex rollout record carrying rate_limits: its primary and its secondary.
func CodexWindows(line []byte) []Window {
	if len(line) == 0 {
		return nil
	}
	var rec struct {
		Timestamp string `json:"timestamp"`
		Payload   struct {
			RateLimits *struct {
				Primary   *codexWindow `json:"primary"`
				Secondary *codexWindow `json:"secondary"`
			} `json:"rate_limits"`
		} `json:"payload"`
	}
	if json.Unmarshal(line, &rec) != nil || rec.Payload.RateLimits == nil {
		return nil
	}
	at, _ := time.Parse(time.RFC3339Nano, rec.Timestamp)
	var out []Window
	for _, w := range []*codexWindow{rec.Payload.RateLimits.Primary, rec.Payload.RateLimits.Secondary} {
		if w == nil || w.ResetsAt <= 0 {
			continue
		}
		out = append(out, Window{Agent: "codex", Name: windowName(w.WindowMinutes), Used: w.UsedPercent, ResetsAt: time.Unix(w.ResetsAt, 0).UTC(), At: at})
	}
	return out
}

type codexWindow struct {
	UsedPercent   float64 `json:"used_percent"`
	WindowMinutes int     `json:"window_minutes"`
	ResetsAt      int64   `json:"resets_at"`
}

func windowName(minutes int) string {
	switch minutes {
	case 300:
		return "5h"
	case 10080:
		return "week"
	}
	return fmt.Sprintf("%dm", minutes)
}

// ClaudeWindows are the 5-hour and weekly windows of the status line wrapper's snapshot.
func ClaudeWindows(data []byte) []Window {
	if len(data) == 0 {
		return nil
	}
	var snap Snapshot
	if json.Unmarshal(data, &snap) != nil || len(snap.RateLimits) == 0 {
		return nil
	}
	var limits map[string]*struct {
		UsedPercentage *float64 `json:"used_percentage"`
		ResetsAt       int64    `json:"resets_at"`
	}
	if json.Unmarshal(snap.RateLimits, &limits) != nil {
		return nil
	}
	var out []Window
	for _, name := range []struct{ key, name string }{{"five_hour", "5h"}, {"seven_day", "week"}} {
		w := limits[name.key]
		if w == nil || w.UsedPercentage == nil || w.ResetsAt <= 0 {
			continue
		}
		out = append(out, Window{Agent: "claude", Name: name.name, Used: *w.UsedPercentage, ResetsAt: time.Unix(w.ResetsAt, 0).UTC(), At: snap.At})
	}
	return out
}
