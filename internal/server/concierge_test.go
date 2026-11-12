package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/concierge"
)

func TestTheServersHostIsNeverTheConciergesMachine(t *testing.T) {
	s := serverModeServer(t)
	s.Concierge = concierge.Open(t.TempDir())
	rec := httptest.NewRecorder()
	s.handleConcierge(rec, httptest.NewRequest(http.MethodGet, "/api/concierge", nil))
	var view conciergeView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Machine != "" || len(view.Machines) != 0 || !strings.Contains(view.Trouble, "No connected machine has Claude Code") {
		t.Errorf("no machine of the fleet, nowhere to run: %+v", view)
	}
	put := httptest.NewRecorder()
	s.handleConciergePick(put, httptest.NewRequest(http.MethodPut, "/api/concierge", strings.NewReader(`{"machine":"local"}`)))
	if put.Code != http.StatusBadRequest || s.Concierge.Picked() != "" {
		t.Errorf("the host picked: %d %s", put.Code, put.Body.String())
	}
	start := httptest.NewRecorder()
	s.handleConciergeStart(start, httptest.NewRequest(http.MethodPost, "/api/concierge", strings.NewReader(`{}`)))
	if start.Code != http.StatusConflict || !strings.Contains(start.Body.String(), "Claude Code") {
		t.Errorf("a start with nowhere to run says why: %d %s", start.Code, start.Body.String())
	}
}

func TestAPickOfAMachineTheFleetDoesNotHaveIsRefused(t *testing.T) {
	s := serverModeServer(t)
	s.Machines.SetServer(false)
	s.Concierge = concierge.Open(t.TempDir())
	bad := httptest.NewRecorder()
	s.handleConciergePick(bad, httptest.NewRequest(http.MethodPut, "/api/concierge", strings.NewReader(`{"machine":"nowhere"}`)))
	if bad.Code != http.StatusBadRequest {
		t.Errorf("an unknown machine: %d", bad.Code)
	}
	good := httptest.NewRecorder()
	s.handleConciergePick(good, httptest.NewRequest(http.MethodPut, "/api/concierge", strings.NewReader(`{"machine":"local"}`)))
	var view conciergeView
	if err := json.Unmarshal(good.Body.Bytes(), &view); err != nil || good.Code != 200 || view.Picked != "local" || view.Machine != "local" {
		t.Fatalf("the pick: %d %s", good.Code, good.Body.String())
	}
	if len(view.Machines) != 1 || view.Machines[0].ID != "local" || view.Trouble != "" {
		t.Errorf("a pick whose vitals are not read yet stays in the picker, and a start will tell: %+v", view)
	}
}
