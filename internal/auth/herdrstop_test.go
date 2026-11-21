package auth

import (
	"io"
	"net/http"
	"testing"
)

// Stopping herdr's server on a machine moved to remuda-core ends every agent still running in it there, and resuming
// herdr's agents starts agents: the person's alone, never a viewer link's or an agent's key's.
func TestStoppingHerdrIsThePersonsOnly(t *testing.T) {
	g, h := viewerGuard(t)
	viewer, _, _ := g.Keys.MintViewer("the office TV")
	routes := []struct{ method, path string }{{"POST", "/api/machines/gog/herdr/stop"}, {"POST", "/api/panes/recover"}}
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
