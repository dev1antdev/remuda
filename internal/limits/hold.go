package limits

import (
	"fmt"
	"math"
	"time"
)

// HoldAt is the use from which a new agent of the kind waits for the window's reset: a run started on the last few
// percent stops half way.
const HoldAt = 95.0

// RunAhead is how soon a window forecast to run out holds a new start the same way: within it, the run would not finish.
const RunAhead = 30 * time.Minute

// Hold is why a new agent of a kind should wait: the window it waits on, in words, and its episode — the window's own
// key, so whatever hangs on the hold (an offer to run on the other vendor) is made once and goes with the reset.
type Hold struct {
	Window  Window
	Why     string
	Episode string
}

// Holding is why a new agent of the kind should not start on the machine now: one of its windows at HoldAt or past it,
// or forecast to run out within RunAhead. The machine's own reading counts, else the freshest of another machine's —
// one subscription is one account, whatever machine reads it.
func (s *Service) Holding(machine, agent string) (Hold, bool) {
	now := s.now()
	for _, w := range s.Windows(machine, agent) {
		if why := holdWords(w, now); why != "" {
			return Hold{Window: w, Why: why, Episode: alertKey(w)}, true
		}
	}
	return Hold{}, false
}

// Windows is the agent's open windows as the machine read them, else as the freshest reading of any machine says.
func (s *Service) Windows(machine, agent string) []Window {
	own := agentWindows(s.Of(machine).Windows, agent)
	if len(own) > 0 {
		return own
	}
	s.mu.Lock()
	ids := make([]string, 0, len(s.readings))
	for id := range s.readings {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	best := map[string]Window{}
	for _, id := range ids {
		for _, w := range agentWindows(s.Of(id).Windows, agent) {
			if have, ok := best[w.Name]; !ok || w.At.After(have.At) {
				best[w.Name] = w
			}
		}
	}
	var out []Window
	for _, name := range []string{"5h", "week"} {
		if w, ok := best[name]; ok {
			out = append(out, w)
			delete(best, name)
		}
	}
	for _, w := range best {
		out = append(out, w)
	}
	return out
}

func agentWindows(windows []Window, agent string) []Window {
	var out []Window
	for _, w := range windows {
		if w.Agent == agent {
			out = append(out, w)
		}
	}
	return out
}

// holdWords says why the window holds a new start, "" when it does not.
func holdWords(w Window, now time.Time) string {
	until := w.ResetsAt.Local().Format("Jan 2 15:04")
	switch {
	case w.Used >= 100:
		return fmt.Sprintf("%s's %s window is spent until %s", AgentName(w.Agent), shortWindow(w.Name), until)
	case w.Used >= HoldAt:
		return fmt.Sprintf("%s's %s window is at %d %% until %s", AgentName(w.Agent), shortWindow(w.Name), int(math.Floor(w.Used)), until)
	case w.Forecast != nil && w.Forecast.RunsOutAt != nil && w.Forecast.RunsOutAt.Before(now.Add(RunAhead)):
		return fmt.Sprintf("%s's %s window runs out ≈ %s at this pace, before its reset at %s", AgentName(w.Agent), shortWindow(w.Name), w.Forecast.RunsOutAt.Local().Format("15:04"), until)
	}
	return ""
}

// shortWindow is a window's length as a reason says it: "5 h", "weekly".
func shortWindow(name string) string {
	switch name {
	case "5h":
		return "5 h"
	case "week":
		return "weekly"
	}
	return name
}

// Offer is an offer to run what this hold keeps waiting, the window it waits on filled in; the caller names the work.
func (h Hold) Offer(o Offer) Offer {
	o.Why, o.Episode, o.Window, o.Used, o.ResetsAt = h.Why, h.Episode, h.Window.Name, h.Window.Used, h.Window.ResetsAt
	if h.Window.Used < HoldAt && h.Window.Forecast != nil {
		o.RunsOut = h.Window.Forecast.RunsOutAt
	}
	return o
}

// Other is the vendor whose agent may run what the plan limit holds for this one: Claude Code and Codex stand in for
// each other, and no other agent has its limits read.
func Other(agent string) string {
	switch agent {
	case "claude":
		return "codex"
	case "codex":
		return "claude"
	}
	return ""
}
