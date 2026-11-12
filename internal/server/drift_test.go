package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dev1antdev/remuda/internal/drift"
	"github.com/dev1antdev/remuda/internal/jobs"
	"github.com/dev1antdev/remuda/internal/restart"
	"github.com/dev1antdev/remuda/internal/updates"
)

func TestFleetSetupHasNoColumnForAServersHost(t *testing.T) {
	s := serverModeServer(t)
	s.Drift = &drift.Service{Fleet: s.Machines, Health: s.Health, Updates: &updates.Service{Health: s.Health, Jobs: jobs.New(nil), Restart: restart.New(s.Machines), Watch: s.Watch, Machines: s.Machines}}
	read := func() drift.Table {
		rec := httptest.NewRecorder()
		s.handleFleetSetup(rec, httptest.NewRequest(http.MethodGet, "/api/fleet-setup", nil))
		var table drift.Table
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &table) != nil {
			t.Fatalf("fleet setup: %d %s", rec.Code, rec.Body.String())
		}
		return table
	}
	if table := read(); len(table.Machines) != 0 || table.Sections == nil {
		t.Fatalf("the host of a remuda serving the page only is a column: %+v", table)
	}
	s.Machines.SetServer(false)
	if table := read(); len(table.Machines) != 1 || table.Machines[0].ID != "local" {
		t.Fatalf("a cockpit's own machine is no column: %+v", table)
	}
}
