package auth

import (
	"io"
	"net/http"
	"testing"
)

// The Repositories place reads every checkout and fetches, pulls and pushes at the person's press: neither a viewer
// link nor an agent's key reaches it.
func TestTheRepositoriesAreThePersonsOnly(t *testing.T) {
	g, h := viewerGuard(t)
	viewer, _, _ := g.Keys.MintViewer("the office TV")
	routes := []struct{ method, path string }{{"GET", "/api/repos"}, {"PUT", "/api/repos/scan"}, {"POST", "/api/repos/fetch"}, {"POST", "/api/repos/pull"}, {"POST", "/api/repos/push"}}
	for _, try := range routes {
		if code, body, _ := call(h, try.method, try.path, nil, viewer); code != 403 || body != "this link is for viewing only" {
			t.Fatalf("%s %s with a viewer link: %d %s", try.method, try.path, code, body)
		}
	}

	dir := t.TempDir()
	devices, _ := OpenDevices(dir)
	keys := openKeys(t, dir)
	guarded := NewGuard(testToken, devices, keys).Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "reached") }))
	key, _, _ := keys.Mint("GOG")
	for _, try := range routes {
		if code, body := withKeyMethod(guarded, try.method, try.path, key); code != 403 || body != "this key is for tasks and reading only" {
			t.Fatalf("%s %s with an agent's key: %d %q", try.method, try.path, code, body)
		}
	}
}
