package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/repos"
	"github.com/dev1antdev/remuda/internal/undo"
)

func TestAProjectsSettingsAreKeptAndReadBack(t *testing.T) {
	dir := t.TempDir()
	s := &Server{Machines: machines.NewManager(dir, filepath.Join(dir, "no-herdr.sock"), "herdr", nil), Repos: repos.Open(dir)}
	api := http.NewServeMux()
	s.repoRoutes(api)

	body := `{"machine":"local","root":"/repo/app","settings":{"base":"main","setup":"pnpm i","setup_when":"ask","actions":[{"name":"Dev","command":"npm run dev","place":"right"}]}}`
	rec := ask(api, "PUT", "/api/repos/settings", body)
	if rec.Code != 200 {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}
	rec = ask(api, "GET", "/api/repos/settings?machine=local&root=/repo/app", "")
	var got struct {
		Name     string            `json:"name"`
		Settings repos.Settings    `json:"settings"`
		Defaults map[string]string `json:"defaults"`
		Trouble  string            `json:"trouble"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 200 {
		t.Fatalf("get: %d %s", rec.Code, rec.Body)
	}
	if got.Name != "app" || got.Settings.Base != "main" || got.Settings.SetupWhen != "ask" || len(got.Settings.Actions) != 1 || got.Settings.Actions[0].ID == "" {
		t.Errorf("read back %+v", got)
	}
	if got.Defaults["issue_branch"] != repos.DefaultIssueBranch || got.Trouble == "" {
		t.Errorf("no defaults, or a machine with no herdr read as fine: %+v", got)
	}
	for _, bad := range []string{
		`{"machine":"local","root":"repo","settings":{}}`,
		`{"machine":"local","root":"/repo/app","settings":{"actions":[{"name":"Dev","command":"a\nb"}]}}`,
		`{"machine":"elsewhere","root":"/repo/app","settings":{}}`,
	} {
		if rec := ask(api, "PUT", "/api/repos/settings", bad); rec.Code < 400 {
			t.Errorf("%s was kept: %d", bad, rec.Code)
		}
	}
}

func TestGitWaitsWhileAnAgentWorksInTheCheckout(t *testing.T) {
	s := &Server{Undo: &undo.Service{Working: func(machineID, folder string) string {
		if folder == "/repo/app" {
			return "Claude Code"
		}
		return ""
	}}}
	if got := s.busyIn("local", "/repo/app"); got != "Claude Code works or asks there" {
		t.Errorf("got %q", got)
	}
	if got := s.busyIn("local", "/repo/other"); got != "" {
		t.Errorf("an idle folder answered %q", got)
	}
}

func TestTheWorkspacesSharingAWorktreeAreNamed(t *testing.T) {
	tree := "/home/me/.herdr/worktrees/app/fix"
	snap := &herdr.Snapshot{
		Workspaces: []herdr.WorkspaceInfo{
			{WorkspaceID: "w1", Label: "fix", Worktree: &herdr.WorktreeInfo{CheckoutPath: tree, IsLinkedWorktree: true}},
			{WorkspaceID: "w2", Label: "api-fix", Worktree: &herdr.WorktreeInfo{CheckoutPath: tree, IsLinkedWorktree: true}},
			{WorkspaceID: "w3", Label: "app"},
		},
		Panes: []herdr.PaneInfo{
			{PaneID: "w1:p1", WorkspaceID: "w1", CWD: tree},
			{PaneID: "w3:p1", WorkspaceID: "w3", CWD: "/home/me/code/app", ForegroundCWD: tree + "/web"},
		},
	}
	got := sharers(snap, "w1", tree)
	if len(got) != 2 || got[0] != "api-fix" || got[1] != "app" {
		t.Errorf("got %v", got)
	}
}

func TestTheNewRoutesAreThePersonsAlone(t *testing.T) {
	key := auth.Key{Scope: auth.ScopeTasks}
	for _, route := range [][2]string{
		{"GET", "/api/worktrees/source"}, {"GET", "/api/worktrees/github"}, {"GET", "/api/worktrees/lookup"},
		{"POST", "/api/worktrees/fetch"}, {"GET", "/api/worktrees/w1/removal"}, {"POST", "/api/worktrees/w1/remove"},
		{"GET", "/api/repos/settings"}, {"PUT", "/api/repos/settings"}, {"POST", "/api/run"},
	} {
		if auth.ViewerAllows(route[0], route[1]) {
			t.Errorf("a viewer link reaches %s %s", route[0], route[1])
		}
		if key.Allows(route[0], route[1]) {
			t.Errorf("an agent's key reaches %s %s", route[0], route[1])
		}
	}
}
