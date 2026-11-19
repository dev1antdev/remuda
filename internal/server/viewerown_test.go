package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/auth"
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
	// herdr names a folder as the system resolves it
	appDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	shellDir, err := filepath.EvalSymlinks(t.TempDir())
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
	const closedShellSession = "33333333-3333-4333-8333-333333333333"
	elsewhere := filepath.Join(home, "private-project") // where the floating terminal stood, no workspace there now
	projects := filepath.Join(home, ".claude", "projects")
	for session, folder := range map[string]string{conciergeSession: filepath.Join(mc.StateDir(), "concierge"), appSession: appDir, closedShellSession: elsewhere} {
		dir := filepath.Join(projects, transcript.EncodePath(folder))
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, session+".jsonl"), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const codexSession = "44444444-4444-4444-8444-444444444444"
	rollouts := filepath.Join(home, ".codex", "sessions", "2026", "11", "19")
	if err := os.MkdirAll(rollouts, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rollouts, "rollout-2026-11-19T00-00-00-"+codexSession+".jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"/api/chat", "/api/recap"} {
		if code, _ := fileCall(passes, route+"?machine=local&dir="+url.QueryEscape(appDir)+"&session="+codexSession, true); code != http.StatusNotFound {
			t.Errorf("%s: a Codex session nothing ties to the folder: %d", route, code)
		}
		if code, _ := fileCall(passes, route+"?machine=local&dir="+url.QueryEscape(appDir)+"&session="+conciergeSession, true); code != http.StatusNotFound {
			t.Errorf("%s: the concierge's session asked for through another folder: %d", route, code)
		}
		if code, _ := fileCall(passes, route+"?machine=local&dir="+url.QueryEscape(filepath.Join(mc.StateDir(), "concierge"))+"&session="+appSession, true); code != http.StatusNotFound {
			t.Errorf("%s: the concierge's folder: %d", route, code)
		}
		if code, _ := fileCall(passes, route+"?machine=local&dir="+url.QueryEscape(appDir)+"&session="+closedShellSession, true); code != http.StatusNotFound {
			t.Errorf("%s: a conversation of a folder no workspace stands in: %d", route, code)
		}
		if code, _ := fileCall(passes, route+"?machine=local&dir="+url.QueryEscape(elsewhere)+"&session="+closedShellSession, true); code != http.StatusNotFound {
			t.Errorf("%s: a folder no workspace stands in: %d", route, code)
		}
		if code, _ := fileCall(passes, route+"?machine=local&dir="+url.QueryEscape(appDir)+"&session="+appSession, true); code != 200 {
			t.Errorf("%s: a session of the person's folder: %d", route, code)
		}
	}

	s.noteSent(context.Background(), machines.LocalID, ownPane, "the-secret", false)
	s.noteSent(context.Background(), machines.LocalID, appPane, "hello", false)
	s.Feed.Add(sent.Entry("Phone", "unmarked-secret, its pane gone", false, machines.LocalID, "w99:p9", ""))
	s.Feed.Add(feed.Entry{Kind: "asked", Machine: machines.LocalID, Pane: "w96:p1", Title: "unmarked-secret alert, its pane gone"})
	s.Feed.Add(feed.Entry{Kind: "done", Machine: machines.LocalID, Pane: "w98:p1", Title: "own-secret, its pane gone", Own: new(true)})
	s.Feed.Add(feed.Entry{Kind: "done", Machine: machines.LocalID, Pane: "w97:p1", Title: "finished, its pane closed since", Own: new(false)})
	_, body := fileCall(h, "/api/feed", true)
	if strings.Contains(body, "secret") || !strings.Contains(body, "hello") || !strings.Contains(body, "finished") {
		t.Errorf("a viewer's activity: %s", body)
	}
	if all := s.Feed.List(0); len(all) != 6 {
		t.Errorf("the person's activity lost rows: %+v", all)
	}

	// ended, and still on the watcher's last snapshot
	if err := s.Scratch.Close(machines.LocalID, client, nil); err != nil {
		t.Fatal(err)
	}
	if code, _ := fileCall(h, "/api/screen?machine=local&pane="+url.QueryEscape(ownPane), true); code != http.StatusNotFound {
		t.Errorf("the floating terminal just ended: %d", code)
	}
	if ended, _ := s.rosterFor(true); strings.Contains(string(ended), ownPane) {
		t.Errorf("a viewer's roster shows the floating terminal just ended: %s", ended)
	}
	if s.ownFolder(machines.LocalID, shellDir) != true {
		t.Error("the folder of a floating terminal just ended counts as the person's")
	}
	reach, _ := s.viewerReachOn(auth.WithViewer(context.Background()), mc)
	if reach.Allows(filepath.Join(shellDir, "notes.txt")) {
		t.Error("a viewer reaches into the folder of a floating terminal just ended")
	}
}

func TestAViewerGetsNoRecordingOfRemudasOwnWorkspaces(t *testing.T) {
	for _, label := range []string{scratch.Label, concierge.Label, "renamed by hand"} {
		_, api, meta := recordedIn(t, label, true)
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
	s, _, meta := recordedIn(t, "renamed long ago", false)
	meta.Own, meta.Dir = nil, filepath.Join(s.Machines.All()[0].StateDir(), "concierge")
	if !s.ownRecording(meta) {
		t.Error("an unmarked recording made in a state dir")
	}
	meta.Machine, meta.Dir = "a machine removed since", "/anywhere"
	if !s.ownRecording(meta) {
		t.Error("an unmarked recording of a machine that can no longer be asked")
	}
	for _, label := range []string{"app", scratch.Label} { // marked as the person's, whatever the workspace is called
		_, api, meta := recordedIn(t, label, false)
		if code, _ := fileCall(api, "/api/timelapse/recordings/"+meta.ID, true); code != 200 {
			t.Errorf("a recording of the person's workspace %s: %d", label, code)
		}
	}
}
