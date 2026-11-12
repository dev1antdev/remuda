package server

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/timelapse"
)

// recordedServer is a server whose recorder holds one finished turn of the local pane w1:p1
func recordedServer(t *testing.T) (*Server, *http.ServeMux, timelapse.Meta) {
	t.Helper()
	dir := t.TempDir()
	var reads atomic.Int32
	recorder := timelapse.Open(dir, timelapse.Inputs{Screen: func(machineID, paneID string) (string, error) {
		return fmt.Sprintf("\x1b[32mstep %d\x1b[0m", reads.Add(1)), nil
	}})
	t.Cleanup(recorder.Close)
	s := &Server{Machines: machines.NewManager(dir, filepath.Join(dir, "no-herdr.sock"), "herdr", nil), Timelapse: recorder}
	pane := func(status string) *herdr.Snapshot {
		return &herdr.Snapshot{
			Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1", Label: "app"}},
			Panes:      []herdr.PaneInfo{{PaneID: "w1:p1", WorkspaceID: "w1", Agent: "claude", AgentStatus: status, CWD: "/repo/app"}},
		}
	}
	start := time.Now().Add(-3 * time.Minute)
	// each read lands before the next snapshot: a pane's next read is asked for only once the last one is in
	for i, step := range []struct {
		after  time.Duration
		status string
	}{{0, herdr.StatusWorking}, {time.Minute, herdr.StatusWorking}, {3 * time.Minute, herdr.StatusDone}, {3*time.Minute + timelapse.Grace, herdr.StatusDone}} {
		recorder.Observe(machines.LocalID, pane(step.status), start.Add(step.after))
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && int(reads.Load()) < min(i+1, 3); time.Sleep(5 * time.Millisecond) {
		}
		time.Sleep(20 * time.Millisecond)
	}
	var list []timelapse.Meta
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && len(list) == 0; time.Sleep(5 * time.Millisecond) {
		list = recorder.List(machines.LocalID, "w1:p1", "")
	}
	if len(list) != 1 {
		t.Fatalf("no recording made: %+v", list)
	}
	api := http.NewServeMux()
	s.timelapseRoutes(api)
	return s, api, list[0]
}

func ask(api *http.ServeMux, method, path, body string, header ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	return rec
}

func TestTheRecordingsOfAPaneAreListedAndPlayedGzippedAsTheyAreKept(t *testing.T) {
	_, api, meta := recordedServer(t)
	rec := ask(api, "GET", "/api/timelapse/recordings?machine=local&pane=w1:p1", "")
	var listed struct {
		Recordings []timelapse.Meta `json:"recordings"`
		On         bool             `json:"on"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil || rec.Code != 200 || len(listed.Recordings) != 1 || !listed.On || listed.Recordings[0].Workspace != "app" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if rec := ask(api, "GET", "/api/timelapse/recordings?machine=local&dir=/repo/app", ""); !strings.Contains(rec.Body.String(), meta.ID) {
		t.Fatalf("the folder's: %s", rec.Body)
	}
	if rec := ask(api, "GET", "/api/timelapse/recordings?machine=local", ""); rec.Code != 400 {
		t.Fatalf("neither a pane nor a folder: %d", rec.Code)
	}
	if rec := ask(api, "GET", "/api/timelapse/recordings?machine=nowhere&pane=w1:p1", ""); rec.Code != 404 {
		t.Fatalf("an unknown machine: %d", rec.Code)
	}

	zipped := ask(api, "GET", "/api/timelapse/recordings/"+meta.ID, "", "Accept-Encoding", "gzip, deflate")
	if zipped.Code != 200 || zipped.Header().Get("Content-Encoding") != "gzip" || zipped.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("%d %v", zipped.Code, zipped.Header())
	}
	zr, err := gzip.NewReader(zipped.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(zr)
	var whole timelapse.Recording
	if err := json.Unmarshal(body, &whole); err != nil || whole.ID != meta.ID || len(whole.Frames) != 3 || whole.Frames[0].Rows[0].Text != "\x1b[32mstep 1\x1b[0m" {
		t.Fatalf("%v %+v", err, whole)
	}
	plain := ask(api, "GET", "/api/timelapse/recordings/"+meta.ID, "")
	if plain.Header().Get("Content-Encoding") != "" || !json.Valid(plain.Body.Bytes()) {
		t.Fatalf("a client that takes no gzip: %v", plain.Header())
	}
	if rec := ask(api, "GET", "/api/timelapse/recordings/../../token", ""); rec.Code == 200 {
		t.Fatal("a path read through the id")
	}
}

func TestTheSwitchAndTheDeletionsArePutAndCounted(t *testing.T) {
	_, api, meta := recordedServer(t)
	var stats timelapse.Stats
	json.Unmarshal(ask(api, "GET", "/api/timelapse", "").Body.Bytes(), &stats)
	if !stats.On || stats.Count != 1 || stats.Bytes <= 0 || stats.Max != timelapse.MaxTotal {
		t.Fatalf("stats %+v", stats)
	}
	if rec := ask(api, "PUT", "/api/timelapse", `{}`); rec.Code != 400 {
		t.Fatalf("no switch: %d", rec.Code)
	}
	if rec := ask(api, "PUT", "/api/timelapse", `{"on":false}`); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"on":false`) {
		t.Fatalf("off: %d %s", rec.Code, rec.Body)
	}
	if rec := ask(api, "DELETE", "/api/timelapse/recordings/"+meta.ID, ""); rec.Code != 200 {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if rec := ask(api, "GET", "/api/timelapse/recordings/"+meta.ID, ""); rec.Code != 404 {
		t.Fatalf("a deleted recording played: %d", rec.Code)
	}
	if rec := ask(api, "DELETE", "/api/timelapse/recordings/"+meta.ID, ""); rec.Code != 404 {
		t.Fatalf("deleted twice: %d", rec.Code)
	}
	if rec := ask(api, "DELETE", "/api/timelapse", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"deleted":0`) {
		t.Fatalf("delete everything: %d %s", rec.Code, rec.Body)
	}
}
