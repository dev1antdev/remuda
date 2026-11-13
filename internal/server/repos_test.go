package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/gitsync"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/watch"
)

func reposServer(t *testing.T, answer func(string) (string, error)) *Server {
	t.Helper()
	dir := t.TempDir()
	fleet := machines.NewManager(dir, filepath.Join(dir, "no-herdr.sock"), "herdr", nil)
	host := &shell.Fake{Name: machines.LocalID, Dir: "/home/me", Answer: answer}
	return &Server{Machines: fleet, Watch: watch.New(fleet, events.NewBus()), Repos: &gitsync.Service{
		Fleet: func() []gitsync.Machine { return []gitsync.Machine{{Host: host}} },
		Host:  func(string) (shell.Host, error) { return host, nil },
		Scans: gitsync.OpenScans(dir),
	}}
}

func TestTheRepositoryRoutesNameTheirMachineAndFolder(t *testing.T) {
	s := reposServer(t, func(script string) (string, error) {
		if strings.Contains(script, "missing=1") {
			return "missing=1\n", nil
		}
		return "", nil
	})
	h := http.NewServeMux()
	s.repoRoutes(h)
	call := func(method, path, body string) (int, string) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec.Code, rec.Body.String()
	}
	if code, body := call("POST", "/api/repos/pull", `{"machine":"local","dir":"code/app"}`); code != 400 {
		t.Fatalf("a relative folder: %d %s", code, body)
	}
	if code, body := call("POST", "/api/repos/fetch", `{"machine":"gog","dir":"/code/app"}`); code != 404 {
		t.Fatalf("a machine the fleet lacks: %d %s", code, body)
	}
	code, body := call("POST", "/api/repos/push", `{"machine":"local","dir":"/code/app"}`)
	var outcome gitsync.Outcome
	if code != 200 || json.Unmarshal([]byte(body), &outcome) != nil || outcome.Action != gitsync.ActPush || outcome.Reason != gitsync.ReasonMissing {
		t.Fatalf("a push in a folder that is not there: %d %s", code, body)
	}
	if code, body := call("PUT", "/api/repos/scan", `{"machine":"local","folder":"~/code","depth":9}`); code != 400 || !strings.Contains(body, "1 to 5") {
		t.Fatalf("a scan too deep: %d %s", code, body)
	}
	if code, body := call("PUT", "/api/repos/scan", `{"machine":"local","folder":"/srv/code/","depth":2}`); code != 200 || !strings.Contains(body, `"/srv/code"`) {
		t.Fatalf("a scan: %d %s", code, body)
	}
	code, body = call("GET", "/api/repos", "")
	var listed struct {
		Machines []gitsync.Listed `json:"machines"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &listed) != nil || len(listed.Machines) != 1 || listed.Machines[0].Scan == nil || listed.Machines[0].Repos == nil {
		t.Fatalf("the list: %d %s", code, body)
	}
}
