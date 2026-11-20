package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/core/coretest"
	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/radar"
	"github.com/dev1antdev/remuda/internal/scratch"
	"github.com/dev1antdev/remuda/internal/watch"
)

// ticker prints a line every 100 ms; flood prints as fast as it can.
var (
	ticker = []string{"sh", "-c", "while :; do echo tick; sleep 0.1; done"}
	flood  = []string{"yes", "tick"}
)

// coreTerminalServer is a remuda on remuda-core in front of a throwaway core with one terminal running argv: the routes
// of the live terminal behind the real guard, its token, its agent keys and the terminal.
func coreTerminalServer(t *testing.T, ctx context.Context, argv []string) (srv *httptest.Server, token string, keys *auth.Keys, terminal string) {
	c := coretest.Server(t)
	for _, kv := range c.Env {
		key, value, _ := strings.Cut(kv, "=")
		t.Setenv(key, value)
	}
	t.Setenv("PATH", filepath.Dir(c.Bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	state := t.TempDir()
	fleet := machines.NewCoreManager(state, "", nil, false)
	t.Cleanup(fleet.Close)
	mc, err := fleet.Get(machines.LocalID)
	if err != nil {
		t.Fatal(err)
	}
	ticking, err := c.Client(t).Run(ctx, core.RunParams{Argv: argv, Size: &core.Size{Cols: 100, Rows: 30}})
	if err != nil {
		t.Fatal(err)
	}
	watcher := watch.New(fleet, events.NewBus())
	for start := time.Now(); ; time.Sleep(50 * time.Millisecond) {
		if snap, _ := watcher.Refresh(mc); snap != nil && snap.Pane(ticking.Address) != nil {
			break
		}
		if time.Since(start) > 10*time.Second {
			t.Fatal("the watcher never saw the terminal")
		}
	}
	keys, err = auth.OpenKeys(state)
	if err != nil {
		t.Fatal(err)
	}
	devices, err := auth.OpenDevices(state)
	if err != nil {
		t.Fatal(err)
	}
	token = strings.Repeat("t", 40)
	guard := auth.NewGuard(token, devices, keys)
	s := &Server{Machines: fleet, Watch: watcher, Guard: guard, Keys: keys, Scratch: scratch.Open(state), Radar: radar.New(state, nil)}
	mux := http.NewServeMux()
	s.terminalRoutes(mux)
	srv = httptest.NewServer(guard.Middleware(s.viewerOwnGuard(mux)))
	t.Cleanup(srv.Close)
	return srv, token, keys, ticking.Address
}

// A viewer link watching a live terminal on remuda-core is cut off once the link is revoked, whatever the terminal
// prints next; the first thing it is told is the terminal's grid, a text frame of its own.
func TestARevokedViewerLinkSeesNoMoreOfTheTerminal(t *testing.T) {
	for name, argv := range map[string][]string{"a line every 100 ms": ticker, "a flood of output": flood} {
		t.Run(name, func(t *testing.T) { revokedWhileWatching(t, argv) })
	}
}

// revokedWhileWatching revokes a viewer link while it watches a terminal running argv: the socket ends within a second
// with the reason, however fast the terminal prints (a frame gathers at most maxFrame bytes before the link is asked).
func revokedWhileWatching(t *testing.T, argv []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	srv, _, keys, terminal := coreTerminalServer(t, ctx, argv)

	link, rec, err := keys.MintViewer("the office TV")
	if err != nil {
		t.Fatal(err)
	}
	address := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/pane?machine=local&id=" + terminal + "&cols=80&rows=24"
	conn, _, err := websocket.Dial(ctx, address, &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + link}}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	if typ, first, err := conn.Read(ctx); err != nil || typ != websocket.MessageText || string(first) != `{"cols":100,"rows":30,"type":"grid"}` {
		t.Fatalf("the link's first frame: %v %q %v", typ, first, err)
	}
	var seen strings.Builder
	for !strings.Contains(seen.String(), "tick") {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("the link never saw the terminal: %v", err)
		}
		seen.Write(data)
	}

	keys.Revoke(rec.ID)
	revokedAt := time.Now()
	readCtx, readCancel := context.WithTimeout(ctx, 3*time.Second)
	defer readCancel()
	for {
		_, _, err := conn.Read(readCtx)
		if err == nil {
			continue // a write already under way when the link went
		}
		if websocket.CloseStatus(err) != websocket.StatusPolicyViolation || !strings.Contains(err.Error(), "this link was revoked") {
			t.Fatalf("after the revocation the socket ended with %v", err)
		}
		if took := time.Since(revokedAt); took > time.Second {
			t.Fatalf("a terminal kept a revoked link for %v", took)
		}
		return
	}
}

// Over the HTTP transport a viewer that keeps the size is told on every read the grid its bytes are drawn at, so a read
// lost on the way loses no grid: after the terminal is resized, every read carries the new one.
func TestAKeepViewerOverHTTPIsToldTheGridOnEveryRead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	srv, token, _, terminal := coreTerminalServer(t, ctx, ticker)
	call := func(method, path string) *http.Response {
		req, _ := http.NewRequestWithContext(ctx, method, srv.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	open := func(query string) string {
		res := call("POST", "/api/term/open?machine=local&id="+terminal+"&"+query)
		defer res.Body.Close()
		var answer struct{ SID string }
		if err := json.NewDecoder(res.Body).Decode(&answer); err != nil || answer.SID == "" {
			t.Fatalf("open %s: %d %v", query, res.StatusCode, err)
		}
		return answer.SID
	}
	gridOfRead := func(sid string) string {
		res := call("GET", "/api/term/read?sid="+sid+"&wait=2")
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
		return res.Header.Get("X-Remuda-Grid")
	}
	phone := open("cols=40&rows=10&keep=1")
	if grid := gridOfRead(phone); grid != "100x30" {
		t.Fatalf("the first read's grid %q", grid)
	}
	desk := open("cols=100&rows=30")
	call("POST", "/api/term/resize?sid="+desk+"&cols=90&rows=28").Body.Close()
	for start := time.Now(); gridOfRead(phone) != "90x28"; {
		if time.Since(start) > 10*time.Second {
			t.Fatal("the phone's reads never carried the new grid")
		}
	}
	for range 3 { // a read the phone never got is no grid lost: the next ones carry it still
		if grid := gridOfRead(phone); grid != "90x28" {
			t.Fatalf("a later read's grid %q", grid)
		}
	}
	for _, sid := range []string{phone, desk} {
		call("DELETE", "/api/term/close?sid="+sid).Body.Close()
	}
}
