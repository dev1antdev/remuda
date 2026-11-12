// Package placement picks the machine a project's queued card starts on when its queue may use any machine that has
// room: among the connected machines with a workspace open on a checkout of the project's repository, the one with
// the lightest load per core, a cool CPU, its charger in, room on the agent's plan window and the fewest agents
// working — and says why, and why the others were left out. The choosing is pure; the remotes that tell which
// checkouts hold the repository are read by remotes.go.
package placement

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// The thresholds a machine is left out at, and where it starts losing points.
const (
	MaxPerCore = 2.0 // a load of two per core: the machine is busy enough
	Warm       = 75  // °C: a look, and points lost
	Hot        = 85  // °C: left out
	LowBattery = 20  // %: on battery and this low, left out
)

// Why a machine has no room.
const (
	OutLoad    = "load"
	OutHot     = "hot"
	OutBattery = "battery"
	OutPlan    = "plan"
	OutBudget  = "budget"
)

// Battery is a machine's battery as its vitals read it.
type Battery struct {
	Percent     int  `json:"percent"`
	Discharging bool `json:"discharging,omitempty"`
}

// Machine is a candidate: a connected machine of the fleet holding a workspace on a checkout of the repository.
type Machine struct {
	ID        string   `json:"machine"`
	Label     string   `json:"label"`
	Dir       string   `json:"dir"`
	Workspace string   `json:"-"`              // the workspace the worktree branches off
	Home      bool     `json:"home,omitempty"` // the machine the project's settings name
	PerCore   float64  `json:"load_per_core"`  // the load over the cores; 0 when it was not read
	TempC     float64  `json:"temp_c,omitempty"`
	Battery   *Battery `json:"battery,omitempty"`
	Plan      float64  `json:"plan"`           // the agent kind's fullest window there, percent; -1 when none was read
	Held      string   `json:"held,omitempty"` // the plan window holds a new start (limits.Holding), in words
	Working   int      `json:"working"`        // agents working there now
	Budget    string   `json:"budget,omitempty"`
	Out       string   `json:"out,omitempty"` // why it has no room (Out…), "" when it has
}

// Result is the pick, if any, and every other candidate in the order they scored, the ones with no room saying why.
type Result struct {
	Pick   *Machine  `json:"pick,omitempty"`
	Others []Machine `json:"others,omitempty"`
}

// PlanOnly says whether no machine had room and every one was left out on the plan window alone: running the work on
// another vendor's agent would find room.
func (r Result) PlanOnly() bool {
	if r.Pick != nil || len(r.Others) == 0 {
		return false
	}
	for _, m := range r.Others {
		if m.Out != OutPlan {
			return false
		}
	}
	return true
}

// LoadPerCore is a load over the cores, a machine of unknown cores counting as one.
func LoadPerCore(load float64, cores int) float64 {
	if cores < 1 {
		cores = 1
	}
	return math.Round(load/float64(cores)*100) / 100
}

// out is why a machine has no room, "" when it has.
func out(m Machine) string {
	switch {
	case m.Held != "":
		return OutPlan
	case m.Budget != "":
		return OutBudget
	case m.TempC >= Hot:
		return OutHot
	case m.Battery != nil && m.Battery.Discharging && m.Battery.Percent <= LowBattery:
		return OutBattery
	case m.PerCore >= MaxPerCore:
		return OutLoad
	}
	return ""
}

// Score is a machine's cost of taking the next agent, lower the better: 40 points a load of one per core, 25 for a
// warm CPU, 30 on battery, 0.3 a percent of the plan window spent, 15 an agent working there; the home machine gets 5
// back, so it wins a tie.
func Score(m Machine) float64 {
	score := m.PerCore * 40
	if m.TempC >= Warm {
		score += 25
	}
	if m.Battery != nil && m.Battery.Discharging {
		score += 30
	}
	if m.Plan > 0 {
		score += m.Plan * 0.3
	}
	score += float64(m.Working) * 15
	if m.Home {
		score -= 5
	}
	return score
}

// Choose picks the candidate with room and the lowest score; candidates scoring the same keep the order given.
func Choose(candidates []Machine) Result {
	scored := make([]Machine, len(candidates))
	copy(scored, candidates)
	for i := range scored {
		scored[i].Out = out(scored[i])
	}
	sort.SliceStable(scored, func(i, j int) bool {
		a, b := scored[i], scored[j]
		if (a.Out == "") != (b.Out == "") {
			return a.Out == ""
		}
		return Score(a) < Score(b)
	})
	var r Result
	for i := range scored {
		if r.Pick == nil && scored[i].Out == "" {
			pick := scored[i]
			r.Pick = &pick
			continue
		}
		r.Others = append(r.Others, scored[i])
	}
	return r
}

// Facts are a machine's figures as a reason says them: "load 0.3 per core, 1 agent working, Claude at 40 %".
func Facts(m Machine, agent string) string {
	var facts []string
	facts = append(facts, fmt.Sprintf("load %.1f per core", m.PerCore))
	if m.TempC >= Warm {
		facts = append(facts, fmt.Sprintf("warm at %.0f °C", m.TempC))
	}
	if m.Battery != nil && m.Battery.Discharging {
		facts = append(facts, fmt.Sprintf("on battery at %d %%", m.Battery.Percent))
	}
	switch m.Working {
	case 0:
		facts = append(facts, "no agent working")
	case 1:
		facts = append(facts, "1 agent working")
	default:
		facts = append(facts, fmt.Sprintf("%d agents working", m.Working))
	}
	if m.Plan >= 0 && agent != "" {
		facts = append(facts, fmt.Sprintf("%s at %.0f %%", agent, m.Plan))
	}
	return strings.Join(facts, ", ")
}

// OutWords is why a machine has no room, in words: "GOG: hot at 88 °C".
func OutWords(m Machine) string {
	why := ""
	switch m.Out {
	case OutPlan:
		why = m.Held
	case OutBudget:
		why = m.Budget
	case OutHot:
		why = fmt.Sprintf("hot at %.0f °C", m.TempC)
	case OutBattery:
		why = fmt.Sprintf("on battery at %d %%", m.Battery.Percent)
	case OutLoad:
		why = fmt.Sprintf("load %.1f per core", m.PerCore)
	}
	return m.Label + ": " + why
}

// NoRoom is why no candidate has room, every one's reason in a line.
func NoRoom(r Result) string {
	if len(r.Others) == 0 {
		return "no connected machine has a workspace open on a checkout of this repository"
	}
	var why []string
	for _, m := range r.Others {
		why = append(why, OutWords(m))
	}
	return "no machine has room: " + strings.Join(why, "; ")
}
