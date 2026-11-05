package preview

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// upstream is a dev server that says what it was asked, sets a cookie of remuda's name and one of its own, asks the
// browser to clear the site, redirects to its own address, and echoes on an upgraded connection
func upstream(t *testing.T, port int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ws":
			if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
				http.Error(w, "no upgrade", 400)
				return
			}
			conn, rw, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			defer conn.Close()
			fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
			rw.Flush()
			got, _ := rw.ReadString('\n')
			fmt.Fprintf(rw, "echo %s", got)
			rw.Flush()
		case "/away":
			http.Redirect(w, r, fmt.Sprintf("http://localhost:%d/landed?x=1", port), http.StatusFound)
		default:
			w.Header().Add("Set-Cookie", "remuda_token=stolen; Path=/")
			w.Header().Add("Set-Cookie", "app_session=abc; Path=/")
			w.Header().Set("Clear-Site-Data", `"cookies"`)
			fmt.Fprintf(w, "host=%s cookie=%s auth=%s origin=%s referer=%s", r.Host, r.Header.Get("Cookie"), r.Header.Get("Authorization"), r.Header.Get("Origin"), r.Header.Get("Referer"))
		}
	}))
}

func previewOf(t *testing.T, allow bool, port int) (*httptest.Server, *atomic.Int64, *atomic.Int64) {
	t.Helper()
	up := upstream(t, port)
	t.Cleanup(up.Close)
	var last, holds atomic.Int64
	addr := strings.TrimPrefix(up.URL, "http://")
	front := httptest.NewServer(Guarded(func(*http.Request) bool { return allow }, "https://remuda.example.ts.net", Proxy(func() (string, error) { return addr, nil }, port, "https://remuda.example.ts.net:3000", func() { last.Store(time.Now().UnixNano()) }, &holds)))
	t.Cleanup(front.Close)
	return front, &last, &holds
}

func TestAPreviewKeepsRemudasSecretsOnBothWays(t *testing.T) {
	front, last, _ := previewOf(t, true, 3000)
	req, _ := http.NewRequest("GET", front.URL+"/", nil)
	req.Header.Set("Cookie", "remuda_token=d.dev1.secret; app_session=abc")
	req.Header.Set("Authorization", "Bearer the-token")
	req.Header.Set("Origin", "https://remuda.example.ts.net:3000")
	req.Header.Set("Referer", "https://remuda.example.ts.net:3000/page")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if got := string(body); got != "host=localhost:3000 cookie=app_session=abc auth= origin=http://localhost:3000 referer=http://localhost:3000/page" {
		t.Fatalf("the dev server was asked %q", got)
	}
	if cookies := res.Header.Values("Set-Cookie"); len(cookies) != 1 || !strings.HasPrefix(cookies[0], "app_session=") {
		t.Fatalf("cookies back to the browser: %q", cookies)
	}
	if res.Header.Get("Clear-Site-Data") != "" {
		t.Fatal("Clear-Site-Data reached the browser: it would sign the person out of remuda")
	}
	if last.Load() == 0 {
		t.Fatal("the request was not noted for the idle closer")
	}
}

func TestAnotherOriginReachesTheDevServerAsItIs(t *testing.T) {
	front, _, _ := previewOf(t, true, 3000)
	for _, c := range []struct{ origin, referer, want string }{
		{"https://evil.example", "https://evil.example/x", "origin=https://evil.example referer="},
		{"https://remuda.example.ts.net:4000", "https://remuda.example.ts.net:30001/x", "origin=https://remuda.example.ts.net:4000 referer="},
		{"HTTPS://REMUDA.EXAMPLE.TS.NET:3000", "https://remuda.example.ts.net:3000", "origin=http://localhost:3000 referer=http://localhost:3000"},
	} {
		req, _ := http.NewRequest("GET", front.URL+"/", nil)
		req.Header.Set("Origin", c.origin)
		req.Header.Set("Referer", c.referer)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if !strings.HasSuffix(string(body), c.want) {
			t.Errorf("Origin %q, Referer %q reached the dev server as %q, want …%q", c.origin, c.referer, body, c.want)
		}
	}
}

func TestAPreviewIsTheSignedInBrowsersOnly(t *testing.T) {
	front, _, _ := previewOf(t, false, 3000)
	res, err := http.Get(front.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized || !strings.Contains(string(body), `href="https://remuda.example.ts.net"`) || strings.Contains(string(body), "host=") {
		t.Fatalf("a stranger got %d %q", res.StatusCode, body)
	}
	if strings.Contains(string(body), "<form") || strings.Contains(string(body), "<input") {
		t.Fatal("the preview's port asks for the token itself")
	}
}

func TestARedirectToTheDevServersOwnAddressStaysInThePreview(t *testing.T) {
	front, _, _ := previewOf(t, true, 3000)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Get(front.URL + "/away")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if loc := res.Header.Get("Location"); loc != "https://remuda.example.ts.net:3000/landed?x=1" {
		t.Fatalf("Location %q", loc)
	}
	for loc, want := range map[string]string{
		"https://example.com/x":    "https://example.com/x",
		"/relative":                "/relative",
		"http://localhost:4000/x":  "http://localhost:4000/x",
		"http://127.0.0.1:3000/y?": "https://remuda.example.ts.net:3000/y?",
	} {
		if got := ownLocation(loc, "localhost:3000", "https://remuda.example.ts.net:3000"); got != want {
			t.Errorf("ownLocation(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestAnUpgradedConnectionPassesAndHoldsThePreviewOpen(t *testing.T) {
	front, _, holds := previewOf(t, true, 3000)
	conn, err := net.Dial("tcp", strings.TrimPrefix(front.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "GET /ws HTTP/1.1\r\nHost: remuda.example.ts.net:3000\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: x\r\nSec-WebSocket-Version: 13\r\n\r\n")
	reader := bufio.NewReader(conn)
	status, _ := reader.ReadString('\n')
	if !strings.Contains(status, "101") {
		t.Fatalf("the upgrade answered %q", status)
	}
	for {
		header, _ := reader.ReadString('\n')
		if header == "\r\n" || header == "" {
			break
		}
	}
	if holds.Load() != 1 {
		t.Fatalf("an open upgraded connection holds %d", holds.Load())
	}
	fmt.Fprintf(conn, "hello\n")
	echo, _ := reader.ReadString('\n')
	if echo != "echo hello\n" {
		t.Fatalf("the upgraded connection echoed %q", echo)
	}
}
