package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/herdr/herdrtest"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/radar"
	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/sleep"
	"github.com/dev1antdev/remuda/internal/usage"
	"github.com/dev1antdev/remuda/internal/watch"
)

// TestSleepRefusesWhatHasNothingToSleep runs the routes against a throwaway herdr: a workspace of shells has nothing
// to put to sleep, an awake one nothing to wake, and the switches are kept. No agent is started: the sleep and the
// wake themselves are tested on fakes.
func TestSleepRefusesWhatHasNothingToSleep(t *testing.T) {
	sock := herdrtest.Server(t)
	client := herdr.NewClient(sock)
	dir, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	made, err := client.WorkspaceCreate(dir, "shells only")
	if err != nil {
		t.Fatal(err)
	}
	defer client.WorkspaceClose(made.Workspace.WorkspaceID)
	workspace := made.Workspace.WorkspaceID

	fleet := machines.NewManager(t.TempDir(), sock, "herdr", nil)
	watcher := watch.New(fleet, events.NewBus())
	s := &Server{Machines: fleet, Watch: watcher, Input: sessions.NewInput(fleet, watcher), Sleep: &sleep.Service{Store: sleep.Open(t.TempDir())}}
	mux := http.NewServeMux()
	s.sleepRoutes(mux)
	call := func(method, path, body string) (int, string) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec.Code, rec.Body.String()
	}
	if code, body := call("POST", "/api/sessions/"+workspace+"/sleep", `{"machine":"local"}`); code != 409 || !strings.Contains(body, "nothing to put to sleep in shells only: no agent runs there") {
		t.Fatalf("sleep of shells: %d %s", code, body)
	}
	if code, body := call("POST", "/api/sessions/w999/sleep", `{"machine":"local"}`); code != 404 {
		t.Fatalf("sleep of no workspace: %d %s", code, body)
	}
	if code, body := call("POST", "/api/sessions/"+workspace+"/wake", `{"machine":"local"}`); code != 400 || !strings.Contains(body, "not asleep") {
		t.Fatalf("wake of an awake workspace: %d %s", code, body)
	}
	if code, body := call("POST", "/api/sessions/"+workspace+"/sleep", `{"machine":"nowhere"}`); code != 404 {
		t.Fatalf("sleep on a machine the fleet lacks: %d %s", code, body)
	}
	if code, body := call("GET", "/api/sleep/settings", ""); code != 200 || body != `{"wake_on_open":false,"resume_automatically":false}`+"\n" {
		t.Fatalf("the switches start off: %d %s", code, body)
	}
	if code, body := call("PUT", "/api/sleep/settings", `{"wake_on_open":true,"resume_automatically":true}`); code != 200 || !strings.Contains(body, `"resume_automatically":true`) {
		t.Fatalf("switched on: %d %s", code, body)
	}
}

// TestTheRosterSaysAWorkspaceIsAsleep: the record a sleep keeps is the roster's session.asleep, nothing for the rest.
func TestTheRosterSaysAWorkspaceIsAsleep(t *testing.T) {
	kept := t.TempDir()
	record := map[string]map[string]sleep.Workspace{"local": {"w1": {Name: "api", Dir: "/work/api", Agents: []sleep.Agent{{Pane: "w1:p1", Kind: "claude", Session: "9b1c2d3e-4f50-4a6b-8c7d-0e1f2a3b4c5d", Dir: "/work/api"}}}}}
	raw, _ := json.Marshal(record)
	if err := os.WriteFile(filepath.Join(kept, "asleep.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	snap := &herdr.Snapshot{
		Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1", Label: "api"}, {WorkspaceID: "w2", Label: "web"}},
		Tabs:       []herdr.TabInfo{{TabID: "w1:t1", WorkspaceID: "w1", Number: 1}, {TabID: "w2:t1", WorkspaceID: "w2", Number: 1}},
		Panes:      []herdr.PaneInfo{{PaneID: "w1:p1", TabID: "w1:t1", WorkspaceID: "w1", CWD: "/work/api"}, {PaneID: "w2:p1", TabID: "w2:t1", WorkspaceID: "w2", CWD: "/work/web"}},
	}
	s := &Server{Radar: radar.New(t.TempDir(), nil), Usage: usage.NewTracker(), Review: review.New(t.TempDir()), Sleep: &sleep.Service{Store: sleep.Open(kept)}}
	sessions := s.sessionsFromSnapshot("local", snap)
	if sessions[0].Asleep == nil || sessions[0].Asleep.Agents != 1 || sessions[1].Asleep != nil {
		t.Fatalf("asleep: %+v, %+v", sessions[0].Asleep, sessions[1].Asleep)
	}
}
