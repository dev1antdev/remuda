package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/watch"
)

func TestWithinFoldersFollowsSymlinksOnThisMachine(t *testing.T) {
	root := t.TempDir()
	workspace, outside := filepath.Join(root, "ws"), filepath.Join(root, "outside")
	for _, dir := range []string{workspace, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(workspace, "a.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(workspace, "link")); err != nil {
		t.Fatal(err)
	}
	folders := []string{workspace}
	for path, want := range map[string]bool{
		filepath.Join(workspace, "a.png"):                   true,
		workspace:                                           true,
		filepath.Join(workspace, "not-yet.png"):             true, // a file not there yet keeps its name under the folder
		filepath.Join(workspace, "link"):                    false,
		filepath.Join(outside, "secret"):                    false,
		filepath.Join(workspace, "..", "outside", "secret"): false,
		root: false,
	} {
		if got := withinFolders(path, folders, true); got != want {
			t.Errorf("withinFolders(%s) = %v, want %v", path, got, want)
		}
	}
	// a remote machine: the paths as given, since nothing there resolves them
	for path, want := range map[string]bool{"/w/api/x.png": true, "/w/api": true, "/w/api2/x.png": false, "/w": false, "/w/api/../../etc/passwd": false} {
		if got := withinFolders(path, []string{"/w/api/"}, false); got != want {
			t.Errorf("remote withinFolders(%s) = %v, want %v", path, got, want)
		}
	}
	if withinFolders("/w/api/x", nil, false) {
		t.Error("no folders should reach nothing")
	}
}

// TestAViewerReachesTheWorkspacesAndNoSecret: a workspace at the home or above it opens nothing, and the places keys
// live in stay closed inside any workspace, whatever the case they are asked in.
func TestAViewerReachesTheWorkspacesAndNoSecret(t *testing.T) {
	reach := reachOf([]string{"/", "/home", "/home/me", "/home/me/", "/home/me/app", "/home/me/.config", "/home/me/.ssh"}, "/home/me", "/srv/remuda", rpath.POSIX, false)
	for path, want := range map[string]bool{
		"/home/me/app/main.go":            true,
		"/home/me/app":                    true,
		"/home/me/.config/gh/hosts":       true,
		"/home/me/notes.txt":              false,
		"/etc/passwd":                     false,
		"/home/other/x":                   false,
		"/home/me/.ssh/id_ed25519":        false,
		"/home/me/.SSH/id_ed25519":        false,
		"/home/me/.config/remuda/token":   false,
		"/home/me/.config/REMUDA/token":   false,
		"/home/me/app/../.ssh/id_ed25519": false,
	} {
		if got := reach.allows(path); got != want {
			t.Errorf("%s: %v, want %v", path, got, want)
		}
	}
	mac := reachOf([]string{"/Users/me/Library", "/Users/me"}, "/Users/me", "", rpath.POSIX, false)
	if mac.allows("/Users/me/Library/Application Support/remuda/token") || !mac.allows("/Users/me/Library/Notes/a") {
		t.Error("a Mac's state dir")
	}
	windows := reachOf([]string{`C:\Users\me`, `C:\Users\me\AppData`, `C:\Users\me\app`}, `C:\Users\me`, "", rpath.Style{Windows: true}, false)
	for path, want := range map[string]bool{
		`C:\Users\me\app\main.go`:                  true,
		`C:\Users\me\AppData\Local\x`:              true,
		`c:\users\me\appdata\roaming\remuda\token`: false,
		`C:\Users\me\.ssh\id_ed25519`:              false,
		`C:\Users\me\notes.txt`:                    false,
	} {
		if got := windows.allows(path); got != want {
			t.Errorf("%s: %v, want %v", path, got, want)
		}
	}
	if reachOf([]string{"/w"}, "", "", rpath.POSIX, false).allows("/w/x") {
		t.Error("a machine whose home is not known yet reached a file")
	}
}

// TestAViewerReachFollowsLinksOnThisMachine: a link out of a workspace into a closed place reaches nothing.
func TestAViewerReachFollowsLinksOnThisMachine(t *testing.T) {
	home := t.TempDir()
	home, _ = filepath.EvalSymlinks(home)
	app, keys := filepath.Join(home, "app"), filepath.Join(home, ".ssh")
	for _, dir := range []string{app, keys} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(home, filepath.Join(home, "up")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(keys, filepath.Join(app, "keys")); err != nil {
		t.Fatal(err)
	}
	reach := reachOf([]string{app, filepath.Join(home, "up")}, home, "", rpath.POSIX, true)
	if len(reach.folders) != 1 || reach.allows(filepath.Join(app, "keys", "id")) || !reach.allows(filepath.Join(app, "main.go")) {
		t.Fatalf("folders %v", reach.folders)
	}
}

// fileServer is the file routes over this machine, with a watcher that has no snapshot yet: a viewer reaches nothing.
func fileServer(t *testing.T) http.Handler {
	t.Helper()
	dir := t.TempDir()
	fleet := machines.NewManager(dir, filepath.Join(dir, "no-herdr.sock"), "herdr", nil)
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
