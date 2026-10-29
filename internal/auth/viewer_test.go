package auth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// viewerGuard is a guard over fresh device and key stores whose handler says who the request came from.
func viewerGuard(t *testing.T) (*Guard, http.Handler) {
	t.Helper()
	dir := t.TempDir()
	devices, err := OpenDevices(dir)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := OpenKeys(dir)
	if err != nil {
		t.Fatal(err)
	}
	g := NewGuard(testToken, devices, keys)
	return g, g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		viewer := "no"
		if IsViewer(r.Context()) {
			viewer = "yes"
		}
		io.WriteString(w, "device="+DeviceID(r.Context())+" viewer="+viewer)
	}))
}

func call(h http.Handler, method, path string, cookie *http.Cookie, bearer string) (int, string, *http.Response) {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "192.168.1.30:5000"
	req.Header.Set("User-Agent", chromeMac)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()
	return rec.Code, strings.TrimSpace(rec.Body.String()), res
}

func TestViewerKeyReadsAndNeverTypes(t *testing.T) {
	g, h := viewerGuard(t)
	key, rec, err := g.Keys.MintViewer("the office TV")
	if err != nil || !strings.HasPrefix(key, ViewerPrefix) || rec.Scope != ScopeViewer {
		t.Fatalf("mint: %v %q %+v", err, key, rec)
	}
	if code, body, _ := call(h, "GET", "/api/roster", nil, key); code != 200 || body != "device= viewer=yes" {
		t.Fatalf("roster with the key: %d %s", code, body)
	}
	for _, try := range []struct{ method, path string }{{"POST", "/api/keys"}, {"POST", "/api/submit"}, {"GET", "/ws/pane"}, {"GET", "/api/tasks"}, {"GET", "/api/devices"}, {"DELETE", "/api/sessions/w1"}, {"GET", "/code/local/"}, {"POST", "/"}} {
		if code, body, _ := call(h, try.method, try.path, nil, key); code != 403 || body != "this link is for viewing only" {
			t.Fatalf("%s %s with the key: %d %s", try.method, try.path, code, body)
		}
	}
	for _, path := range []string{"/api/chat", "/api/screen", "/api/scrollback", "/api/feed", "/api/standup", "/ws/events", "/", "/assets/index-abc.js", "/icons/icon-512.png"} {
		if code, _, _ := call(h, "GET", path, nil, key); code != 200 {
			t.Fatalf("GET %s with the key: %d", path, code)
		}
	}
	if !rec.Allows("GET", "/api/roster") || rec.Allows("POST", "/api/roster") || rec.Allows("GET", "/api/tasks") {
		t.Fatal("Allows for the viewer scope")
	}
}

func TestViewerLinkSignsAScreenInReadOnly(t *testing.T) {
	g, h := viewerGuard(t)
	key, rec, _ := g.Keys.MintViewer("the office TV")

	code, _, res := call(h, "GET", "/?viewer="+key+"&mode=wall", nil, "")
	if code != http.StatusSeeOther || res.Header.Get("Location") != "/?mode=wall" {
		t.Fatalf("the link: %d %s", code, res.Header.Get("Location"))
	}
	screen := cookieOf(t, res)
	if !IsDeviceCookie(screen.Value) {
		t.Fatalf("the cookie: %q", screen.Value)
	}
	if code, body, _ := call(h, "GET", "/api/chat", screen, ""); code != 200 || !strings.HasSuffix(body, "viewer=yes") || !strings.Contains(body, "device=") {
		t.Fatalf("chat with the screen's cookie: %d %s", code, body)
	}
	if code, body, _ := call(h, "POST", "/api/submit", screen, ""); code != 403 || body != "this link is for viewing only" {
		t.Fatalf("submit with the screen's cookie: %d %s", code, body)
	}
	devices := g.Devices.List()
	if len(devices) != 1 || !devices[0].Viewer || devices[0].Key != rec.ID || devices[0].Kind() != "viewer · Chrome on macOS" || devices[0].Name != "viewer · Chrome on macOS" {
		t.Fatalf("the device: %+v", devices)
	}

	// the link again on a screen already signed in keeps its device
	code, _, res = call(h, "GET", "/?viewer="+key, screen, "")
	if code != http.StatusSeeOther || len(res.Cookies()) != 0 || len(g.Devices.List()) != 1 {
		t.Fatalf("the link twice: %d %d cookies, %d devices", code, len(res.Cookies()), len(g.Devices.List()))
	}

	// a wrong key is refused like a wrong token
	if code, _, _ := call(h, "GET", "/?viewer="+ViewerPrefix+"0000", nil, ""); code != http.StatusUnauthorized {
		t.Fatalf("a wrong link: %d", code)
	}

	// the key revoked: the screen's cookie is stale at once, and dropped
	if !g.Keys.Revoke(rec.ID) {
		t.Fatal("revoke")
	}
	code, _, res = call(h, "GET", "/api/roster", screen, "")
	if code != http.StatusUnauthorized || cookieOf(t, res).MaxAge != -1 {
		t.Fatalf("after the revoke: %d, cookie %+v", code, cookieOf(t, res))
	}
}

func TestAFullDeviceOpeningAViewerLinkStaysFull(t *testing.T) {
	g, h := viewerGuard(t)
	key, _, _ := g.Keys.MintViewer("the office TV")
	_, _, res := call(h, "GET", "/?token="+testToken, nil, "")
	mine := cookieOf(t, res)
	code, _, res := call(h, "GET", "/?viewer="+key, mine, "")
	if code != http.StatusSeeOther || len(res.Cookies()) != 0 {
		t.Fatalf("the link on a full device: %d %d cookies", code, len(res.Cookies()))
	}
	if code, body, _ := call(h, "POST", "/api/submit", mine, ""); code != 200 || !strings.HasSuffix(body, "viewer=no") {
		t.Fatalf("still full: %d %s", code, body)
	}
}
