package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/feed"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/herdr/herdrtest"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/watch"
)

// TestTheScreenTerminalTypesIntoAThrowawayHerdr is POST /api/input against herdr itself: a command typed a few
// keystrokes at a time and sent with Enter, read back off GET /api/screen, and one row in the activity for the line.
func TestTheScreenTerminalTypesIntoAThrowawayHerdr(t *testing.T) {
	sock := herdrtest.Server(t)
	client := herdr.NewClient(sock)
	dir, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	made, err := client.WorkspaceCreate(dir, "screen terminal")
	if err != nil {
		t.Fatal(err)
	}
	defer client.WorkspaceClose(made.Workspace.WorkspaceID)
	pane := made.RootPane.PaneID

	state := t.TempDir()
	fleet := machines.NewManager(state, sock, "herdr", nil)
	watcher := watch.New(fleet, events.NewBus())
	s := &Server{Machines: fleet, Watch: watcher, Input: sessions.NewInput(fleet, watcher), Feed: feed.New(state)}
	mux := http.NewServeMux()
	s.screenRoutes(mux)
	post := func(body string) int {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/input", strings.NewReader(body)))
		return rec.Code
	}
	screen := func() string {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/screen?machine=local&pane="+pane, nil))
		var read struct{ Text string }
		_ = json.Unmarshal(rec.Body.Bytes(), &read)
		return read.Text
	}
	for i := 0; i < 40 && !strings.Contains(screen(), "%"); i++ {
		time.Sleep(150 * time.Millisecond)
	}

	for _, refused := range []string{`{"machine":"local","text":"ls"}`, `{"machine":"local","pane":"` + pane + `"}`, `{"machine":"local","pane":"` + pane + `","keys":["f13"]}`} {
		if code := post(refused); code != 400 {
			t.Errorf("%s answered %d", refused, code)
		}
	}
	for _, body := range []string{`{"machine":"local","pane":"` + pane + `","text":"echo scr"}`, `{"machine":"local","pane":"` + pane + `","text":"een-ok","keys":["enter"]}`} {
		if code := post(body); code != 200 {
			t.Fatalf("%s answered %d", body, code)
		}
	}
	var text string
	for i := 0; i < 40; i++ {
		if text = screen(); strings.Contains(text, "\r\nscreen-ok") || strings.Contains(text, "\nscreen-ok") {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if !strings.Contains(text, "screen-ok\r\n") && !strings.Contains(text, "screen-ok\n") {
		t.Fatalf("the command's output is not on the screen:\n%s", text)
	}
	rows := s.Feed.List(0)
	if len(rows) != 1 || !strings.HasPrefix(rows[0].Title, "an API client sent «echo screen-ok» to ") {
		t.Fatalf("one row for the line, not one per request: %+v", rows)
	}
}
