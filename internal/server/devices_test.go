package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/auth"
)

const deviceTestToken = "0123456789abcdef0123456789abcdef01234567"

// deviceServer is the device routes behind the guard over a fresh store, with two devices signed in.
func deviceServer(t *testing.T) (http.Handler, *auth.Devices, *http.Cookie, *http.Cookie) {
	t.Helper()
	devices, err := auth.OpenDevices(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Guard: auth.NewGuard(deviceTestToken, devices)}
	mux := http.NewServeMux()
	s.deviceRoutes(mux)
	mine, _, _ := devices.Mint("10.0.0.1", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36")
	other, _, _ := devices.Mint("10.0.0.2", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1")
	return s.Guard.Middleware(mux), devices, &http.Cookie{Name: auth.CookieName, Value: mine}, &http.Cookie{Name: auth.CookieName, Value: other}
}

func deviceCall(h http.Handler, method, path string, cookie *http.Cookie, body string) (*http.Response, []map[string]any) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()
	var list []map[string]any
	if strings.HasPrefix(strings.TrimSpace(rec.Body.String()), "[") {
		json.Unmarshal(rec.Body.Bytes(), &list)
	}
	return res, list
}

func TestDevicesListMarksThisOne(t *testing.T) {
	h, _, mine, other := deviceServer(t)
	res, list := deviceCall(h, "GET", "/api/devices", mine, "")
	if res.StatusCode != 200 || len(list) != 2 {
		t.Fatalf("list: %d, %d devices", res.StatusCode, len(list))
	}
	this := 0
	for _, d := range list {
		if d["hash"] != nil {
			t.Fatal("the hash must never leave the server")
		}
		if d["this"] == true {
			this++
			if d["kind"] != "Chrome on macOS" {
				t.Fatalf("this device should be the Chrome one, got %v", d["kind"])
			}
		}
	}
	if this != 1 {
		t.Fatalf("%d devices marked as this one", this)
	}
	_ = other
	req := httptest.NewRequest("GET", "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer "+deviceTestToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var byBearer []map[string]any
	json.Unmarshal(rec.Body.Bytes(), &byBearer)
	for _, d := range byBearer {
		if d["this"] == true {
			t.Fatal("a bearer caller has no device of its own")
		}
	}
}

func TestDeviceRenameAndRevoke(t *testing.T) {
	h, devices, mine, other := deviceServer(t)
	_, list := deviceCall(h, "GET", "/api/devices", mine, "")
	var otherID string
	for _, d := range list {
		if d["this"] != true {
			otherID = d["id"].(string)
		}
	}
	res, _ := deviceCall(h, "PATCH", "/api/devices/"+otherID, mine, `{"name":"the phone"}`)
	if res.StatusCode != 200 {
		t.Fatalf("rename: %d", res.StatusCode)
	}
	if res, _ := deviceCall(h, "PATCH", "/api/devices/nosuch", mine, `{"name":"x"}`); res.StatusCode != 404 {
		t.Fatalf("renaming an unknown device: %d", res.StatusCode)
	}
	if res, _ := deviceCall(h, "DELETE", "/api/devices", mine, ""); res.StatusCode != 400 {
		t.Fatalf("DELETE without ?others=1: %d", res.StatusCode)
	}
	res, _ = deviceCall(h, "DELETE", "/api/devices/"+otherID, mine, "")
	if res.StatusCode != 200 || len(res.Cookies()) != 0 {
		t.Fatalf("revoking another device: %d, cookies %d", res.StatusCode, len(res.Cookies()))
	}
	if res, _ := deviceCall(h, "GET", "/api/devices", other, ""); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("the revoked device still gets in: %d", res.StatusCode)
	}
	if len(devices.List()) != 1 {
		t.Fatalf("%d devices left", len(devices.List()))
	}
	// the caller signs itself out: the cookie goes with the answer
	_, list = deviceCall(h, "GET", "/api/devices", mine, "")
	res, _ = deviceCall(h, "DELETE", "/api/devices/"+list[0]["id"].(string), mine, "")
	if res.StatusCode != 200 {
		t.Fatalf("self revoke: %d", res.StatusCode)
	}
	dropped := false
	for _, c := range res.Cookies() {
		if c.Name == auth.CookieName && c.MaxAge < 0 {
			dropped = true
		}
	}
	if !dropped {
		t.Fatal("signing out should drop the cookie")
	}
	if res, _ := deviceCall(h, "GET", "/api/devices", mine, ""); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after signing out: %d", res.StatusCode)
	}
}

func TestDevicesRevokeOthers(t *testing.T) {
	h, devices, mine, other := deviceServer(t)
	devices.Mint("10.0.0.3", "")
	res, _ := deviceCall(h, "DELETE", "/api/devices?others=1", mine, "")
	if res.StatusCode != 200 || len(devices.List()) != 1 {
		t.Fatalf("revoke others: %d, %d left", res.StatusCode, len(devices.List()))
	}
	if res, _ := deviceCall(h, "GET", "/api/devices", other, ""); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("another device still gets in: %d", res.StatusCode)
	}
	if res, _ := deviceCall(h, "GET", "/api/devices", mine, ""); res.StatusCode != 200 {
		t.Fatalf("this device should stay: %d", res.StatusCode)
	}
}
