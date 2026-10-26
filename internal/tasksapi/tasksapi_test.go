package tasksapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindDoorPrefersTheEnvironmentThenTheFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REMUDA_STATE_DIR", dir)
	t.Setenv(EnvKey, "")
	t.Setenv(EnvURL, "")
	if _, err := FindDoor(); err == nil || !strings.Contains(err.Error(), KeyFile) {
		t.Fatalf("without a key the door opens: %v", err)
	}
	os.WriteFile(filepath.Join(dir, KeyFile), []byte("rk_abc\n"), 0o600)
	d, err := FindDoor()
	if err != nil || d.Key != "rk_abc" || d.URL != DefaultURL {
		t.Fatalf("the key file alone: %+v %v", d, err)
	}
	os.WriteFile(filepath.Join(dir, URLFile), []byte("http://127.0.0.1:7318/\n"), 0o600)
	if d, _ = FindDoor(); d.URL != "http://127.0.0.1:7318" {
		t.Errorf("the url file, trimmed: %q", d.URL)
	}
	t.Setenv(EnvKey, "rk_env")
	t.Setenv(EnvURL, "http://127.0.0.1:7443")
	if d, _ = FindDoor(); d.Key != "rk_env" || d.URL != "http://127.0.0.1:7443" {
		t.Errorf("the environment wins: %+v", d)
	}
}

func TestListSendsTheQueryAndTheKey(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.Write([]byte(`{"tasks":[{"id":3,"title":"Fix the thing","source":"GOG · w3:p1"}],"pages":2}`))
	}))
	defer srv.Close()
	c := New(Door{URL: srv.URL, Key: "rk_test"})
	ts, pages, err := c.List(Query{View: "today", Project: 7, Done: true, Search: "thing", Page: 2, TZ: "Europe/Kyiv"})
	if err != nil || pages != 2 || len(ts) != 1 || ts[0].ID != 3 || ts[0].Source != "GOG · w3:p1" {
		t.Fatalf("list: %v %d %+v", err, pages, ts)
	}
	q := got.URL.Query()
	if got.Header.Get("Authorization") != "Bearer rk_test" || q.Get("view") != "today" || q.Get("project") != "7" || q.Get("done") != "1" || q.Get("q") != "thing" || q.Get("page") != "2" || q.Get("tz") != "Europe/Kyiv" {
		t.Errorf("the request: %s %s", got.Header.Get("Authorization"), got.URL.RawQuery)
	}
}

func TestCreateSendsOnlyWhatIsSet(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		w.Write([]byte(`{"id":9,"identifier":"#9","title":"T"}`))
	}))
	defer srv.Close()
	c := New(Door{URL: srv.URL, Key: "k"})
	title := "T"
	task, err := c.Create(2, Write{Title: &title, Markdown: "# hi", Source: "w1:p1"})
	if err != nil || task.ID != 9 {
		t.Fatalf("create: %v %+v", err, task)
	}
	if body["project_id"].(float64) != 2 || body["title"] != "T" || body["markdown"] != "# hi" || body["source"] != "w1:p1" {
		t.Errorf("body: %v", body)
	}
	for _, absent := range []string{"done", "due_date", "priority", "status_id", "description"} {
		if _, ok := body[absent]; ok {
			t.Errorf("%s was sent unset", absent)
		}
	}
}

func TestRefusalsBecomeErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tasks/404":
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"not found: task 404"}`))
		default:
			w.WriteHeader(401)
			w.Write([]byte("unauthorized\n"))
		}
	}))
	defer srv.Close()
	c := New(Door{URL: srv.URL, Key: "k"})
	if _, err := c.Get(404); !IsNotFound(err) || err.Error() != "not found: task 404" {
		t.Errorf("404: %v", err)
	}
	if _, err := c.Labels(); err == nil || !strings.Contains(err.Error(), "refused the key") {
		t.Errorf("401: %v", err)
	}
}
