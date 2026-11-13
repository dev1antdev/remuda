package server

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestEveryMachineOnTheRosterSaysItsSystem is the page's contract: os is there for every machine, "" while unknown.
func TestEveryMachineOnTheRosterSaysItsSystem(t *testing.T) {
	for system, want := range map[string]string{"windows": `"os":"windows"`, "": `"os":""`} {
		raw, err := json.Marshal(rosterMachine{ID: "pc", OS: system, Sessions: []rosterSession{}})
		if err != nil || !strings.Contains(string(raw), want) {
			t.Errorf("a machine with os %q marshals as %s (%v)", system, raw, err)
		}
	}
}
