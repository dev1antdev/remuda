package auth

import (
	"io"
	"net/http"
	"testing"
)

// prPanelRoutes are the pull request panel's routes, reads and writes: the person's alone.
var prPanelRoutes = []struct{ method, path string }{
	{"GET", "/api/pr/branch?machine=local&dir=/w/app"},
	{"GET", "/api/pr/search?repo=octo/trips"},
	{"GET", "/api/pr/detail?repo=octo/trips&number=482"},
	{"GET", "/api/pr/job?repo=octo/trips&job=1229"},
	{"GET", "/api/pr/log?repo=octo/trips&job=1229"},
	{"GET", "/api/pr/files?repo=octo/trips&number=482"},
	{"POST", "/api/pr/comment"},
	{"POST", "/api/pr/reply"},
	{"POST", "/api/pr/resolve"},
	{"POST", "/api/pr/ask-again"},
	{"POST", "/api/pr/state"},
	{"POST", "/api/pr/failure"},
	{"POST", "/api/pr/review"},
}

func TestThePullRequestPanelIsNeitherAViewersNorAKeys(t *testing.T) {
	g, h := viewerGuard(t)
	viewer, _, err := g.Keys.MintViewer("the office TV")
	if err != nil {
		t.Fatal(err)
	}
	for _, try := range prPanelRoutes {
		if code, body, _ := call(h, try.method, try.path, nil, viewer); code != 403 || body != "this link is for viewing only" {
			t.Fatalf("%s %s with a viewer link: %d %s", try.method, try.path, code, body)
		}
	}
	dir := t.TempDir()
	devices, _ := OpenDevices(dir)
	keys := openKeys(t, dir)
	withKeys := NewGuard(testToken, devices, keys).Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "reached") }))
	key, _, _ := keys.Mint("GOG")
	for _, try := range prPanelRoutes {
		if code, body := withKeyMethod(withKeys, try.method, try.path, key); code != 403 || body != "this key is for tasks and reading only" {
			t.Fatalf("%s %s with an agent's key: %d %s", try.method, try.path, code, body)
		}
		if code, body := withKeyMethod(withKeys, try.method, try.path, testToken); code != 200 || body != "reached" {
			t.Fatalf("%s %s with the token: %d %s", try.method, try.path, code, body)
		}
	}
}
