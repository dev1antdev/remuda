package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/dev1antdev/herdr-ui/internal/machines"
)

// Review: what changed in a workspace's folder since the viewer last looked.

func (s *Server) reviewTarget(w http.ResponseWriter, r *http.Request, dir string) (*machines.Machine, bool) {
	m, err := s.machineFor(r, "")
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

// GET /api/review?machine=&dir= → the summary
func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("dir")
	mc, ok := s.reviewTarget(w, r, dir)
	if !ok {
		return
	}
	writeJSON(w, s.Review.Summary(mc, dir))
}

// GET /api/review/diff?machine=&dir=&path= → one file's unified diff
func (s *Server) handleReviewDiff(w http.ResponseWriter, r *http.Request) {
	dir, path := r.URL.Query().Get("dir"), r.URL.Query().Get("path")
	mc, ok := s.reviewTarget(w, r, dir)
	if !ok {
		return
	}
	if path == "" || strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
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

// POST /api/review/seen {machine, dir} → the baseline moves to now
func (s *Server) handleReviewSeen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string `json:"machine"`
		Dir     string `json:"dir"`
	}
	if !decode(w, r, &req) {
		return
	}
	m, err := s.machineFor(r, req.Machine)
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	if !strings.HasPrefix(req.Dir, "/") {
		httpErr(w, 400, errors.New("an absolute folder is required"))
		return
	}
	b, err := s.Review.MarkSeen(m, req.Dir)
	if err != nil {
		httpErr(w, 502, err)
		return
	}
	writeJSON(w, b)
}
