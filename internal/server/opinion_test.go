package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/opinion"
	"github.com/dev1antdev/remuda/internal/review"
)

func opinionServer(t *testing.T) (*Server, *http.ServeMux) {
	t.Helper()
	s := &Server{Opinions: opinion.NewService(t.TempDir()), Machines: machines.NewManager(t.TempDir(), "/nowhere.sock", "herdr", nil)}
	mux := http.NewServeMux()
	s.opinionRoutes(mux)
	return s, mux
}

func TestAnOpinionIsAskedOnlyOfAnAgentForAFolder(t *testing.T) {
	_, mux := opinionServer(t)
	for name, c := range map[string]struct{ body, want string }{
		"no folder":     {`{"machine":"local","agent":"codex"}`, "a folder and an agent"},
		"a shell":       {`{"machine":"local","dir":"/w/app","agent":"shell"}`, "a folder and an agent"},
		"no agent":      {`{"machine":"local","dir":"/w/app"}`, "a folder and an agent"},
		"unknown agent": {`{"machine":"local","dir":"/w/app","agent":"rm -rf"}`, "unknown agent"},
		"unread agent":  {`{"machine":"local","dir":"/w/app","agent":"kimi"}`, "reads the answers of"},
		"bad arguments": {`{"machine":"local","dir":"/w/app","agent":"codex","args":"\"unclosed"}`, ""},
	} {
		code, out := taskCall(mux, "POST", "/api/opinion", "", c.body)
		msg, _ := out["error"].(string)
		if code != 400 || !strings.Contains(msg, c.want) {
			t.Errorf("%s: %d %q, want 400 with %q", name, code, msg, c.want)
		}
	}
}

func TestAnOpinionIsReadDismissedAndForgotten(t *testing.T) {
	s, mux := opinionServer(t)
	if code, out := taskCall(mux, "GET", "/api/opinion?machine=local&dir=/w/app", "", ""); code != 200 || out["opinion"] != nil {
		t.Fatalf("no opinion yet: %d %v", code, out)
	}
	s.Opinions.Begin(opinion.Opinion{Machine: "local", Dir: "/w/app", Agent: "codex", Pane: "w1:p2", Tab: "w1:t2"})
	s.Opinions.Hear("local", "w1:p2", "FINDINGS\nsrc/a.go:3: [high] x\nsrc/b.go:9: y\nSUMMARY\nfine")
	code, out := taskCall(mux, "GET", "/api/opinion?machine=local&dir=/w/app/", "", "")
	got, _ := out["opinion"].(map[string]any)
	if code != 200 || got["state"] != "done" || len(got["comments"].([]any)) != 2 || got["summary"] != "fine" {
		t.Fatalf("read: %d %v", code, out)
	}
	code, out = taskCall(mux, "POST", "/api/opinion/dismiss", "", `{"machine":"local","dir":"/w/app","ids":[1]}`)
	first := out["opinion"].(map[string]any)["comments"].([]any)[0].(map[string]any)
	if code != 200 || first["dismissed"] != true || first["path"] != "src/a.go" || first["severity"] != "high" {
		t.Fatalf("dismiss: %d %v", code, out)
	}
	if code, _ := taskCall(mux, "POST", "/api/opinion/dismiss", "", `{"machine":"local","dir":"/w/none","ids":[1]}`); code != 404 {
		t.Fatalf("a folder without one: %d", code)
	}
	if code, _ := taskCall(mux, "GET", "/api/opinion?machine=nowhere&dir=/w/app", "", ""); code != 404 {
		t.Fatalf("a machine the fleet does not know: %d", code)
	}
	if code, _ := taskCall(mux, "DELETE", "/api/opinion?machine=local&dir=/w/app", "", ""); code != 200 {
		t.Fatalf("forget: %d", code)
	}
	if _, ok := s.Opinions.Get("local", "/w/app"); ok {
		t.Fatal("the opinion stayed")
	}
}

func TestTheReviewerGoesIntoTheFoldersWorkspace(t *testing.T) {
	snap := &herdr.Snapshot{
		Workspaces: []herdr.WorkspaceInfo{
			{WorkspaceID: "w1", Worktree: &herdr.WorktreeInfo{CheckoutPath: "/w/app"}},
			{WorkspaceID: "w2", Worktree: &herdr.WorktreeInfo{CheckoutPath: "/w/tree", IsLinkedWorktree: true}},
		},
		Panes: []herdr.PaneInfo{
			{PaneID: "w1:p1", WorkspaceID: "w1", CWD: "/w/app"},
			{PaneID: "w2:p1", WorkspaceID: "w2", CWD: "/w/tree/src"},
		},
	}
	for _, c := range []struct{ pane, dir, want, err string }{
		{"w1:p1", "/w/app", "w1", ""},
		{"", "/w/tree", "w2", ""},
		{"w2:p1", "/w/tree", "w2", ""},
		{"w1:p1", "/w/tree", "", "not the folder of the pane's workspace"},
		{"w9:p9", "/w/app", "", "not found"},
		{"", "/w/else", "", "no workspace stands on"},
	} {
		got, err := reviewedWorkspace(snap, c.pane, c.dir)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%s %s: %q %v, want an error with %q", c.pane, c.dir, got, err, c.err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s %s: %q %v, want %q", c.pane, c.dir, got, err, c.want)
		}
	}
}

func TestTheReviewerIsToldWhatTheLensShows(t *testing.T) {
	summary := review.Summary{
		Branch:   "task-41",
		Baseline: &review.Baseline{Head: "abc1234"},
		Commits:  []review.Commit{{SHA: "1"}, {SHA: "2"}},
		Files:    []review.File{{Path: "src/a.go", Added: 3, Removed: 1}, {Path: "src/new.ts", Untracked: true}},
	}
	scope := scopeOf(summary)
	if scope.Branch != "task-41" || scope.Base != "abc1234" || scope.Commits != 2 || strings.Join(scope.Files, ", ") != "src/a.go (+3 −1), src/new.ts (new)" {
		t.Fatalf("scope %+v", scope)
	}
	summary.CommitsTotal = 900
	if scopeOf(summary).Commits != 900 {
		t.Fatal("the commits past the listed ones went uncounted")
	}
	if scope := scopeOf(review.Summary{Files: []review.File{{Path: "a"}}}); scope.Base != "" {
		t.Fatal("a folder never reviewed was given a base")
	}
}
