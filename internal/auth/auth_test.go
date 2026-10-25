package auth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const testToken = "0123456789abcdef0123456789abcdef01234567"
const chromeMac = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"

// newGuard is a guard over a fresh device store whose handler answers with the device the request came from.
func newGuard(t *testing.T) (*Guard, http.Handler) {
	t.Helper()
	devices, err := OpenDevices(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	g := NewGuard(testToken, devices)
	return g, g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "device="+DeviceID(r.Context()))
	}))
}

func cookieOf(t *testing.T, res *http.Response) *http.Cookie {
	t.Helper()
	for _, c := range res.Cookies() {
		if c.Name == CookieName {
			return c
		}
	}
	t.Fatalf("no %s cookie in %v", CookieName, res.Header)
	return nil
}

func get(h http.Handler, path string, cookie *http.Cookie, agent string) *http.Response {
	req := httptest.NewRequest("GET", path, nil)
	req.RemoteAddr = "192.168.1.20:51234"
	if agent != "" {
		req.Header.Set("User-Agent", agent)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func TestPairingLinkMintsDevice(t *testing.T) {
	g, h := newGuard(t)
	res := get(h, "/?token="+testToken+"&machine=local&pane=w1:p1", nil, chromeMac)
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("status %d, want 303", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); strings.Contains(loc, "token") || !strings.Contains(loc, "pane=w1%3Ap1") {
		t.Fatalf("location %q should keep the pane and lose the token", loc)
	}
	c := cookieOf(t, res)
	if !IsDeviceCookie(c.Value) || c.Value == testToken {
		t.Fatalf("cookie %q should name a device, not hold the token", c.Value)
	}
	if !c.HttpOnly || c.MaxAge <= 0 {
		t.Fatalf("cookie should be HttpOnly and kept: %+v", c)
	}
	list := g.Devices.List()
	if len(list) != 1 || list[0].Name != "Chrome on macOS" || list[0].IP != "192.168.1.20" {
		t.Fatalf("devices after pairing: %+v", list)
	}
	res = get(h, "/api/roster", c, chromeMac)
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || string(body) != "device="+list[0].ID {
		t.Fatalf("with the device cookie: %d %q", res.StatusCode, body)
	}
	// the link opened again by a signed-in browser mints no second device
	res = get(h, "/?token="+testToken, c, chromeMac)
	if res.StatusCode != http.StatusSeeOther || len(g.Devices.List()) != 1 {
		t.Fatalf("a second pairing by the same browser: %d, %d devices", res.StatusCode, len(g.Devices.List()))
	}
}

func TestLoginFormMintsDevice(t *testing.T) {
	g, _ := newGuard(t)
	form := url.Values{"token": {testToken}}
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1")
	rec := httptest.NewRecorder()
	g.HandleLogin(rec, req)
	res := rec.Result()
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/" {
		t.Fatalf("login: %d %q", res.StatusCode, res.Header.Get("Location"))
	}
	c := cookieOf(t, res)
	if _, ok := g.Devices.Check(c.Value); !ok {
		t.Fatal("the login cookie does not check")
	}
	if list := g.Devices.List(); len(list) != 1 || list[0].Name != "Safari on iPhone" {
		t.Fatalf("devices after login: %+v", list)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/login", strings.NewReader(url.Values{"token": {"wrong"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	g.HandleLogin(rec, req)
	if rec.Code != http.StatusForbidden || len(g.Devices.List()) != 1 {
		t.Fatalf("a wrong token: %d, %d devices", rec.Code, len(g.Devices.List()))
	}
}

func TestLegacyCookieUpgraded(t *testing.T) {
	g, h := newGuard(t)
	res := get(h, "/api/roster", &http.Cookie{Name: CookieName, Value: testToken}, chromeMac)
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || string(body) != "device=" {
		t.Fatalf("a legacy cookie should pass as no device: %d %q", res.StatusCode, body)
	}
	c := cookieOf(t, res)
	if !IsDeviceCookie(c.Value) {
		t.Fatalf("the response should carry a device cookie, got %q", c.Value)
	}
	if len(g.Devices.List()) != 1 {
		t.Fatalf("%d devices after the upgrade", len(g.Devices.List()))
	}
	res = get(h, "/api/roster", c, chromeMac)
	body, _ = io.ReadAll(res.Body)
	if res.StatusCode != 200 || string(body) != "device="+g.Devices.List()[0].ID {
		t.Fatalf("the upgraded cookie: %d %q", res.StatusCode, body)
	}
	if len(res.Cookies()) != 0 {
		t.Fatal("a device cookie is not set again on every response")
	}
}

func TestRevokedCookieRefusedAtOnce(t *testing.T) {
	g, h := newGuard(t)
	cookie, device, err := g.Devices.Mint("10.0.0.2", chromeMac)
	if err != nil {
		t.Fatal(err)
	}
	c := &http.Cookie{Name: CookieName, Value: cookie}
	if res := get(h, "/api/roster", c, ""); res.StatusCode != 200 {
		t.Fatalf("before revoking: %d", res.StatusCode)
	}
	if !g.Devices.Revoke(device.ID) || g.Devices.Revoke(device.ID) {
		t.Fatal("revoke should succeed once")
	}
	start := time.Now()
	res := get(h, "/api/roster", c, "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after revoking: %d", res.StatusCode)
	}
	if time.Since(start) > 300*time.Millisecond {
		t.Fatal("a stale device cookie must be answered at once")
	}
	if dropped := cookieOf(t, res); dropped.MaxAge >= 0 || dropped.Value != "" {
		t.Fatalf("the stale cookie should be dropped: %+v", dropped)
	}
	res = get(h, "/", c, "")
	if res.StatusCode != http.StatusUnauthorized || res.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("the page with a stale cookie should be the sign-in page: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
}

func TestWrongCookiesAndBearer(t *testing.T) {
	g, h := newGuard(t)
	for _, value := range []string{"garbage", "d.", "d.nosuch.secret", "d.x", strings.Repeat("a", 40)} {
		if res := get(h, "/api/roster", &http.Cookie{Name: CookieName, Value: value}, ""); res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("cookie %q: %d", value, res.StatusCode)
		}
	}
	if res := get(h, "/api/roster", nil, ""); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no cookie: %d", res.StatusCode)
	}
	if len(g.Devices.List()) != 0 {
		t.Fatal("a refused request must mint nothing")
	}
	req := httptest.NewRequest("GET", "/api/roster", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Body.String() != "device=" || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("bearer: %d %q cookies=%d", rec.Code, rec.Body.String(), len(rec.Result().Cookies()))
	}
	// a wrong secret for a real id
	cookie, _, _ := g.Devices.Mint("", "")
	id := strings.Split(cookie, ".")[1]
	if res := get(h, "/api/roster", &http.Cookie{Name: CookieName, Value: "d." + id + ".0000"}, ""); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong secret: %d", res.StatusCode)
	}
}

