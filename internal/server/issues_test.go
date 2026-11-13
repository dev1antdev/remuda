package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/auth"
)

func TestTheIssueRoutesRefuseAViewerAndAKeyBeforeAnythingRuns(t *testing.T) {
	s := &Server{} // no fleet, no gh: a refusal must come before either is asked
	routes := []struct {
		method, path string
		handler      http.HandlerFunc
	}{
		{"GET", "/api/issues/repos?machine=local", s.handleIssueRepos},
		{"GET", "/api/issues?machine=local&repo=acme/widgets", s.handleIssues},
		{"GET", "/api/issues/41?machine=local&repo=acme/widgets", s.handleIssue},
		{"POST", "/api/issues/41/comment", s.handleIssueWrite("comment")},
		{"POST", "/api/issues/41/close", s.handleIssueWrite("close")},
		{"POST", "/api/issues/41/reopen", s.handleIssueWrite("reopen")},
		{"POST", "/api/issues/41/assign", s.handleIssueWrite("assign")},
	}
	for _, route := range routes {
		for who, wrap := range map[string]func(*http.Request) *http.Request{
			"viewer": func(r *http.Request) *http.Request { return r.WithContext(auth.WithViewer(r.Context())) },
			"key":    func(r *http.Request) *http.Request { return r.WithContext(auth.WithCallerName(r.Context(), "GOG")) },
		} {
			req := wrap(httptest.NewRequest(route.method, route.path, strings.NewReader(`{"machine":"local","repo":"acme/widgets","body":"hi"}`)))
			req.SetPathValue("number", "41")
			rec := httptest.NewRecorder()
			route.handler(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Errorf("%s %s by a %s: %d %s", route.method, route.path, who, rec.Code, rec.Body.String())
			}
		}
	}
}
