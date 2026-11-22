package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr/herdrfake"
	"github.com/dev1antdev/remuda/internal/machines"
)

// TestHerdrsServerIsStoppedOnlyWhereTheFleetLeftIt: the stop is refused before any script runs on a scratch instance,
// for a machine the fleet does not have, and on the page's own host under -server. Nothing here reaches a herdr: a
// machine still on herdr is no case any more (Core 26j), and the stop of a local one is not asked here at all.
func TestHerdrsServerIsStoppedOnlyWhereTheFleetLeftIt(t *testing.T) {
	stop := func(s *Server, machine string) (int, string) {
		mux := http.NewServeMux()
		s.recoverRoutes(mux)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/machines/"+machine+"/herdr/stop", nil))
		return rec.Code, rec.Body.String()
	}
	// herdr is pinned to a stand-in that records its call and a socket of the test's: a stop must never be the person's
	// own server, which an agent running in herdr carries in HERDR_SOCKET_PATH
	calls := filepath.Join(t.TempDir(), "calls")
	stub := filepath.Join(t.TempDir(), "herdr")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\necho \"$*\" >> "+calls+"\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	pin := func(m *machines.Manager) { m.SetMovedHerdr(stub, filepath.Join(t.TempDir(), "herdr.sock")) }
	onHerdr := herdrfake.Manager(t.TempDir())
	t.Cleanup(onHerdr.Close)
	pin(onHerdr)
	if code, body := stop(&Server{Machines: onHerdr}, "local"); code != 409 || !strings.Contains(body, "scratch instance") {
		t.Fatalf("a scratch instance: %d %s", code, body)
	}
	if code, body := stop(&Server{Machines: onHerdr, HerdrStops: true}, "nowhere"); code != 404 {
		t.Fatalf("a machine the fleet lacks: %d %s", code, body)
	}
	pageOnly := machines.NewManager(t.TempDir(), nil, true)
	t.Cleanup(pageOnly.Close)
	pin(pageOnly)
	if code, body := stop(&Server{Machines: pageOnly, HerdrStops: true}, "local"); code != 409 || !strings.Contains(body, "runs as a server") {
		t.Fatalf("the page's own host under -server: %d %s", code, body)
	}
	if said, err := os.ReadFile(calls); err == nil && len(said) > 0 {
		t.Fatalf("herdr was asked to stop on a machine the stop is refused for: %s", said)
	}
}
