package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dev1antdev/remuda/internal/tasksapi"
)

// fakeList is the tasks routes with one project (two columns), one label and the tasks made through it.
type fakeList struct {
	mu       sync.Mutex
	tasks    []map[string]any
	comments []map[string]any
	labels   []map[string]any
	requests []string
}

func (f *fakeList) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	if r.Header.Get("Authorization") != "Bearer rk_test" {
		w.WriteHeader(401)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	var body map[string]any
	if r.Body != nil {
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &body)
	}
	switch {
	case r.URL.Path == "/api/tasks/projects":
		json.NewEncoder(w).Encode([]map[string]any{{"id": 1, "title": "remuda", "statuses": []map[string]any{{"id": 10, "title": "New", "position": 1, "on_new": true}, {"id": 11, "title": "In progress", "position": 2}}, "fields": []map[string]any{{"id": 5, "title": "Estimate", "kind": "number"}}}})
	case r.URL.Path == "/api/tasks/labels" && r.Method == "GET":
		json.NewEncoder(w).Encode(f.labels)
	case r.URL.Path == "/api/tasks/labels":
		label := map[string]any{"id": float64(len(f.labels) + 100), "title": body["title"]}
		f.labels = append(f.labels, label)
		json.NewEncoder(w).Encode(label)
	case r.URL.Path == "/api/tasks" && r.Method == "GET":
		json.NewEncoder(w).Encode(map[string]any{"tasks": f.tasks, "pages": 1})
	case r.URL.Path == "/api/tasks":
		task := map[string]any{"id": float64(len(f.tasks) + 1), "identifier": "#" + itoa(len(f.tasks)+1), "project_id": body["project_id"], "status_id": float64(10), "labels": []any{}}
		for k, v := range body {
			task[k] = v
		}
		if body["status_id"] == nil {
			task["status_id"] = float64(10)
		}
		f.tasks = append(f.tasks, task)
		json.NewEncoder(w).Encode(task)
	case strings.HasSuffix(r.URL.Path, "/comments"):
		f.comments = append(f.comments, body)
		json.NewEncoder(w).Encode(map[string]any{"id": len(f.comments), "body": body["body"], "author": "agent"})
	case strings.HasSuffix(r.URL.Path, "/labels"):
		for _, t := range f.tasks {
			if t["id"] == float64(1) {
				t["labels"] = append(t["labels"].([]any), map[string]any{"id": body["label_id"], "title": "bug"})
				json.NewEncoder(w).Encode(t)
				return
			}
		}
	case strings.HasPrefix(r.URL.Path, "/api/tasks/"):
		for _, t := range f.tasks {
			if "/api/tasks/"+itoa(int(t["id"].(float64))) == r.URL.Path {
				if r.Method == "PATCH" {
					for k, v := range body {
						t[k] = v
					}
				}
				json.NewEncoder(w).Encode(t)
				return
			}
		}
		w.WriteHeader(404)
		w.Write([]byte(`{"error":"not found: no such task"}`))
	default:
		w.WriteHeader(404)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

// session runs the server over the lines given and returns its answers, one per request.
func session(t *testing.T, srv *Server, lines ...string) []map[string]any {
	t.Helper()
	var out strings.Builder
	if err := srv.Serve(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var answers []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("not JSON: %s", line)
		}
		answers = append(answers, m)
	}
	return answers
}

func newServer(t *testing.T) (*Server, *fakeList) {
	t.Helper()
	list := &fakeList{}
	web := httptest.NewServer(list)
	t.Cleanup(web.Close)
	client := tasksapi.New(tasksapi.Door{URL: web.URL, Key: "rk_test"})
	return New(&Tools{Client: client, Source: "w1:p1"}, "test"), list
}

func text(answer map[string]any) string {
	result, _ := answer["result"].(map[string]any)
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		return ""
	}
	return content[0].(map[string]any)["text"].(string)
}

