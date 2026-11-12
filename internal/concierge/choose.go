package concierge

import (
	"fmt"
	"strings"
)

// Candidate is a machine of the fleet as the choice of where the concierge runs sees it. The host of a remuda serving
// the page only (-server) is no machine of the fleet, so it is never one.
type Candidate struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Host      string `json:"-"`
	Local     bool   `json:"-"`
	Connected bool   `json:"connected"`
	Read      bool   `json:"-"` // its vitals were read: what it has installed is known
	Claude    bool   `json:"-"`
	Remuda    bool   `json:"-"`
	Mac       bool   `json:"mac,omitempty"`
}

// Hosts is the machines the concierge can run on now, in the fleet's order: connected, with Claude Code.
func Hosts(machines []Candidate) []Candidate {
	var out []Candidate
	for _, c := range machines {
		if c.Connected && c.Claude {
			out = append(out, c)
		}
	}
	return out
}

// Default is where the concierge runs when the person picked nothing: the first Mac it can run on, else the first
// machine it can run on, "" when there is none.
func Default(machines []Candidate) string {
	hosts := Hosts(machines)
	for _, c := range hosts {
		if c.Mac {
			return c.ID
		}
	}
	if len(hosts) > 0 {
		return hosts[0].ID
	}
	return ""
}

// Chosen is where the concierge runs: the person's pick while the fleet has that machine, away or not, else the default.
func Chosen(picked string, machines []Candidate) string {
	for _, c := range machines {
		if picked != "" && c.ID == picked {
			return picked
		}
	}
	return Default(machines)
}

// Trouble is why the concierge cannot run on the machine chosen now, with what to do about it; "" when it can, or
// when its vitals are not read yet and a start will tell.
func Trouble(chosen string, machines []Candidate) string {
	if chosen == "" {
		return "No connected machine has Claude Code, and the concierge is a Claude Code agent: install Claude Code on one of your machines (its vitals tell remuda a minute after it connects)."
	}
	var c Candidate
	for _, m := range machines {
		if m.ID == chosen {
			c = m
		}
	}
	switch {
	case !c.Connected:
		return fmt.Sprintf("%s is away: wake it, or pick another machine.", c.Label)
	case !c.Read:
		return ""
	case !c.Claude:
		return fmt.Sprintf("Claude Code is not installed on %s: install it there, or pick another machine.", c.Label)
	case !c.Local && !c.Remuda:
		return fmt.Sprintf("remuda is not installed on %s, and the concierge reads your agents and tasks through it: run `remuda setup -host %s` from a machine that has it, or pick another machine.", c.Label, hostOf(c))
	}
	return ""
}

func hostOf(c Candidate) string {
	if c.Host != "" {
		return c.Host
	}
	return strings.ToLower(c.Label)
}
