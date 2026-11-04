package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/restart"
	"github.com/dev1antdev/remuda/internal/updates"
	"github.com/dev1antdev/remuda/internal/watch"
)

func TestClosedListsTheAgentsWhosePanesWentAway(t *testing.T) {
	dir := t.TempDir()
	fleet := machines.NewManager(dir, filepath.Join(dir, "no-herdr.sock"), "herdr", nil)
	restarter := restart.New(fleet)
	restarter.Memory = restart.OpenMemory(dir)
	pane := herdr.PaneInfo{PaneID: "w1:p1", WorkspaceID: "w1", Agent: "claude", CWD: "/work/api", Title: "Fix the queue", AgentSession: &herdr.AgentSession{Kind: "id", Value: "9b1c2d3e-4f50-4a6b-8c7d-0e1f2a3b4c5d"}}
	snap := &herdr.Snapshot{Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1", Label: "api"}}, Panes: []herdr.PaneInfo{pane}}
	restarter.Memory.Observe(events.Snapshot{Machine: machines.LocalID, Snap: snap, At: time.Now()})
	restarter.Memory.Observe(events.Snapshot{Machine: machines.LocalID, Snap: &herdr.Snapshot{}, At: time.Now()})

	s := &Server{Machines: fleet, Watch: watch.New(fleet, events.NewBus()), Updates: &updates.Service{Restart: restarter}}
	mux := http.NewServeMux()
	s.closedRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/closed", nil))
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		Closed []restart.Closed `json:"closed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Closed) != 1 || body.Closed[0].Title != "Fix the queue" || body.Closed[0].Workspace != "api" || body.Closed[0].Machine != machines.LocalID {
		t.Fatalf("closed: %+v", body.Closed)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/closed/reopen", strings.NewReader(`{"machine":"nowhere","id":"x"}`)))
	if rec.Code != 404 {
		t.Fatalf("a machine the fleet does not know: %d %s", rec.Code, rec.Body)
	}
}
