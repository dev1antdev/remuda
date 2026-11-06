package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/machines"
)

// The wake route refuses what it cannot do: an unknown machine, one without an address, one that is awake already.
func TestWakeRefusesWhatItCannotDo(t *testing.T) {
	s := &Server{Machines: machines.NewManager(t.TempDir(), "", "", nil)}
	mux := http.NewServeMux()
	s.machineRoutes(mux) // registers the wake route too

	if code, _ := taskCall(mux, "POST", "/api/machines/nowhere/wake", "", ""); code != 404 {
		t.Fatalf("unknown machine: %d, want 404", code)
	}
	code, out := taskCall(mux, "POST", "/api/machines/local/wake", "", "")
	if msg, _ := out["error"].(string); code != 400 || !strings.Contains(msg, "no MAC address") {
		t.Fatalf("no address: %d %q, want 400", code, msg)
	}

	code, out = taskCall(mux, "PATCH", "/api/machines/local", "", `{"mac":"AA-BB-CC-DD-EE-FF"}`)
	if code != 200 || out["mac"] != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("set the address: %d %v", code, out)
	}
	if got := s.Machines.MAC("local"); got != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("kept address = %q", got)
	}
	code, out = taskCall(mux, "POST", "/api/machines/local/wake", "", "")
	if msg, _ := out["error"].(string); code != 409 || !strings.Contains(msg, "connected") {
		t.Fatalf("a connected machine: %d %q, want 409", code, msg)
	}

	if code, out := taskCall(mux, "PATCH", "/api/machines/local", "", `{"mac":"not-an-address"}`); code != 400 || out["error"] == nil {
		t.Fatalf("a bad address: %d %v, want 400", code, out)
	}
	if code, _ := taskCall(mux, "PATCH", "/api/machines/local", "", `{"mac":""}`); code != 200 || s.Machines.MAC("local") != "" {
		t.Fatalf("clearing the address: %d, kept %q", code, s.Machines.MAC("local"))
	}
}

// A PATCH that names only the address leaves the machine's name alone, and one that names only the name leaves the address.
func TestMachinePatchIsPartial(t *testing.T) {
	s := &Server{Machines: machines.NewManager(t.TempDir(), "", "", nil)}
	mux := http.NewServeMux()
	s.machineRoutes(mux)
	if code, _ := taskCall(mux, "PATCH", "/api/machines/local", "", `{"label":"Desk"}`); code != 200 {
		t.Fatalf("rename: %d", code)
	}
	if code, _ := taskCall(mux, "PATCH", "/api/machines/local", "", `{"mac":"aa:bb:cc:dd:ee:ff"}`); code != 200 {
		t.Fatalf("set the address: %d", code)
	}
	mc, _ := s.Machines.Get("local")
	if mc.Label() != "Desk" || s.Machines.MAC("local") != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("label %q, mac %q after the two patches", mc.Label(), s.Machines.MAC("local"))
	}
	if code, _ := taskCall(mux, "PATCH", "/api/machines/local", "", `{"label":""}`); code != 200 {
		t.Fatalf("the name back: %d", code)
	}
	if s.Machines.MAC("local") != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("the address went with the name")
	}
}
