package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/auth"
)

// keyServer is the key routes and a stand-in task route behind the guard, with one device signed in and one key minted.
func keyServer(t *testing.T) (http.Handler, *auth.Keys, *http.Cookie, string) {
	t.Helper()
	dir := t.TempDir()
	devices, err := auth.OpenDevices(dir)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := auth.OpenKeys(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Guard: auth.NewGuard(deviceTestToken, devices, keys), Keys: keys}
	mux := http.NewServeMux()
	s.agentKeyRoutes(mux)
	mux.HandleFunc("GET /api/tasks", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "by "+auth.CallerName(r.Context())) })
	cookie, _, _ := devices.Mint("10.0.0.1", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36")
	key, _, _ := keys.Mint("GOG")
	return s.Guard.Middleware(mux), keys, &http.Cookie{Name: auth.CookieName, Value: cookie}, key
}

func keyCall(h http.Handler, method, path string, cookie *http.Cookie, bearer string) (int, string) {
	req := httptest.NewRequest(method, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, strings.TrimSpace(rec.Body.String())
}

func TestKeysListedWithoutTheirHashAndRevoked(t *testing.T) {
	h, _, mine, key := keyServer(t)
	code, body := keyCall(h, "GET", "/api/agent-keys", mine, "")
	if code != 200 {
		t.Fatalf("list: %d %s", code, body)
	}
	var list []map[string]any
	if err := json.Unmarshal([]byte(body), &list); err != nil || len(list) != 1 {
		t.Fatalf("list: %v %s", err, body)
	}
	if list[0]["name"] != "GOG" || list[0]["scope"] != "tasks" || list[0]["hash"] != nil {
		t.Fatalf("the view: %v", list[0])
	}

	if code, body := keyCall(h, "GET", "/api/tasks", nil, key); code != 200 || body != "by GOG" {
		t.Fatalf("tasks with the key: %d %s", code, body)
	}
	if code, body := keyCall(h, "GET", "/api/agent-keys", nil, key); code != 403 || body != "this key is for tasks only" {
		t.Fatalf("the keys with a key: %d %s", code, body)
	}

	id, _ := list[0]["id"].(string)
	if code, _ := keyCall(h, "DELETE", "/api/agent-keys/"+id, mine, ""); code != 200 {
		t.Fatalf("revoke: %d", code)
	}
	if code, _ := keyCall(h, "DELETE", "/api/agent-keys/"+id, mine, ""); code != 404 {
		t.Fatalf("revoke twice: %d", code)
	}
	if code, _ := keyCall(h, "GET", "/api/tasks", nil, key); code != 401 {
		t.Fatalf("a revoked key on tasks: %d", code)
	}
	if code, body := keyCall(h, "GET", "/api/agent-keys", mine, ""); code != 200 || body != "[]" {
		t.Fatalf("the list after: %d %s", code, body)
	}
}

func TestKeysNeedAStore(t *testing.T) {
	s := &Server{}
	mux := http.NewServeMux()
	s.agentKeyRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/agent-keys", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("without a store: %d", rec.Code)
	}
}
