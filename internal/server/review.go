package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/review"
)

// reviewTarget resolves the request's machine and insists on an absolute folder.
func (s *Server) reviewTarget(w http.ResponseWriter, r *http.Request, bodyMachine, dir string) (*machines.Machine, bool) {
	m, err := s.machineFor(r, bodyMachine)
	if err != nil {
		httpErr(w, 404, err)
		return nil, false
	}
	if !strings.HasPrefix(dir, "/") {
		httpErr(w, 400, errors.New("an absolute folder is required"))
		return nil, false
	}
	return m, true
}

// handleReview answers GET /api/review?machine=&dir= with the folder's Summary.
func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("dir")
	mc, ok := s.reviewTarget(w, r, "", dir)
	if !ok {
		return
	}
	writeJSON(w, s.Review.Summary(mc, dir))
}

// handleReviewDiff answers GET /api/review/diff?machine=&dir=&path= with one file's unified diff.
func (s *Server) handleReviewDiff(w http.ResponseWriter, r *http.Request) {
	dir, path := r.URL.Query().Get("dir"), r.URL.Query().Get("path")
	mc, ok := s.reviewTarget(w, r, "", dir)
	if !ok {
		return
	}
	// a path starting with "-" would reach git as an option
	if path == "" || strings.HasPrefix(path, "/") || strings.HasPrefix(path, "-") || strings.Contains(path, "..") {
		httpErr(w, 400, errors.New("a path relative to the folder is required"))
		return
	}
	diff, err := s.Review.Diff(mc, dir, path)
	if err != nil {
		httpErr(w, 502, err)
		return
	}
	writeJSON(w, map[string]string{"diff": diff})
}

// handleReviewSeen answers POST /api/review/seen {machine, dir, verdict?}: the
// baseline moves to now, with the verdict the page told the agent, if any.
func (s *Server) handleReviewSeen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string `json:"machine"`
		Dir     string `json:"dir"`
		Verdict string `json:"verdict"`
	}
	if !decode(w, r, &req) {
		return
	}
	if !review.VerdictOK(req.Verdict) {
		httpErr(w, 400, errors.New("the verdict is approved, changes, or none"))
		return
	}
	m, ok := s.reviewTarget(w, r, req.Machine, req.Dir)
	if !ok {
		return
	}
	b, err := s.Review.MarkSeen(m, req.Dir, req.Verdict)
	if err != nil {
		httpErr(w, 502, err)
		return
	}
	writeJSON(w, b)
}