func TestDeviceName(t *testing.T) {
	cases := map[string]string{
		chromeMac: "Chrome on macOS",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1": "Safari on iPhone",
		"Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0":                                                                  "Firefox on Linux",
		"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36":                   "Chrome on Android",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0":           "Edge on Windows",
		"Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/130.0.0.0 Mobile/15E148 Safari/604.1":       "Chrome on iPad",
		"Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36":                          "Chrome on ChromeOS",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/130.0.0.0 Safari/537.36":           "Chrome on macOS",
		"curl/8.4.0":                    "a browser",
		"":                              "a browser",
		"Mozilla/5.0 (Windows NT 10.0)": "a browser on Windows",
	}
	for agent, want := range cases {
		if got := DeviceName(agent); got != want {
			t.Errorf("DeviceName(%q) = %q, want %q", agent, got, want)
		}
	}
}

func TestStoreReloadRenameAndRevokeOthers(t *testing.T) {
	dir := t.TempDir()
	devices, err := OpenDevices(dir)
	if err != nil {
		t.Fatal(err)
	}
	first, a, _ := devices.Mint("10.0.0.1", chromeMac)
	second, b, _ := devices.Mint("10.0.0.2", "")
	if _, err := devices.Rename(a.ID, "  Sergii's laptop  "); err != nil {
		t.Fatal(err)
	}
	if _, err := devices.Rename("nosuch", "x"); err != ErrNoDevice {
		t.Fatalf("renaming an unknown device: %v", err)
	}
	again, err := OpenDevices(dir)
	if err != nil {
		t.Fatal(err)
	}
	list := again.List()
	if len(list) != 2 {
		t.Fatalf("%d devices after a reload", len(list))
	}
	names := map[string]string{}
	for _, d := range list {
		names[d.ID] = d.Name
	}
	if names[a.ID] != "Sergii's laptop" || names[b.ID] != "a browser" {
		t.Fatalf("names after a reload: %v", names)
	}
	if _, ok := again.Check(first); !ok {
		t.Fatal("the first cookie should still check after a reload")
	}
	if renamed, _ := again.Rename(a.ID, ""); renamed.Name != "Chrome on macOS" {
		t.Fatalf("an empty name should fall back to the kind, got %q", renamed.Name)
	}
	if gone := again.RevokeOthers(a.ID); gone != 1 {
		t.Fatalf("RevokeOthers removed %d", gone)
	}
	if _, ok := again.Check(second); ok {
		t.Fatal("the second cookie should be refused after RevokeOthers")
	}
	if _, ok := again.Check(first); !ok {
		t.Fatal("the kept cookie should still check")
	}
	if b, err := os.ReadFile(filepath.Join(dir, devicesFile)); err != nil || strings.Contains(string(b), strings.Split(first, ".")[2]) {
		t.Fatal("the file must hold hashes, never a secret")
	}
}

func TestLastSeenBumpsOncePerMinute(t *testing.T) {
	devices, _ := OpenDevices(t.TempDir())
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	devices.now = func() time.Time { return now }
	cookie, device, _ := devices.Mint("", "")
	now = now.Add(30 * time.Second)
	if seen, _ := devices.Check(cookie); !seen.LastSeen.Equal(device.LastSeen) {
		t.Fatal("a check within the minute must not bump last_seen")
	}
	now = now.Add(31 * time.Second)
	if seen, _ := devices.Check(cookie); !seen.LastSeen.Equal(now) {
		t.Fatalf("a check past the minute should bump last_seen, got %v", seen.LastSeen)
	}
}

func TestConcurrentChecks(t *testing.T) {
	devices, _ := OpenDevices(t.TempDir())
	cookie, _, _ := devices.Mint("", chromeMac)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := devices.Check(cookie); !ok {
				t.Error("a concurrent check failed")
			}
			if i%10 == 0 {
				devices.Mint("", "")
				devices.List()
			}
		}()
	}
	wg.Wait()
}