func TestSessionInitializesListsAndCallsTools(t *testing.T) {
	srv, list := newServer(t)
	answers := session(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"x","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"projects_list","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"task_create","arguments":{"title":"Fix the thing","project":"Remuda","description":"It breaks.\n\nSee main.go.","column":"in progress","due":"2026-11-01","priority":3,"labels":["bug"],"fields":{"estimate":2}}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"tasks_list","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"task_update","arguments":{"id":1,"done":true}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"task_comment","arguments":{"id":1,"body":"On it."}}}`,
		`{"jsonrpc":"2.0","id":8,"method":"ping"}`,
	)
	if len(answers) != 8 {
		t.Fatalf("%d answers to 8 requests and a notification", len(answers))
	}
	init := answers[0]["result"].(map[string]any)
	if init["protocolVersion"] != "2025-03-26" || init["serverInfo"].(map[string]any)["name"] != "remuda" || init["instructions"] == "" {
		t.Errorf("initialize: %v", init)
	}
	tools := answers[1]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 7 || tools[0].(map[string]any)["name"] != "tasks_list" {
		t.Errorf("tools/list: %d tools, first %v", len(tools), tools[0])
	}
	if !strings.Contains(text(answers[2]), "**remuda** (id 1): columns New → In progress; fields Estimate (number)") {
		t.Errorf("projects_list: %q", text(answers[2]))
	}
	if !strings.HasPrefix(text(answers[3]), "Created task #1 (id 1) \"Fix the thing\"") {
		t.Errorf("task_create: %q", text(answers[3]))
	}
	created := list.tasks[0]
	if created["status_id"] != float64(11) || created["priority"] != float64(3) || created["source"] != "w1:p1" || created["markdown"] != "It breaks.\n\nSee main.go." || created["description"] != "<p>It breaks.</p><p>See main.go.</p>" {
		t.Errorf("the body sent: %v", created)
	}
	if fields := created["fields"].(map[string]any); fields["5"] != float64(2) {
		t.Errorf("the field by title → id: %v", created["fields"])
	}
	if due := created["due_date"].(string); !strings.HasPrefix(due, "2026-11-01") && !strings.HasPrefix(due, "2026-11-02") {
		t.Errorf("due: %s", due)
	}
	if len(list.labels) != 1 || list.labels[0]["title"] != "bug" {
		t.Errorf("the missing label was not made: %v", list.labels)
	}
	if got := text(answers[4]); !strings.Contains(got, "| 1 | remuda | In progress | Fix the thing |") || !strings.Contains(got, "| high | bug | w1:p1 |") {
		t.Errorf("tasks_list: %q", got)
	}
	if got := text(answers[5]); !strings.Contains(got, "# #1 Fix the thing") || !strings.Contains(got, "(done)") || !strings.Contains(got, "It breaks.") {
		t.Errorf("task_update → task_get: %q", got)
	}
	if text(answers[6]) != "Commented on task 1 (comment 1)" || list.comments[0]["body"] != "On it." {
		t.Errorf("task_comment: %q %v", text(answers[6]), list.comments)
	}
	if _, ok := answers[7]["result"].(map[string]any); !ok {
		t.Errorf("ping: %v", answers[7])
	}
}

func TestRefusalsAndUnknowns(t *testing.T) {
	srv, _ := newServer(t)
	answers := session(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"nothing/here"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"task_fly","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"task_create","arguments":{"title":"x","project":"nope"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"task_create","arguments":{"title":"x","project":"remuda","column":"Done"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"task_get","arguments":{"id":42}}}`,
		`not json at all`,
		`{"jsonrpc":"2.0","id":7,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`,
	)
	if len(answers) != 7 {
		t.Fatalf("%d answers", len(answers))
	}
	code := func(a map[string]any) float64 { return a["error"].(map[string]any)["code"].(float64) }
	if code(answers[0]) != codeMethodNotFound {
		t.Errorf("unknown method: %v", answers[0])
	}
	if code(answers[1]) != codeInvalidParams {
		t.Errorf("unknown tool: %v", answers[1])
	}
	for i, want := range map[int]string{2: `no project "nope"; the projects: remuda (id 1)`, 3: `has no column "Done"; its columns: New → In progress`, 4: "not found: no such task"} {
		result := answers[i]["result"].(map[string]any)
		if result["isError"] != true || !strings.Contains(text(answers[i]), want) {
			t.Errorf("answer %d: %v", i, answers[i])
		}
	}
	if code(answers[5]) != codeParse || answers[5]["id"] != nil {
		t.Errorf("a parse error answers with a null id: %v", answers[5])
	}
	if answers[6]["result"].(map[string]any)["protocolVersion"] != Version {
		t.Errorf("an unknown protocol version gets ours: %v", answers[6])
	}
}

func TestDueAndParagraphs(t *testing.T) {
	if _, err := dueOf("tomorrow"); err == nil {
		t.Error("a word is not a date")
	}
	if got, _ := dueOf("2026-01-02T03:04:05+02:00"); got != "2026-01-02T01:04:05Z" {
		t.Errorf("a moment kept in UTC: %s", got)
	}
	if got := paragraphsHTML("a <b>\nb\n\n```go\nx := 1\n```\n\nc"); got != "<p>a &lt;b&gt;<br>b</p><pre>x := 1</pre><p>c</p>" {
		t.Errorf("paragraphs: %s", got)
	}
}
