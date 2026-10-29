package server

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/push"
)

// The mute routes: a folder is silenced until a time, listed, lifted; what a page can get wrong is refused.
func TestMuteRoutes(t *testing.T) {
	svc, err := push.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Push: svc}
	mux := http.NewServeMux()
	s.muteRoutes(mux)
	until := time.Now().Add(time.Hour).UTC().Truncate(time.Second)

	code, out := taskCall(mux, "PUT", "/api/mutes", "", `{"machine":"local","dir":"/w/api","until":"`+until.Format(time.RFC3339)+`"}`)
	if code != 200 || out["muted_until"] == nil {
		t.Fatalf("mute: %d %v", code, out)
	}
	if got, _ := svc.Mutes.Until("local", "/w/api"); !got.Equal(until) {
		t.Fatalf("stored until = %v, want %v", got, until)
	}
	code, out = taskCall(mux, "GET", "/api/mutes", "", "")
	list, _ := out["mutes"].([]any)
	if code != 200 || len(list) != 1 {
		t.Fatalf("list: %d %v", code, out)
	}

	refused := map[string]string{
		"no folder":  `{"machine":"local","until":"` + until.Format(time.RFC3339) + `"}`,
		"bad time":   `{"machine":"local","dir":"/w/api","until":"tomorrow"}`,
		"past time":  `{"machine":"local","dir":"/w/api","until":"2020-01-01T00:00:00Z"}`,
		"no machine": `{"dir":"/w/api","until":"` + until.Format(time.RFC3339) + `"}`,
	}
	for name, body := range refused {
		code, out := taskCall(mux, "PUT", "/api/mutes", "", body)
		msg, _ := out["error"].(string)
		if code != 400 || msg == "" {
			t.Errorf("%s: %d %q, want 400 with a reason", name, code, msg)
		}
	}

	code, _ = taskCall(mux, "DELETE", "/api/mutes", "", `{"machine":"local","dir":"/w/api"}`)
	if code != 200 {
		t.Fatalf("unmute: %d", code)
	}
	if _, muted := svc.Mutes.Until("local", "/w/api"); muted {
		t.Fatal("still muted after DELETE")
	}
	code, out = taskCall(mux, "DELETE", "/api/mutes", "", `{"machine":"local"}`)
	if msg, _ := out["error"].(string); code != 400 || !strings.Contains(msg, "folder") {
		t.Fatalf("unmute without a folder: %d %v", code, out)
	}
}
