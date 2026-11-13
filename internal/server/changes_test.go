package server

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/gitstage"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/review"
)

func TestTheLensWritesAreThePersonsAlone(t *testing.T) {
	for _, path := range []string{"/api/review/stage", "/api/review/unstage", "/api/review/discard", "/api/review/commit"} {
		for _, key := range []auth.Key{{Scope: auth.ScopeTasks}, {Scope: auth.ScopeViewer}} {
			if key.Allows("POST", path) {
				t.Errorf("a %s key reaches %s", key.Scope, path)
			}
		}
		if auth.ViewerAllows("POST", path) {
			t.Errorf("a viewer's device reaches %s", path)
		}
	}
	for _, path := range []string{"/api/review/commit", "/api/review/base", "/api/review/conflict", "/api/review/last-commit"} {
		if auth.ViewerAllows("GET", path) || (auth.Key{Scope: auth.ScopeTasks}).Allows("GET", path) {
			t.Errorf("%s is read by a viewer or a key", path)
		}
	}
}

func TestTheLensStagesCommitsAndSaysWhyNot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	home := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	dir := t.TempDir()
	git := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
		return strings.TrimRight(string(output), "\n")
	}
	git("init", "-q", "-b", "main")
	_ = os.WriteFile(filepath.Join(dir, "a.go"), []byte("a\n"), 0o644)
	git("add", "-A")
	git("commit", "-qm", "init")
	_ = os.WriteFile(filepath.Join(dir, "a.go"), []byte("a2\n"), 0o644)

	busy := ""
	s := &Server{Machines: machines.NewManager(t.TempDir(), "/nowhere.sock", "herdr", nil), Review: review.New(t.TempDir()),
		Stage: &gitstage.Service{Busy: func(string, string, string) string { return busy }}}
	mux := http.NewServeMux()
	s.reviewRoutes(mux)
	s.changesRoutes(mux)
	body := func(v any) string {
		out, _ := json.Marshal(v)
		return string(out)
	}

	busy = "Codex"
	code, out := taskCall(mux, "POST", "/api/review/stage", "", body(map[string]any{"machine": "local", "dir": dir, "paths": []string{"a.go"}}))
	if code != 409 || !strings.Contains(out["error"].(string), "Codex is working") {
		t.Fatalf("a stage while Codex works: %d %v", code, out)
	}
	if code, summary := taskCall(mux, "GET", "/api/review?machine=local&dir="+dir, "", ""); code != 200 || summary["busy"] != "Codex" {
		t.Fatalf("the summary names the agent at work: %d %v", code, summary["busy"])
	}
	busy = ""
	if code, out := taskCall(mux, "POST", "/api/review/stage", "", body(map[string]any{"machine": "local", "dir": dir, "paths": []string{"a.go"}})); code != 200 {
		t.Fatalf("stage: %d %v", code, out)
	}
	if code, out := taskCall(mux, "GET", "/api/review/diff?machine=local&dir="+dir+"&path=a.go&side=staged", "", ""); code != 200 || !strings.Contains(out["diff"].(string), "+a2") {
		t.Fatalf("the staged side: %d %v", code, out)
	}
	code, made := taskCall(mux, "POST", "/api/review/commit", "", body(map[string]any{"machine": "local", "dir": dir, "message": "Say a2"}))
	if code != 200 || made["subject"] != "Say a2" {
		t.Fatalf("commit: %d %v", code, made)
	}
	if code, out := taskCall(mux, "GET", "/api/review/commit?machine=local&dir="+dir+"&sha="+made["sha"].(string), "", ""); code != 200 || out["subject"] != "Say a2" {
		t.Fatalf("the commit read back: %d %v", code, out)
	}
	if code, out := taskCall(mux, "POST", "/api/review/commit", "", body(map[string]any{"machine": "local", "dir": dir, "message": "again"})); code != 409 || !strings.Contains(out["error"].(string), "nothing is staged") {
		t.Fatalf("an empty commit: %d %v", code, out)
	}
	if code, out := taskCall(mux, "POST", "/api/review/discard", "", body(map[string]any{"machine": "local", "dir": dir, "path": "a.go", "kind": "everything"})); code != 409 {
		t.Fatalf("a discard of no known kind: %d %v", code, out)
	}
	if code, _ := taskCall(mux, "GET", "/api/review/conflict?machine=local&dir="+dir+"&path=../x", "", ""); code != 400 {
		t.Fatalf("a path climbing out: %d", code)
	}
	if code, out := taskCall(mux, "GET", "/api/review/last-commit?machine=local&dir="+dir, "", ""); code != 200 || out["message"] != "Say a2" {
		t.Fatalf("the last commit: %d %v", code, out)
	}
}
