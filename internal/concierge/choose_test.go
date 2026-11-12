package concierge

import (
	"strings"
	"testing"
)

func fleet() []Candidate {
	return []Candidate{
		{ID: "local", Label: "This machine · pi", Local: true, Connected: true, Read: true, Remuda: true}, // no Claude Code
		{ID: "gog", Label: "GOG", Host: "gog", Connected: true, Read: true, Claude: true, Remuda: true},   // Linux
		{ID: "mac", Label: "This Mac", Host: "mac", Connected: true, Read: true, Claude: true, Remuda: true, Mac: true},
		{ID: "away", Label: "Laptop", Host: "laptop", Read: true, Claude: true, Remuda: true, Mac: true}, // away
		{ID: "bare", Label: "Roofr", Host: "roofr", Connected: true, Read: true, Claude: true},           // no remuda
		{ID: "fresh", Label: "Fresh", Host: "fresh", Connected: true},                                    // vitals not read yet
	}
}

func TestTheDefaultIsAConnectedMacWithClaudeCodeElseTheFirstMachineWithIt(t *testing.T) {
	machines := fleet()
	if got := Default(machines); got != "mac" {
		t.Errorf("the Mac first, an away one passed over: %q", got)
	}
	noMac := []Candidate{machines[0], machines[1], machines[3]}
	if got := Default(noMac); got != "gog" {
		t.Errorf("else the first connected machine with Claude Code: %q", got)
	}
	if got := Default([]Candidate{machines[0], machines[3], machines[5]}); got != "" {
		t.Errorf("none can host it: %q", got)
	}
	hosts := Hosts(machines)
	if len(hosts) != 3 || hosts[0].ID != "gog" || hosts[1].ID != "mac" || hosts[2].ID != "bare" {
		t.Errorf("the picker's machines, in the fleet's order: %+v", hosts)
	}
}

func TestThePersonsPickStandsWhileTheFleetHasIt(t *testing.T) {
	machines := fleet()
	if got := Chosen("gog", machines); got != "gog" {
		t.Errorf("the pick: %q", got)
	}
	if got := Chosen("away", machines); got != "away" {
		t.Errorf("a pick that is away stays the pick: %q", got)
	}
	if got := Chosen("removed", machines); got != "mac" {
		t.Errorf("a machine the fleet no longer has falls back to the default: %q", got)
	}
	if got := Chosen("", machines); got != "mac" {
		t.Errorf("no pick is the default: %q", got)
	}
}

func TestTroubleSaysWhyAndWhatToDo(t *testing.T) {
	machines := fleet()
	for id, want := range map[string]string{
		"":      "No connected machine has Claude Code",
		"away":  "Laptop is away",
		"local": "Claude Code is not installed on This machine · pi",
		"bare":  "remuda setup -host roofr",
	} {
		if got := Trouble(id, machines); !strings.Contains(got, want) {
			t.Errorf("%q: %q, want it to say %q", id, got, want)
		}
	}
	for _, id := range []string{"mac", "gog", "fresh"} {
		if got := Trouble(id, machines); got != "" {
			t.Errorf("%s can host it (or a start will tell): %q", id, got)
		}
	}
	local := []Candidate{{ID: "local", Label: "This Mac", Local: true, Connected: true, Read: true, Claude: true}}
	if got := Trouble("local", local); got != "" {
		t.Errorf("the cockpit's own machine runs this very remuda: %q", got)
	}
}
