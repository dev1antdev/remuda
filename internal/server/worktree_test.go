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

	folder := filepath.Join(t.TempDir(), "app") // a folder the machine can name
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"machine":"local","root":"` + folder + `","settings":{"base":"main","setup":"pnpm i","setup_when":"ask","actions":[{"name":"Dev","command":"npm run dev","place":"right"}]}}`
	rec := ask(api, "PUT", "/api/repos/settings", body)
	if rec.Code != 200 {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}
	rec = ask(api, "GET", "/api/repos/settings?machine=local&root="+folder, "")
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

// TestASaveThroughTheSameFolderInAnotherSpellingKeepsIt: a folder named a second way — . or a resolved .. that cleans
// to the same place — is the same record, never one that retires the other.
func TestASaveThroughTheSameFolderInAnotherSpellingKeepsIt(t *testing.T) {
	dir := t.TempDir()
	store := repos.Open(dir)
	s := &Server{Machines: herdrfake.Manager(dir), Repos: store}
	api := http.NewServeMux()
	s.repoRoutes(api)
	root := t.TempDir()
	for _, spelling := range []string{root, root + "/."} {
		body := fmt.Sprintf(`{"machine":"local","root":%q,"settings":{"base":"main"}}`, spelling)
		if rec := ask(api, "PUT", "/api/repos/settings", body); rec.Code != 200 {
			t.Fatalf("put %q: %d %s", spelling, rec.Code, rec.Body)
		}
		rec := ask(api, "GET", "/api/repos/settings?machine=local&root="+root+"&quick=1", "")
		var got struct {
			Settings repos.Settings `json:"settings"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Settings.Base != "main" {
			t.Fatalf("settings saved through %q are not there: %+v", spelling, got)
		}
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
	put(`{"base":"main"}`) // saved through the link: the record is the machine's spelling's
	put(`{}`)              // cleared through the link
	read := func(root string) repos.Settings {
		t.Helper()
		rec := ask(api, "GET", "/api/repos/settings?machine=local&root="+root+"&quick=1", "")
		var got struct {
			Settings repos.Settings `json:"settings"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got.Settings
	}
	if base := read(link).Base; base != "" {
		t.Fatalf("settings cleared through a link came back: %q", base)
	}
	// the same, when the clear is made through the machine's own spelling and read through the link
	physical, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put("local", link, repos.Settings{Base: "stale"}); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"machine":"local","root":%q,"settings":{}}`, physical)
	if rec := ask(api, "PUT", "/api/repos/settings", body); rec.Code != 200 {
		t.Fatalf("clear through %q: %d %s", physical, rec.Code, rec.Body)
	}
	if base := read(link).Base; base != "" {
		t.Fatalf("an older record came back after a clear through the machine's spelling: %q", base)
	}
	// a clear through the link stands even when the machine can no longer name the folder: the link goes away
	if _, err := store.Put("local", link, repos.Settings{Base: "stale"}); err != nil {
		t.Fatal(err)
	}
	body = fmt.Sprintf(`{"machine":"local","root":%q,"settings":{}}`, link)
	if rec := ask(api, "PUT", "/api/repos/settings", body); rec.Code != 200 {
		t.Fatalf("clear through %q: %d %s", link, rec.Code, rec.Body)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if base := read(link).Base; base != "" {
		t.Fatalf("a cleared record came back while the folder could not be resolved: %q", base)
	}
}

// TestSavingThroughASymlinkFollowedByItsParentLeavesTheOtherCheckoutAlone: a folder named with a .. behind a link
// resolves to the folder the link points into, and the settings of the folder it was written through are untouched —
// nothing is ever retired for another spelling.
func TestSavingThroughASymlinkFollowedByItsParentLeavesTheOtherCheckoutAlone(t *testing.T) {
	dir := t.TempDir()
	store := repos.Open(dir)
	s := &Server{Machines: herdrfake.Manager(dir), Repos: store}
	api := http.NewServeMux()
	s.repoRoutes(api)
	first, second := t.TempDir(), t.TempDir()
	if _, err := store.Put("local", first, repos.Settings{Base: "first"}); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(second, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(nested, filepath.Join(first, "link")); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"machine":"local","root":%q,"settings":{"base":"landed"}}`, first+"/link/..") // not Join: it would clean the .. away
	if rec := ask(api, "PUT", "/api/repos/settings", body); rec.Code != 200 {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}
	if got := store.Get("local", first).Base; got != "first" {
		t.Fatalf("the folder the path was written through lost its settings: %q", got)
	}
	physical, err := filepath.EvalSymlinks(second)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Get("local", physical).Base; got != "landed" {
		t.Fatalf("the folder the path resolved to has %q", got)
	}
}

// TestSettingsTheMachineCannotNameAreNotReplaced: a record is kept for a folder the machine cannot name just now, and
// a save read without it — the whole record, as the dialog sends it — is refused rather than erasing what it did not
// show.
func TestSettingsTheMachineCannotNameAreNotReplaced(t *testing.T) {
	dir := t.TempDir()
	store := repos.Open(dir)
	s := &Server{Machines: herdrfake.Manager(dir), Repos: store}
	api := http.NewServeMux()
	s.repoRoutes(api)
	folder := filepath.Join(t.TempDir(), "app")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put("local", folder, repos.Settings{Base: "main", Setup: "pnpm i"}); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(folder); err != nil { // the checkout is not there just now
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"machine":"local","root":%q,"settings":{"location":"sibling"}}`, folder)
	if rec := ask(api, "PUT", "/api/repos/settings", body); rec.Code != http.StatusBadGateway {
		t.Fatalf("a save read without the record replaced it: %d %s", rec.Code, rec.Body)
	}
	if got := store.Get("local", folder); got.Base != "main" || got.Setup != "pnpm i" {
		t.Fatalf("the record the machine could not show was erased: %+v", got)
	}
}
