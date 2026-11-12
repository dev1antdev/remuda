package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/retire"
)

func TestTheRosterCarriesTheRetirementsAndWhatIsOnTheirWay(t *testing.T) {
	s := serverModeServer(t)
	s.Machines.SetServer(false)
	s.Retire = &retire.Service{
		Settings: func() []retire.Setting {
			return []retire.Setting{{Kind: "schedule", ID: "s1", Machine: machines.LocalID, Agent: "claude", Model: "claude-opus-4-8"}}
		},
		Defaults: func() map[string]retire.Defaults { return map[string]retire.Defaults{machines.LocalID: {Fast: true}} },
	}
	body, _ := s.rosterBody()
	var roster struct {
		Machines []struct {
			ID       string       `json:"id"`
			Retiring []retire.Hit `json:"retiring"`
		} `json:"machines"`
		Retirements []retire.Retirement `json:"retirements"`
	}
	if err := json.Unmarshal(body, &roster); err != nil {
		t.Fatal(err)
	}
	if len(roster.Retirements) != len(retire.Table) || roster.Retirements[0].Model != "claude-opus-4-8" {
		t.Fatalf("the table: %s", body)
	}
	if len(roster.Machines) != 1 || len(roster.Machines[0].Retiring) != 1 || roster.Machines[0].Retiring[0].Kind != "schedule" || roster.Machines[0].Retiring[0].Mode != "fast" {
		t.Fatalf("the machine's hits: %s", body)
	}
	if n := strings.Count(string(body), retire.Table[0].Source); n != 1 {
		t.Fatalf("a hit names its retirement by its key, the table says the rest once: the source %d times", n)
	}
}
