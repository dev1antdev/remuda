package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/auth"
)

// viewerServer is the viewer-link and agent-key routes behind the guard, with one full device signed in.
func viewerServer(t *testing.T) (http.Handler, *Server, *http.Cookie) {
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
	s.viewerKeyRoutes(mux)
	s.agentKeyRoutes(mux)
	cookie, _, _ := devices.Mint("10.0.0.1", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36")
	return s.Guard.Middleware(mux), s, &http.Cookie{Name: auth.CookieName, Value: cookie}
}

func viewerCall(h http.Handler, method, path, body string, cookie *http.Cookie) (int, string) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Host = "cockpit.local:7317"
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, strings.TrimSpace(rec.Body.String())
}

func TestViewerLinksMintedListedAndRevokedWithTheirScreens(t *testing.T) {
	h, s, mine := viewerServer(t)
	if code, body := viewerCall(h, "POST", "/api/viewer-keys", `{"name":"  "}`, mine); code != 400 {
		t.Fatalf("a nameless link: %d %s", code, body)
	}
	code, body := viewerCall(h, "POST", "/api/viewer-keys", `{"name":"the office TV"}`, mine)
	if code != 200 {
		t.Fatalf("mint: %d %s", code, body)
	}
	var made struct{ ID, Name, Link string }
	if err := json.Unmarshal([]byte(body), &made); err != nil || made.Name != "the office TV" || !strings.HasPrefix(made.Link, "http://cockpit.local:7317/?viewer="+auth.ViewerPrefix) {
		t.Fatalf("the answer: %v %s", err, body)
	}
	key := strings.TrimPrefix(made.Link, "http://cockpit.local:7317/?viewer=")

	code, body = viewerCall(h, "GET", "/api/viewer-keys", "", mine)
	var list []map[string]any
	if err := json.Unmarshal([]byte(body), &list); code != 200 || err != nil || len(list) != 1 || list[0]["name"] != "the office TV" || list[0]["hash"] != nil {
		t.Fatalf("the list: %d %v %s", code, err, body)
	}
	if code, body := viewerCall(h, "GET", "/api/agent-keys", "", mine); code != 200 || body != "[]" {
		t.Fatalf("the agent keys should not list a viewer link: %d %s", code, body)
	}

	// a screen signed in by the link cannot reach these routes at all
	screen, _, err := s.Guard.Devices.MintViewer("10.0.0.9", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/130.0.0.0 Safari/537.36", made.ID)
	if err != nil {
		t.Fatal(err)
	}
	screenCookie := &http.Cookie{Name: auth.CookieName, Value: screen}
	if code, body := viewerCall(h, "GET", "/api/viewer-keys", "", screenCookie); code != 403 || body != "this link is for viewing only" {
		t.Fatalf("the links with a screen's cookie: %d %s", code, body)
	}
	if code, body := viewerCall(h, "POST", "/api/viewer-keys", `{"name":"x"}`, &http.Cookie{Name: auth.CookieName, Value: ""}); code != 401 {
		t.Fatalf("minting without a sign-in: %d %s", code, body)
	}
	_ = key

	code, body = viewerCall(h, "DELETE", "/api/viewer-keys/"+made.ID, "", mine)
	var gone struct {
		Revoked string
		Screens int
	}
	if err := json.Unmarshal([]byte(body), &gone); code != 200 || err != nil || gone.Revoked != made.ID || gone.Screens != 1 {
		t.Fatalf("revoke: %d %v %s", code, err, body)
	}
	if code, _ := viewerCall(h, "DELETE", "/api/viewer-keys/"+made.ID, "", mine); code != 404 {
		t.Fatalf("revoke twice: %d", code)
	}
	if len(s.Guard.Devices.List()) != 1 {
		t.Fatalf("the screen should be signed out with its link: %+v", s.Guard.Devices.List())
	}
}
