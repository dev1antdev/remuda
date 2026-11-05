package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/backup"
	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/jira"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/watch"
)

// serverModeServer is a remuda whose host serves the page only: no machine of the roster, its vitals in roster.server
func serverModeServer(t *testing.T) *Server {
	dir := t.TempDir()
	fleet := machines.NewManager(dir, filepath.Join(dir, "no-herdr.sock"), "herdr", nil)
	fleet.SetServer(true)
	return &Server{Machines: fleet, Watch: watch.New(fleet, events.NewBus()), Health: health.New("", fleet, "v-test", false), Jira: jira.New(dir)}
}

func TestAServersRosterHasNoHostMachineButItsServer(t *testing.T) {
	s := serverModeServer(t)
	body, _ := s.rosterBody()
	var roster struct {
		Machines []struct {
			ID string `json:"id"`
		} `json:"machines"`
		Server *struct {
			ID    string `json:"id"`
			Label string `json:"label"`
		} `json:"server"`
	}
	if err := json.Unmarshal(body, &roster); err != nil {
		t.Fatal(err)
	}
	if len(roster.Machines) != 0 {
		t.Fatalf("the host stands in the roster's machines: %+v", roster.Machines)
	}
	if roster.Server == nil || roster.Server.ID != machines.LocalID || roster.Server.Label == "" {
		t.Fatalf("no server entry: %s", body)
	}
	s.Machines.SetServer(false)
	body, _ = s.rosterBody()
	roster.Server = nil
	if err := json.Unmarshal(body, &roster); err != nil || len(roster.Machines) != 1 || roster.Server != nil {
		t.Fatalf("a cockpit's roster: %s", body)
	}
}

func TestAServersHealthSaysItIsOne(t *testing.T) {
	s := serverModeServer(t)
	rec := httptest.NewRecorder()
	s.handleHealth(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["ok"] != true || got["server"] != true || got["herdr"] != false || got["version"] == nil {
		t.Fatalf("health %v", got)
	}
	r, peer := health.Parse(`peer={"ok":true,"server":true,"version":"v1"}` + "\npeer_token=t\n")
	if r.Running || peer != nil {
		t.Fatal("another cockpit would relay terminals through a server, which has no herdr")
	}
}

func TestAServerIsNoTargetForItsOwnBackup(t *testing.T) {
	s := serverModeServer(t)
	s.Backup = backup.Open(t.TempDir(), "pi", "v-test")
	put := httptest.NewRecorder()
	s.handleBackupsPut(put, httptest.NewRequest(http.MethodPut, "/api/backups", strings.NewReader(`{"machine":"local","at":"03:30","keep":14}`)))
	if put.Code != http.StatusBadRequest || !strings.Contains(put.Body.String(), "pick one of its machines") {
		t.Fatalf("the host as its own backup's target: %d %s", put.Code, put.Body.String())
	}
	off := httptest.NewRecorder()
	s.handleBackupsPut(off, httptest.NewRequest(http.MethodPut, "/api/backups", strings.NewReader(`{"machine":"","at":"03:30","keep":14}`)))
	if off.Code != http.StatusOK {
		t.Fatalf("no backups: %d %s", off.Code, off.Body.String())
	}
}
