package server

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/herdr/herdrtest"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/repos"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/watch"
)

// worktreeServer is a throwaway herdr with a repository of one commit in its HOME, a workspace on its main checkout,
// and the server's worktree routes over them.
func worktreeServer(t *testing.T) (*Server, *http.ServeMux, *herdr.Client, *machines.Machine, string, string) {
	t.Helper()
	sock := herdrtest.Server(t)
	client := herdr.NewClient(sock)
	home := os.Getenv("HOME")
	repo := filepath.Join(home, "code", "app")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"commit", "-q", "--allow-empty", "-m", "one"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	main, err := client.WorkspaceCreate(repo, "app")
	if err != nil {
		t.Fatal(err)
	}
	state := t.TempDir()
	fleet := machines.NewManager(state, sock, "herdr", nil)
	watcher := watch.New(fleet, events.NewBus())
	s := &Server{Machines: fleet, Watch: watcher, Input: sessions.NewInput(fleet, watcher), Repos: repos.Open(state)}
	mc, err := fleet.Get("local")
	if err != nil {
		t.Fatal(err)
	}
	api := http.NewServeMux()
	s.worktreeRoutes(api)
	s.sessionRoutes(api)
	return s, api, client, mc, repo, main.Workspace.WorkspaceID
}

// A branch checked out in a workspace the watcher has not read yet: herdr's worktree.open answers that workspace
// (already_open), and the create answers it as opened — nothing started in its pane, which the person uses.
func TestOpeningACheckoutHerdrHasOpenAlreadyStartsNothingThere(t *testing.T) {
	_, api, client, _, repo, mainWS := worktreeServer(t)
	made, err := client.WorktreeCreateAt(mainWS, "feat-x", "", "x", "")
	if err != nil {
		t.Fatal(err)
	}
	body := `{"machine":"local","workspace":"` + mainWS + `","root":"` + repo + `","branch":"feat-x","agent":"","pick":{"kind":"branch","branch":"feat-x"}}`
	rec := ask(api, "POST", "/api/worktrees", body)
	if rec.Code != 200 {
		t.Fatalf("the create answered %d %s", rec.Code, rec.Body)
	}
	var answer struct {
		ID     string `json:"id"`
		Pane   string `json:"pane"`
		Opened bool   `json:"opened"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.ID != made.Workspace.WorkspaceID || answer.Pane != made.RootPane.PaneID {
		t.Fatalf("herdr answered another workspace: %s", rec.Body)
	}
	if !answer.Opened {
		t.Errorf("a workspace herdr had open already was answered as made, its pane would get the agent and its first prompt: %s", rec.Body)
	}
}

// A workspace whose worktree folder was deleted behind herdr's back stays a linked worktree in herdr's snapshot: the
// remove dialog cannot look at it, and its Close, keep the folder must still close the workspace — the page's close of
// such a workspace goes through that dialog alone.
func TestAWorkspaceWhoseWorktreeFolderIsGoneStillCloses(t *testing.T) {
	s, api, client, mc, _, mainWS := worktreeServer(t)
	s.prRoutes(api)
	made, err := client.WorktreeCreateAt(mainWS, "feat-x", "", "x", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(made.Worktree.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Watch.Refresh(mc); err != nil {
		t.Fatal(err)
	}
	id := made.Workspace.WorkspaceID
	if rec := ask(api, "GET", "/api/worktrees/"+id+"/removal?machine=local", ""); rec.Code == 200 {
		t.Fatalf("a worktree whose folder is gone was looked at: %s", rec.Body)
	}
	if rec := ask(api, "DELETE", "/api/worktrees/"+id+"?machine=local", ""); rec.Code == 200 {
		t.Fatalf("a worktree went past the dialog's checks: %s", rec.Body)
	}
	rec := ask(api, "POST", "/api/worktrees/"+id+"/remove", `{"machine":"local","keep_folder":true}`)
	if rec.Code != 200 {
		t.Fatalf("Close, keep the folder answered %d %s", rec.Code, rec.Body)
	}
	var answer struct {
		Closed bool `json:"closed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil || !answer.Closed {
		t.Errorf("the answer is %s", rec.Body)
	}
	snap, err := client.SessionSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Workspace(id) != nil {
		t.Errorf("the workspace %s is still open", id)
	}
}
