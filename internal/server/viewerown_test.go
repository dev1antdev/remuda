package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/concierge"
	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/feed"
	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/herdr/herdrtest"
	"github.com/dev1antdev/remuda/internal/jira"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/scratch"
	"github.com/dev1antdev/remuda/internal/sent"
	"github.com/dev1antdev/remuda/internal/transcript"
	"github.com/dev1antdev/remuda/internal/watch"
)

func TestAViewerNeverReachesRemudasOwnWorkspaces(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // the transcripts below are this test's own
	sock := herdrtest.Server(t)
	home, err := os.UserHomeDir() // the throwaway herdr may have set one of its own
	if err != nil {
		t.Fatal(err)
	}
	client := herdr.NewClient(sock)
	appDir := t.TempDir()
	shellDir, err := filepath.EvalSymlinks(t.TempDir()) // herdr names the folder as the system resolves it
	if err != nil {
		t.Fatal(err)
	}
	app, err := client.WorkspaceCreate(appDir, "app")
	if err != nil {
		t.Fatal(err)
	}
	shell, err := client.WorkspaceCreate(shellDir, scratch.Label)
	if err != nil {
		t.Fatal(err)
	}
	ownPane, appPane := shell.RootPane.PaneID, app.RootPane.PaneID

	state := t.TempDir()
	kept, _ := json.Marshal(map[string]string{machines.LocalID: shell.Workspace.WorkspaceID})
	if err := os.WriteFile(filepath.Join(state, "scratch.json"), kept, 0o600); err != nil {
		t.Fatal(err)
	}
	fleet := machines.NewManager(state, sock, "herdr", nil)
	mc, err := fleet.Get(machines.LocalID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := mc.Ready(); err != nil {
		t.Fatal(err)
	}
	watcher := watch.New(fleet, events.NewBus())
	if _, err := watcher.Refresh(mc); err != nil {
		t.Fatal(err)
	}
	s := &Server{Machines: fleet, Watch: watcher, Scratch: scratch.Open(state), Concierge: concierge.Open(t.TempDir()),
		Health: health.New("", fleet, "v-test", false), Jira: jira.New(state), Feed: feed.New(state), Review: review.New(state)}
	mux := http.NewServeMux()
	s.screenRoutes(mux)
	s.questionRoutes(mux)
	s.feedRoutes(mux)
	h := s.viewerOwnGuard(mux)

	for _, route := range []string{"/api/screen", "/api/scrollback", "/api/question"} {
		path := route + "?machine=local&pane="
		if code, body := fileCall(h, path+url.QueryEscape(ownPane), true); code != http.StatusNotFound {
			t.Errorf("%s of the floating terminal: %d %s", route, code, body)
		}
		if code, body := fileCall(h, path+"w99:p9", true); code != http.StatusNotFound {
			t.Errorf("%s of a pane the roster never showed: %d %s", route, code, body)
		}
		if code, body := fileCall(h, path+url.QueryEscape(appPane), true); code != 200 {
			t.Errorf("%s of a workspace of the person's: %d %s", route, code, body)
		}
		if code, body := fileCall(h, path+url.QueryEscape(ownPane), false); code != 200 {
			t.Errorf("%s for the person: %d %s", route, code, body)
		}
	}
	for _, dir := range []string{shellDir, shellDir + "/"} {
		if code, _ := fileCall(h, "/api/screen?machine=local&dir="+url.QueryEscape(dir)+"&pane="+url.QueryEscape(appPane), true); code != http.StatusNotFound {
			t.Errorf("the floating terminal's folder %s: %d", dir, code)
		}
	}

	person, personTag := s.rosterFor(false)
	viewer, viewerTag := s.rosterFor(true)
	if !strings.Contains(string(person), shell.Workspace.WorkspaceID) {
		t.Fatalf("the person's roster lost the floating terminal: %s", person)
	}
	if strings.Contains(string(viewer), `"id":"`+shell.Workspace.WorkspaceID+`"`) || strings.Contains(string(viewer), ownPane) {
		t.Errorf("a viewer's roster shows the floating terminal: %s", viewer)
	}
	if !strings.Contains(string(viewer), app.Workspace.WorkspaceID) {
		t.Errorf("a viewer's roster lost the person's workspace: %s", viewer)
	}
	if personTag == viewerTag {
		t.Error("the two rosters share an ETag: one would be answered 304 with the other's")
	}

	passes := s.viewerOwnGuard(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	if code, _ := fileCall(passes, "/?machine=local&pane=w99:p9", true); code != 200 {
		t.Errorf("the page's own address, naming a pane gone since: %d", code)
	}
	const conciergeSession, appSession = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	projects := filepath.Join(home, ".claude", "projects")
	for session, folder := range map[string]string{conciergeSession: filepath.Join(mc.StateDir(), "concierge"), appSession: appDir} {
		dir := filepath.Join(projects, transcript.EncodePath(folder))
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, session+".jsonl"), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, route := range []string{"/api/chat", "/api/recap"} {
		if code, _ := fileCall(passes, route+"?machine=local&dir="+url.QueryEscape(appDir)+"&session="+conciergeSession, true); code != http.StatusNotFound {
			t.Errorf("%s: the concierge's session asked for through another folder: %d", route, code)
		}
		if code, _ := fileCall(passes, route+"?machine=local&dir="+url.QueryEscape(filepath.Join(mc.StateDir(), "concierge"))+"&session="+appSession, true); code != http.StatusNotFound {
			t.Errorf("%s: the concierge's folder: %d", route, code)
		}
		if code, _ := fileCall(passes, route+"?machine=local&dir="+url.QueryEscape(appDir)+"&session="+appSession, true); code != 200 {
			t.Errorf("%s: a session of the person's folder: %d", route, code)
		}
	}

	s.Feed.Add(sent.Entry("Phone", "the-secret", false, machines.LocalID, ownPane, scratch.Label))
	s.Feed.Add(sent.Entry("Phone", "gone-pane-secret", false, machines.LocalID, "w99:p9", ""))
	s.Feed.Add(sent.Entry("Phone", "hello", false, machines.LocalID, appPane, "app"))
	_, body := fileCall(h, "/api/feed", true)
	if strings.Contains(body, "secret") || !strings.Contains(body, "hello") {
		t.Errorf("a viewer's activity: %s", body)
	}
	if all := s.Feed.List(0); len(all) != 3 {
		t.Errorf("the person's activity lost rows: %+v", all)
	}
}

func TestAViewerGetsNoRecordingOfRemudasOwnWorkspaces(t *testing.T) {
	for _, label := range []string{scratch.Label, concierge.Label} {
		_, api, meta := recordedIn(t, label)
		for _, path := range []string{"/api/timelapse/recordings?machine=local&pane=w1:p1", "/api/timelapse/search?q=step"} {
			if rec := ask(api, "GET", path, ""); !strings.Contains(rec.Body.String(), meta.ID) {
				t.Fatalf("%s for the person: %s", path, rec.Body)
			}
			if code, body := fileCall(api, path, true); code != 200 || strings.Contains(body, meta.ID) {
				t.Errorf("%s of %s for a viewer: %d %s", path, label, code, body)
			}
		}
		if code, _ := fileCall(api, "/api/timelapse/recordings/"+meta.ID, true); code != http.StatusNotFound {
			t.Errorf("%s's recording by its id: %d", label, code)
		}
	}
	_, api, meta := recordedServer(t)
	if code, _ := fileCall(api, "/api/timelapse/recordings/"+meta.ID, true); code != 200 {
		t.Errorf("a recording of the person's workspace: %d", code)
	}
}
