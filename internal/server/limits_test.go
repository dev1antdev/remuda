package server

import (
	"net/http"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/limits"
)

// The booking routes: the person books a word to an agent at a reset and calls it off; what a page can get wrong is refused.
func TestLimitContinueRoutes(t *testing.T) {
	svc := limits.New(t.TempDir())
	svc.Pane = func(machine, pane string) (limits.Place, bool) {
		switch pane {
		case "w1:p1":
			return limits.Place{Agent: "claude", Session: "s1", Status: "idle", Workspace: "api"}, true
		case "w1:p2":
			return limits.Place{Status: "idle", Workspace: "api"}, true // a shell
		}
		return limits.Place{}, false
	}
	s := &Server{Limits: svc}
	mux := http.NewServeMux()
	s.limitRoutes(mux)
	at := time.Now().Add(time.Hour).UTC().Truncate(time.Second)

	code, out := taskCall(mux, "POST", "/api/limits/continue", "", `{"machine":"local","pane":"w1:p1","at":"`+at.Format(time.RFC3339)+`"}`)
	if code != 200 || out["continue_at"] == nil {
		t.Fatalf("book: %d %v", code, out)
	}
	if p := svc.PaneOf("local", "w1:p1"); p == nil || p.ContinueAt == nil || !p.ContinueAt.Equal(at) {
		t.Fatalf("booking: %+v", p)
	}

	refused := map[string]string{
		"no pane":    `{"machine":"local","at":"` + at.Format(time.RFC3339) + `"}`,
		"bad time":   `{"machine":"local","pane":"w1:p1","at":"after lunch"}`,
		"past time":  `{"machine":"local","pane":"w1:p1","at":"2020-01-01T00:00:00Z"}`,
		"far ahead":  `{"machine":"local","pane":"w1:p1","at":"` + time.Now().Add(30*24*time.Hour).UTC().Format(time.RFC3339) + `"}`,
		"other pane": `{"machine":"local","pane":"w9:p9","at":"` + at.Format(time.RFC3339) + `"}`,
		"a shell":    `{"machine":"local","pane":"w1:p2","at":"` + at.Format(time.RFC3339) + `"}`,
	}
	for name, body := range refused {
		code, out := taskCall(mux, "POST", "/api/limits/continue", "", body)
		msg, _ := out["error"].(string)
		if code != 400 || msg == "" {
			t.Errorf("%s: %d %q, want 400 with a reason", name, code, msg)
		}
	}

	code, _ = taskCall(mux, "DELETE", "/api/limits/continue", "", `{"machine":"local","pane":"w1:p1"}`)
	if code != 200 || svc.PaneOf("local", "w1:p1") != nil {
		t.Fatalf("cancel: %d", code)
	}
}
