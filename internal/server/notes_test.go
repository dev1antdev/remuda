package server

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/notes"
	"github.com/dev1antdev/remuda/internal/prstatus"
	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/shell"
)

// A note is written, rides on the roster as the workspace's, and goes again when saved empty; what a page can get
// wrong is refused.
func TestNoteRoutesKeepANotePerCheckout(t *testing.T) {
	dir := t.TempDir()
	fleet := machines.NewManager(dir, filepath.Join(dir, "no-herdr.sock"), "herdr", nil)
	s := &Server{Machines: fleet, Notes: notes.Open(dir), Review: review.New(dir)}
	mux := http.NewServeMux()
	s.noteRoutes(mux)

	code, out := taskCall(mux, "PUT", "/api/notes", "", `{"machine":"local","dir":"/w/app","text":"Waiting on the API team"}`)
	if code != 200 || out["text"] != "Waiting on the API team" {
		t.Fatalf("put: %d %v", code, out)
	}
	snap := &herdr.Snapshot{Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1", Label: "app", Worktree: &herdr.WorktreeInfo{CheckoutPath: "/w/app"}}}}
	if got := s.sessionsFromSnapshot("local", snap); len(got) != 1 || got[0].Note != "Waiting on the API team" {
		t.Fatalf("the roster's session: %+v", got)
	}

	refused := map[string]string{
		"no folder":  `{"machine":"local","text":"x"}`,
		"relative":   `{"machine":"local","dir":"w/app","text":"x"}`,
		"too long":   `{"machine":"local","dir":"/w/app","text":"` + strings.Repeat("a", notes.MaxText+1) + `"}`,
		"a bell":     `{"machine":"local","dir":"/w/app","text":"ding\u0007"}`,
		"no machine": `{"machine":"gog","dir":"/w/app","text":"x"}`,
	}
	for name, body := range refused {
		code, out := taskCall(mux, "PUT", "/api/notes", "", body)
		msg, _ := out["error"].(string)
		want := 400
		if name == "no machine" {
			want = 404
		}
		if code != want || msg == "" {
			t.Errorf("%s: %d %q, want %d with a reason", name, code, msg, want)
		}
	}

	if code, out := taskCall(mux, "PUT", "/api/notes", "", `{"machine":"local","dir":"/w/app","text":"  "}`); code != 200 || out["text"] != "" {
		t.Fatalf("an empty save: %d %v", code, out)
	}
	if s.Notes.Of("local", "/w/app") != "" {
		t.Fatal("saving it empty kept the note")
	}
	taskCall(mux, "PUT", "/api/notes", "", `{"machine":"local","dir":"/w/app","text":"again"}`)
	if code, _ := taskCall(mux, "DELETE", "/api/notes", "", `{"machine":"local","dir":"/w/app"}`); code != 200 || s.Notes.Of("local", "/w/app") != "" {
		t.Fatalf("delete: %d", code)
	}
}

// An open hover card asks for its requests again: each one read longer ago than prstatus.FreshFor is read once more,
// the pane's read under way and its outcome riding on the roster.
func TestPRRefreshReadsTheCardsRequestsAgain(t *testing.T) {
	dir := t.TempDir()
	fleet := machines.NewManager(dir, filepath.Join(dir, "no-herdr.sock"), "herdr", nil)
	host := &shell.Fake{Name: "local", Answer: func(string) (string, error) {
		return `{"state":"OPEN","title":"Saved carts","isDraft":false,"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN","reviewDecision":"","statusCheckRollup":[]}`, nil
	}}
	requests := prstatus.New(func(string) (shell.Shell, bool) { return host, true })
	s := &Server{Machines: fleet, PRs: requests}
	mux := http.NewServeMux()
	s.prRoutes(mux)
	requests.OnNamed(events.PullRequestNamed{Pane: events.PaneRef{Machine: "local", Pane: "w1:p1"}, URL: "https://github.com/acme/storefront/pull/212", Label: "PR #212"})
	settled := func() bool {
		seen, _ := requests.Seen("local", "w1:p1")
		return seen.Status != nil && !seen.Reading
	}
	for i := 0; i < 200 && !settled(); i++ {
		time.Sleep(5 * time.Millisecond)
	}
	if !settled() {
		t.Fatal("the first read never ended")
	}

	if code, out := taskCall(mux, "POST", "/api/pr/refresh", "", `{"machine":"local","panes":["w1:p1","w1:p9"]}`); code != 200 || out["reading"] != float64(0) {
		t.Fatalf("a fresh read asked again: %d %v", code, out)
	}
	was := prstatus.FreshFor
	prstatus.FreshFor = 0
	t.Cleanup(func() { prstatus.FreshFor = was })
	if code, out := taskCall(mux, "POST", "/api/pr/refresh", "", `{"machine":"local","panes":["w1:p1"]}`); code != 200 || out["reading"] != float64(1) {
		t.Fatalf("refresh: %d %v", code, out)
	}
	if code, _ := taskCall(mux, "POST", "/api/pr/refresh", "", `{"machine":"local","panes":[]}`); code != 400 {
		t.Fatalf("no panes: %d", code)
	}
	if code, _ := taskCall(mux, "POST", "/api/pr/refresh", "", `{"machine":"gog","panes":["w1:p1"]}`); code != 404 {
		t.Fatalf("an unknown machine: %d", code)
	}
	for i := 0; i < 200 && !settled(); i++ {
		time.Sleep(5 * time.Millisecond)
	}
}
