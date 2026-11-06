package server

import (
	"net/http"
	"testing"
)

// A move refuses what it cannot do before it reaches a machine, and a move nobody started has no result.
func TestTeleportRefusals(t *testing.T) {
	s := &Server{}
	mux := http.NewServeMux()
	s.teleportRoutes(mux)
	for name, body := range map[string]string{
		"no agent":        `{"machine":"local","pane":"w1:p1","target":"gog","folder":"/w/app"}`,
		"a shell":         `{"machine":"local","pane":"w1:p1","target":"gog","folder":"/w/app","agent":"shell"}`,
		"unknown agent":   `{"machine":"local","pane":"w1:p1","target":"gog","folder":"/w/app","agent":"nobody"}`,
		"relative folder": `{"machine":"local","pane":"w1:p1","target":"gog","folder":"w/app","agent":"claude"}`,
		"bad arguments":   `{"machine":"local","pane":"w1:p1","target":"gog","folder":"/w/app","agent":"claude","args":"--x 'open"}`,
	} {
		if code, out := taskCall(mux, "POST", "/api/teleport", "", body); code != 400 {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	if code, _ := taskCall(mux, "GET", "/api/teleport/nope", "", ""); code != 404 {
		t.Errorf("a move nobody started: %d", code)
	}
}
