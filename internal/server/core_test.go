package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/jobs"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/updates"
)

// Leftover worktrees and a move are not refused for the backend: remuda speaks remuda-core everywhere now, and the
// guards it carried while herdr's agents were unseen here are gone (Core 26e). A page-only host answers with its own
// reason, never for being on remuda-core; the work itself is e2e's, on a throwaway core.
func TestLeftoversAndTeleportAreNotRefusedForBeingOnRemudaCore(t *testing.T) {
	fleet := machines.NewCoreManager(t.TempDir(), "", nil, true) // a page-only host: no core of its own is started
	defer fleet.Close()
	s := &Server{Machines: fleet, Updates: &updates.Service{Jobs: jobs.New(events.NewBus())}}
	mux := http.NewServeMux()
	s.leftoverRoutes(mux)
	s.teleportRoutes(mux)
	s.sessionRoutes(mux)
	for _, call := range []struct{ method, path, body string }{
		{"GET", "/api/machines/local/leftovers", ""},
		{"POST", "/api/machines/local/leftovers/remove", `{"paths":["/w/x"]}`},
		{"POST", "/api/machines/gone/leftovers/remove?machine=local", `{"paths":["/w/x"]}`},
		{"POST", "/api/teleport", `{"machine":"local","pane":"w1:p1","target":"local","folder":"/w/app","agent":"claude"}`},
	} {
		code, out := taskCall(mux, call.method, call.path, "", call.body)
		if code == http.StatusConflict || strings.Contains(out["error"].(string), "remuda-core") {
			t.Errorf("%s %s on remuda-core: %d %v", call.method, call.path, code, out)
		}
	}
}

// A machine whose herdr server still runs lists and removes no leftover worktree, and has no work moved off it: its
// herdr agents are not the ones remuda sees. The reason goes with the server, stopped at the person's press.
func TestLeftoversAndMovesWaitWhileHerdrRunsThere(t *testing.T) {
	s := &Server{HerdrMayRun: func(string) string { return "herdr's server may still run there" }}
	w := httptest.NewRecorder()
	if !s.heldByHerdr(w, "local") {
		t.Fatal("herdr still runs there and nothing held")
	}
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "herdr's server may still run") {
		t.Fatalf("the refusal: %d %s", w.Code, w.Body.String())
	}
	s.HerdrMayRun = func(string) string { return "" }
	w = httptest.NewRecorder()
	if s.heldByHerdr(w, "local") {
		t.Fatal("herdr is stopped there and the request was still held")
	}
}
