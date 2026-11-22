package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/chat"
	"github.com/dev1antdev/remuda/internal/core/coretest"
	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/feed"
	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/jira"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/recap"
	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/scratch"
	"github.com/dev1antdev/remuda/internal/sent"
	"github.com/dev1antdev/remuda/internal/transcript"
	"github.com/dev1antdev/remuda/internal/usage"
	"github.com/dev1antdev/remuda/internal/watch"
)

func TestAViewerNeverReachesRemudasOwnWorkspaces(t *testing.T) {
	coretest.Server(t).Use(t)
	home, err := os.UserHomeDir() // the throwaway core's own: the transcript below is this test's
	if err != nil {
		t.Fatal(err)
	}
	state := t.TempDir()
	fleet := machines.NewManager(state, nil, false)
	t.Cleanup(fleet.Close)
	mc, err := fleet.Get(machines.LocalID)
	if err != nil {
		t.Fatal(err)
	}
	client, _, err := mc.Ready()
	if err != nil {
		t.Fatal(err)
	}
	// a pane's folder is the one the system resolves
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
	coreSays(t, client, func(snap *herdr.Snapshot) bool { return snap.Pane(ownPane) != nil && snap.Pane(appPane) != nil })

	kept, _ := json.Marshal(map[string]string{machines.LocalID: shell.Workspace.WorkspaceID})
	if err := os.WriteFile(filepath.Join(state, "scratch.json"), kept, 0o600); err != nil {
		t.Fatal(err)
	}
	watcher := watch.New(fleet, events.NewBus())
	if _, err := watcher.Refresh(mc); err != nil {
		t.Fatal(err)
	}
	s := &Server{Machines: fleet, Watch: watcher, Scratch: scratch.Open(state),
		Health: health.New("", fleet, "v-test", false), Jira: jira.New(state), Feed: feed.New(state), Review: review.New(state)}
	mux := http.NewServeMux()
	s.screenRoutes(mux)
	s.questionRoutes(mux)
	s.feedRoutes(mux)
	s.terminalRoutes(mux)
	h := s.viewerOwnGuard(mux)

	// a link watches a live terminal of its own — every machine is on remuda-core now — but never the floating terminal's
	if code, body := fileCall(h, "/ws/pane?machine=local&id="+url.QueryEscape(appPane), true); code != http.StatusUpgradeRequired {
		t.Errorf("a viewer's live terminal, asked without an upgrade: %d %s", code, body)
	}
	if code, body := fileCall(h, "/ws/pane?machine=local&id="+url.QueryEscape(ownPane), true); code != http.StatusNotFound {
		t.Errorf("a viewer's live terminal of the floating terminal: %d %s", code, body)
	}

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

	var asked url.Values
	passes := s.viewerOwnGuard(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { asked = r.URL.Query() }))
	if code, _ := fileCall(passes, "/?machine=local&pane=w99:p9", true); code != 200 {
		t.Errorf("the page's own address, naming a pane gone since: %d", code)
	}
	const session = "11111111-1111-4111-8111-111111111111"
	for _, route := range []string{"/api/chat", "/api/recap"} {
		for _, query := range []string{"dir=" + url.QueryEscape(appDir) + "&session=" + session, "pane=" + url.QueryEscape(appPane) + "&session=" + session} {
			if code, _ := fileCall(passes, route+"?machine=local&"+query, true); code != http.StatusNotFound {
				t.Errorf("%s?%s: a session no pane on the roster is in: %d", route, query, code)
			}
			if code, _ := fileCall(passes, route+"?machine=local&"+query, false); code != 200 || asked.Get("session") != session {
				t.Errorf("%s?%s: the person's: %d %v", route, query, code, asked)
			}
		}
		if code, _ := fileCall(passes, route+"?machine=local&pane="+url.QueryEscape(appPane), true); code != 200 {
			t.Errorf("%s: a pane's own conversation: %d", route, code)
		}
	}
	project := filepath.Join(home, ".claude", "projects", transcript.EncodePath(appDir))
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	record := `{"type":"user","uuid":"u1","sessionId":"` + session + `","timestamp":"2026-11-19T00:00:00Z","cwd":` + strconv.Quote(appDir) + `,"message":{"role":"user","content":"the-other-conversation"}}` + "\n"
	if err := os.WriteFile(filepath.Join(project, session+".jsonl"), []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}
	s.Chat = &chat.Service{Machines: fleet, Watch: watcher}
	s.Recap = &recap.Service{Chat: s.Chat}
	reads := http.NewServeMux()
	s.chatRoutes(reads)
	s.recapRoutes(reads)
	// the shell reports no session: the person gets the folder's newest, a viewer nothing
	for route, read := range map[string]string{"/api/chat": "the-other-conversation", "/api/recap": `"prompts":1`} {
		path := route + "?machine=local&look=0&pane=" + url.QueryEscape(appPane)
		if code, body := fileCall(reads, path, false); code != 200 || !strings.Contains(body, read) {
			t.Errorf("%s for the person: %d %s", route, code, body)
		}
		code, body := fileCall(reads, path, true)
		if code != 200 || strings.Contains(body, read) || (route == "/api/recap" && !strings.Contains(body, `"recap":`)) {
			t.Errorf("%s of a pane with no session: %d %s", route, code, body)
		}
	}
	report := &usage.Report{Projects: []usage.ProjectTotals{{Dir: appDir}, {Dir: shellDir}, {Dir: "/elsewhere"}}, Sessions: []usage.SessionTotals{{Session: session, Dir: appDir}}}
	gone := &usage.Report{Projects: []usage.ProjectTotals{{Dir: appDir}}, Sessions: []usage.SessionTotals{{Session: session}}}
	s.viewerUsage([]usage.MachineReport{{ID: machines.LocalID, Report: report}, {ID: "removed meanwhile", Report: gone}})
	if len(report.Projects) != 1 || report.Projects[0].Dir != appDir || report.Sessions == nil || len(report.Sessions) != 0 {
		t.Errorf("a viewer's usage: %+v", report)
	}
	if len(gone.Projects) != 0 || len(gone.Sessions) != 0 {
		t.Errorf("a viewer's usage of a machine removed meanwhile: %+v", gone)
	}

	s.Feed.Own = func(machineID, paneID string) *bool {
		if _, pane := s.paneOf(machineID, paneID); pane == nil {
			return nil
		}
		return new(s.ownPane(machineID, paneID))
	}
	s.noteSent(context.Background(), machines.LocalID, ownPane, "the-secret", false)
	s.noteSent(context.Background(), machines.LocalID, appPane, "hello", false)
	s.Feed.Add(sent.Entry("Phone", "unmarked-secret, its pane gone", false, machines.LocalID, "w99:p9", ""))
	s.Feed.Add(feed.Entry{Kind: "asked", Machine: machines.LocalID, Pane: "w96:p1", Title: "unmarked-secret alert, its pane gone"})
	s.Feed.Add(feed.Entry{Kind: "digest", Title: "digest-secret, quoting an answer"})
	s.Feed.Add(feed.Entry{Kind: "done", Machine: machines.LocalID, Pane: "w98:p1", Title: "own-secret, its pane gone", Own: new(true)})
	s.Feed.Add(feed.Entry{Kind: "done", Machine: machines.LocalID, Pane: "w97:p1", Title: "finished, its pane closed since", Own: new(false)})
	_, body := fileCall(h, "/api/feed", true)
	if strings.Contains(body, "secret") || !strings.Contains(body, "hello") || !strings.Contains(body, "finished") {
		t.Errorf("a viewer's activity: %s", body)
	}
	if all := s.Feed.List(0); len(all) != 7 {
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
	if s.viewerReach(mc).Allows(filepath.Join(shellDir, "notes.txt")) {
		t.Error("a viewer reaches into the folder of a floating terminal just ended")
	}
}

func TestAViewerGetsNoRecordingOfRemudasOwnWorkspaces(t *testing.T) {
	for _, label := range []string{scratch.Label, "renamed by hand"} {
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
	meta.Own, meta.Dir = nil, filepath.Join(s.Machines.All()[0].StateDir(), "scenes")
	if !s.ownRecording(meta) {
		t.Error("an unmarked recording made in a state dir")
	}
	meta.Dir = "/a/folder/no/workspace/stands/in"
	if !s.ownRecording(meta) {
		t.Error("an unmarked recording of a folder out of a viewer's reach")
	}
	meta.Machine = "a machine removed since"
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
