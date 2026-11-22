package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/herdr/herdrfake"
	"github.com/dev1antdev/remuda/internal/repos"
	"github.com/dev1antdev/remuda/internal/undo"
	"github.com/dev1antdev/remuda/internal/worktrees"
)

func TestAProjectsSettingsAreKeptAndReadBack(t *testing.T) {
	dir := t.TempDir()
	s := &Server{Machines: herdrfake.Manager(dir), Repos: repos.Open(dir)}
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

func TestAConfirmationAnswersForWhatTheDialogSaid(t *testing.T) {
	look := worktrees.Look{Branch: "work", Tip: "abc"}
	plain := removalStamp(look, false, false)
	if plain == removalStamp(look, true, false) {
		t.Error("a close alone reads as a removal once the others left")
	}
	if plain == removalStamp(look, false, true) {
		t.Error("a branch the dialog called safe reads the same once its merged request is forgotten")
	}
}

// TestSettingsOnASymlinkedFolderAreKeptAsTheMachineNamesIt: the repository's settings are keyed by the folder the
// roster's sessions are named in, so saving them through a symlink is not a second repository.
func TestSettingsOnASymlinkedFolderAreKeptAsTheMachineNamesIt(t *testing.T) {
	dir := t.TempDir()
	s := &Server{Machines: herdrfake.Manager(dir), Repos: repos.Open(dir)}
	api := http.NewServeMux()
	s.repoRoutes(api)
	real := t.TempDir()
	physical, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"machine":"local","root":%q,"settings":{"base":"main"}}`, link)
	if rec := ask(api, "PUT", "/api/repos/settings", body); rec.Code != 200 {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}
	for _, root := range []string{link, physical} { // saved through the link, read through either spelling
		rec := ask(api, "GET", "/api/repos/settings?machine=local&root="+root+"&quick=1", "")
		var got struct {
			Root     string         `json:"root"`
			Settings repos.Settings `json:"settings"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Settings.Base != "main" || got.Root != physical {
			t.Fatalf("the settings saved through a symlink are not read back through %q: %+v", root, got)
		}
	}
}

// TestSettingsSavedUnderAnOldSpellingAreStillFound: a repository's settings saved before folders were named the
// machine's way keep their spelling, and are read through either spelling afterwards — no migration, no loss.
func TestSettingsSavedUnderAnOldSpellingAreStillFound(t *testing.T) {
	dir := t.TempDir()
	store := repos.Open(dir)
	s := &Server{Machines: herdrfake.Manager(dir), Repos: store}
	api := http.NewServeMux()
	s.repoRoutes(api)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put("local", link, repos.Settings{Base: "legacy"}); err != nil {
		t.Fatal(err)
	}
	rec := ask(api, "GET", "/api/repos/settings?machine=local&root="+link+"&quick=1", "")
	var got struct {
		Settings repos.Settings `json:"settings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Settings.Base != "legacy" {
		t.Fatalf("settings saved under %q are not read through it: %+v", link, got)
	}
	// a folder that is not there and whose .. names another one is refused, never stored under the other one
	body := `{"machine":"local","root":"/checkout/missing/..","settings":{"base":"main"}}`
	if rec := ask(api, "PUT", "/api/repos/settings", body); rec.Code < 400 {
		t.Fatalf("a folder whose .. names another is saved anyway: %d %s", rec.Code, rec.Body)
	}
}

// TestClearingSettingsThroughAnOldSpellingClearsThem: a repository's settings saved under a link move to the machine's
// spelling when they are next saved, and a clear takes them away — never leaving the old record to be found again.
func TestClearingSettingsThroughAnOldSpellingClearsThem(t *testing.T) {
	dir := t.TempDir()
	store := repos.Open(dir)
	s := &Server{Machines: herdrfake.Manager(dir), Repos: store}
	api := http.NewServeMux()
	s.repoRoutes(api)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put("local", link, repos.Settings{Base: "legacy"}); err != nil {
		t.Fatal(err)
	}
	put := func(settings string) {
		t.Helper()
		body := fmt.Sprintf(`{"machine":"local","root":%q,"settings":%s}`, link, settings)
		if rec := ask(api, "PUT", "/api/repos/settings", body); rec.Code != 200 {
			t.Fatalf("put %s: %d %s", settings, rec.Code, rec.Body)
		}
	}
	put(`{"base":"main"}`) // saved through the link: the record moves to the machine's spelling
	if store.Has("local", link) {
		t.Fatal("the old spelling kept its record after a save")
	}
	put(`{}`) // cleared through the link
	rec := ask(api, "GET", "/api/repos/settings?machine=local&root="+link+"&quick=1", "")
	var got struct {
		Settings repos.Settings `json:"settings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Settings.Base != "" {
		t.Fatalf("settings cleared through a link came back: %+v", got)
	}
}
