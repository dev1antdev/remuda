package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr/herdrfake"
	"github.com/dev1antdev/remuda/internal/watch"
)

// fileServer is the file routes over this machine, with a watcher that has no snapshot yet: a viewer reaches nothing.
func fileServer(t *testing.T) http.Handler {
	t.Helper()
	dir := t.TempDir()
	fleet := herdrfake.Manager(dir)
	s := &Server{Machines: fleet, Watch: watch.New(fleet, events.NewBus())}
	mux := http.NewServeMux()
	s.fileRoutes(mux)
	return mux
}

func fileCall(h http.Handler, path string, viewer bool) (int, string) {
	req := httptest.NewRequest("GET", path, nil)
	if viewer {
		req = req.WithContext(auth.WithViewer(req.Context()))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, strings.TrimSpace(rec.Body.String())
}

func TestAViewerReadsFilesInTheWorkspacesOnly(t *testing.T) {
	h := fileServer(t)
	state := t.TempDir()
	token := filepath.Join(state, "token")
	if err := os.WriteFile(token, []byte("the-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, body := fileCall(h, "/api/fs/file?path="+token, false); code != 200 || body != "the-token" {
		t.Fatalf("the person reads the file: %d %q", code, body)
	}
	if code, body := fileCall(h, "/api/fs/file?path="+token, true); code != http.StatusForbidden || !strings.Contains(body, "viewing only") || strings.Contains(body, "the-token") {
		t.Fatalf("a viewer reading the token: %d %q", code, body)
	}
	if code, body := fileCall(h, "/api/fs/stat?dir="+state+"&path=token", false); code != 200 || !strings.Contains(body, `"kind":"text"`) {
		t.Fatalf("the person stats the file: %d %q", code, body)
	}
	for _, path := range []string{"/api/fs/stat?dir=" + state + "&path=token", "/api/fs/stat?path=" + token} {
		if code, body := fileCall(h, path, true); code != http.StatusForbidden || strings.Contains(body, token) {
			t.Fatalf("a viewer at %s: %d %q", path, code, body)
		}
	}
}

// TestAViewerIsRefusedBeforeAnyFileIsTouched: a pipe would hold a look at it until someone wrote to it.
func TestAViewerIsRefusedBeforeAnyFileIsTouched(t *testing.T) {
	h := fileServer(t)
	pipe := filepath.Join(t.TempDir(), "pipe")
	if err := exec.Command("mkfifo", pipe).Run(); err != nil {
		t.Skip(err)
	}
	for _, viewer := range []bool{true, false} {
		answered := make(chan int, 1)
		go func() {
			code, _ := fileCall(h, "/api/fs/stat?path="+url.QueryEscape(pipe), viewer)
			answered <- code
		}()
		select {
		case <-answered:
		case <-time.After(3 * time.Second):
			t.Fatalf("a look at a pipe hung (viewer %v)", viewer)
		}
	}
	if code, _ := fileCall(h, "/api/fs/file?path="+url.QueryEscape(pipe+" "), true); code != http.StatusForbidden {
		t.Errorf("a viewer with a space after the name: %d", code)
	}
}

func TestTheTreeListsAFolderAndAViewerOnlyTheWorkspaces(t *testing.T) {
	h := fileServer(t)
	state := t.TempDir()
	if err := os.WriteFile(filepath.Join(state, "token"), []byte("t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	listed := "/api/fs/tree?dir=" + url.QueryEscape(state)
	if code, body := fileCall(h, listed, false); code != 200 || !strings.Contains(body, `"name":"token"`) {
		t.Fatalf("the person lists the folder: %d %q", code, body)
	}
	if code, body := fileCall(h, listed, true); code != http.StatusForbidden || strings.Contains(body, `"name"`) {
		t.Fatalf("a viewer lists a folder outside the workspaces: %d %q", code, body)
	}
	if code, _ := fileCall(h, listed+"&path=..", false); code != 400 {
		t.Fatalf("a path climbing out answered %d", code)
	}
}
