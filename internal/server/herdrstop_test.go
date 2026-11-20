package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/machines"
)

// TestHerdrsServerIsStoppedOnlyWhereTheFleetLeftIt: the stop is refused before any script runs on a scratch instance,
// on a machine still reached through herdr, for a machine the fleet does not have, and on the page's own host under
// -server. Nothing here reaches a herdr.
func TestHerdrsServerIsStoppedOnlyWhereTheFleetLeftIt(t *testing.T) {
	stop := func(s *Server, machine string) (int, string) {
		mux := http.NewServeMux()
		s.recoverRoutes(mux)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/machines/"+machine+"/herdr/stop", nil))
		return rec.Code, rec.Body.String()
	}
	onHerdr := machines.NewManager(t.TempDir(), "/nowhere/herdr.sock", "herdr", nil)
	t.Cleanup(onHerdr.Close)
	if code, body := stop(&Server{Machines: onHerdr}, "local"); code != 409 || !strings.Contains(body, "scratch instance") {
		t.Fatalf("a scratch instance: %d %s", code, body)
	}
	if code, body := stop(&Server{Machines: onHerdr, HerdrStops: true}, "local"); code != 409 || !strings.Contains(body, "reached through herdr") {
		t.Fatalf("a machine still on herdr: %d %s", code, body)
	}
	if code, body := stop(&Server{Machines: onHerdr, HerdrStops: true}, "nowhere"); code != 404 {
		t.Fatalf("a machine the fleet lacks: %d %s", code, body)
	}
	pageOnly := machines.NewCoreManager(t.TempDir(), "", "", nil, true)
	t.Cleanup(pageOnly.Close)
	if code, body := stop(&Server{Machines: pageOnly, HerdrStops: true}, "local"); code != 409 || !strings.Contains(body, "runs as a server") {
		t.Fatalf("the page's own host under -server: %d %s", code, body)
	}
}
