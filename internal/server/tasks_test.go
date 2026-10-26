package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/tasks"
)

// taskServer is the task routes over a fresh store, no guard: the caller's name comes straight from the context.
func taskServer(t *testing.T) (http.Handler, *tasks.Store) {
	t.Helper()
	store, err := tasks.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	s := &Server{Tasks: store}
	mux := http.NewServeMux()
	s.tasksRoutes(mux)
	return mux, store
}

func taskCall(h http.Handler, method, path, caller, body string) (int, map[string]any) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if caller != "" {
		req = req.WithContext(auth.WithCallerName(req.Context(), caller))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestTasksTakeMarkdownAndASource(t *testing.T) {
	h, store := taskServer(t)
	projects, _ := store.Projects()
	body := `{"project_id": ` + strconv.FormatInt(projects[0].ID, 10) + `, "title": "Scout", "markdown": "# Plan\n- one\n- two", "source": "w3:p1"}`

	code, person := taskCall(h, "POST", "/api/tasks", "", body)
	if code != 200 || person["source"] != "" {
		t.Fatalf("a person's task: %d %v", code, person)
	}
	if desc, _ := person["description"].(string); desc != "<h1>Plan</h1><ul><li>one</li><li>two</li></ul>" {
		t.Errorf("markdown was not drawn: %q", desc)
	}

	code, agent := taskCall(h, "POST", "/api/tasks", "GOG", body)
	if code != 200 || agent["source"] != "GOG · w3:p1" {
		t.Fatalf("an agent's task: %d %v", code, agent)
	}
	id := strconv.FormatInt(int64(agent["id"].(float64)), 10)

	code, patched := taskCall(h, "PATCH", "/api/tasks/"+id, "GOG", `{"markdown": "plain **bold**", "source": "other"}`)
	if code != 200 || patched["source"] != "GOG · w3:p1" || patched["description"] != "<p>plain <strong>bold</strong></p>" {
		t.Errorf("a patch: %d %v", code, patched)
	}
	code, both := taskCall(h, "PATCH", "/api/tasks/"+id, "", `{"markdown": "ignored", "description": "<p>html wins</p>"}`)
	if code != 200 || both["description"] != "<p>html wins</p>" {
		t.Errorf("html beside markdown: %d %v", code, both)
	}

	code, comment := taskCall(h, "POST", "/api/tasks/"+id+"/comments", "GOG", `{"body": "done"}`)
	if code != 200 || comment["author"] != "agent" {
		t.Errorf("an agent's comment: %d %v", code, comment)
	}
	code, mine := taskCall(h, "POST", "/api/tasks/"+id+"/comments", "", `{"body": "thanks"}`)
	if code != 200 || mine["author"] != "you" {
		t.Errorf("a person's comment: %d %v", code, mine)
	}
}
