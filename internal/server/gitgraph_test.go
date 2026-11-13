package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/gitgraph"
	"github.com/dev1antdev/remuda/internal/gitops"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/watch"
)

// gitRepository is a checkout with two commits on main and a branch beside it, the person's git settings kept out.
func gitRepository(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Ada")
	t.Setenv("GIT_AUTHOR_EMAIL", "ada@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Ada")
	t.Setenv("GIT_COMMITTER_EMAIL", "ada@example.com")
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"commit", "-q", "--allow-empty", "-m", "first"}, {"branch", "side"}, {"commit", "-q", "--allow-empty", "-m", "second"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return dir
}

func gitServer(t *testing.T, busy string) *http.ServeMux {
	t.Helper()
	state := t.TempDir()
	fleet := machines.NewManager(state, filepath.Join(state, "no-herdr.sock"), "herdr", nil)
	s := &Server{Machines: fleet, Watch: watch.New(fleet, events.NewBus()), GitOps: &gitops.Service{Busy: func(string, string, []string) string { return busy }}}
	api := http.NewServeMux()
	s.gitRoutes(api)
	return api
}

func TestTheGitLensReadsTheHistoryAndAViewerOnlyInsideTheWorkspaces(t *testing.T) {
	dir := gitRepository(t)
	api := gitServer(t, "")
	rec := ask(api, "GET", "/api/git/log?machine=local&all=1&dir="+dir, "")
	var log gitgraph.Log
	if err := json.Unmarshal(rec.Body.Bytes(), &log); err != nil || rec.Code != 200 || len(log.Commits) != 2 || log.Branch != "main" {
		t.Fatalf("the log: %d %s", rec.Code, rec.Body)
	}
	if rec := ask(api, "GET", "/api/git/log?machine=local&dir=relative", ""); rec.Code != 400 {
		t.Fatalf("a relative folder: %d", rec.Code)
	}
	if rec := ask(api, "GET", "/api/git/commit?machine=local&sha=HEAD&dir="+dir, ""); rec.Code != 400 {
		t.Fatalf("a commit named otherwise than by its hash: %d", rec.Code)
	}
	req := httptest.NewRequest("GET", "/api/git/log?machine=local&dir="+dir, nil)
	req = req.WithContext(auth.WithViewer(req.Context()))
	viewer := httptest.NewRecorder()
	api.ServeHTTP(viewer, req)
	if viewer.Code != http.StatusForbidden {
		t.Fatalf("a viewer read a folder no workspace stands on: %d", viewer.Code)
	}
}

func TestAGitWriteRunsAtThePersonsPressAndNeverUnderAnAgent(t *testing.T) {
	dir := gitRepository(t)
	head := strings.TrimSpace(func() string {
		out, _ := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
		return string(out)
	}())
	body := `{"machine":"local","dir":"` + dir + `","op":"checkout","ref":"side","head":"` + head + `","branch":"main"}`
	if rec := ask(gitServer(t, "Codex is at work"), "POST", "/api/git/op", body); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "Codex is at work") {
		t.Fatalf("a checkout under an agent's feet: %d %s", rec.Code, rec.Body)
	}
	api := gitServer(t, "")
	if rec := ask(api, "POST", "/api/git/op", strings.Replace(body, head, strings.Repeat("0", 40), 1)); rec.Code != http.StatusConflict {
		t.Fatalf("a checkout drawn before HEAD moved: %d %s", rec.Code, rec.Body)
	}
	if rec := ask(api, "POST", "/api/git/op", strings.Replace(body, `"checkout"`, `"push"`, 1)); rec.Code != 400 {
		t.Fatalf("a push, which is not this lens's: %d %s", rec.Code, rec.Body)
	}
	rec := ask(api, "POST", "/api/git/op", body)
	var result gitops.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || rec.Code != 200 || !result.OK {
		t.Fatalf("the checkout: %d %s", rec.Code, rec.Body)
	}
	if out, _ := exec.Command("git", "-C", dir, "symbolic-ref", "--short", "HEAD").Output(); strings.TrimSpace(string(out)) != "side" {
		t.Fatalf("HEAD is on %s", out)
	}
}

func TestOnlyTheGitLensReadsAreAViewersAndNoneAnAgentKeys(t *testing.T) {
	for _, path := range []string{"/api/git/log", "/api/git/state", "/api/git/commit", "/api/git/diff"} {
		if !auth.ViewerAllows("GET", path) {
			t.Errorf("a viewer cannot read %s", path)
		}
	}
	if auth.ViewerAllows("POST", "/api/git/op") {
		t.Error("a viewer may run git's writes")
	}
	key := auth.Key{Scope: auth.ScopeTasks}
	if key.Allows("POST", "/api/git/op") || key.Allows("GET", "/api/git/log") {
		t.Error("an agent's key reaches the Git lens")
	}
}
