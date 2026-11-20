package server

import (
	"context"
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
	"github.com/dev1antdev/remuda/internal/scratch"
	"github.com/dev1antdev/remuda/internal/watch"
)

// A viewer link watching a live terminal on remuda-core is cut off once the link is revoked, whatever the terminal
// prints next.
func TestARevokedViewerLinkSeesNoMoreOfTheTerminal(t *testing.T) {
	c := coretest.Server(t)
	for _, kv := range c.Env {
		key, value, _ := strings.Cut(kv, "=")
		t.Setenv(key, value)
	}
	t.Setenv("PATH", filepath.Dir(c.Bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	state := t.TempDir()
	fleet := machines.NewCoreManager(state, "", nil, false)
	defer fleet.Close()
	mc, err := fleet.Get(machines.LocalID)
	if err != nil {
		t.Fatal(err)
	}
	ticking, err := c.Client(t).Run(ctx, core.RunParams{Argv: []string{"sh", "-c", "while :; do echo tick; sleep 0.1; done"}})
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
	keys, err := auth.OpenKeys(state)
	if err != nil {
		t.Fatal(err)
	}
	devices, err := auth.OpenDevices(state)
	if err != nil {
		t.Fatal(err)
	}
	guard := auth.NewGuard(strings.Repeat("t", 40), devices, keys)
	s := &Server{Machines: fleet, Watch: watcher, Guard: guard, Keys: keys, Scratch: scratch.Open(state)}
	mux := http.NewServeMux()
	s.terminalRoutes(mux)
	srv := httptest.NewServer(guard.Middleware(s.viewerOwnGuard(mux)))
	defer srv.Close()

	link, rec, err := keys.MintViewer("the office TV")
	if err != nil {
		t.Fatal(err)
	}
	address := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/pane?machine=local&id=" + ticking.Address + "&cols=80&rows=24"
	conn, _, err := websocket.Dial(ctx, address, &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + link}}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
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
			t.Fatalf("a terminal printing every 100 ms kept a revoked link for %v", took)
		}
		return
	}
}
