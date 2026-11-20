package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// A page's path the build does not hold is the page itself, so a reload of any address lands; an API path no route
// answers is a JSON 404, so a page from before a route went away is told so rather than handed the page's HTML.
func TestAnUnknownAPIPathIsANotFoundAndAnyOtherPathThePage(t *testing.T) {
	s := &Server{UI: fstest.MapFS{"index.html": {Data: []byte("<!doctype html>page")}}}
	h := s.staticHandler()
	for path, want := range map[string]int{"/api/standup": 404, "/api/standup?tz=UTC": 404, "/tasks": 200, "/": 200} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != want {
			t.Errorf("%s: %d, want %d", path, rec.Code, want)
		}
		if want == 404 && !strings.Contains(rec.Header().Get("Content-Type"), "json") {
			t.Errorf("%s answered %q, not JSON", path, rec.Header().Get("Content-Type"))
		}
	}
}
