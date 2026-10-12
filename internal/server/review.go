package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/review"
)

// Review: what changed in a workspace's folder since the viewer last looked.

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

// GET /api/review?machine=&dir= → the summary
func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("dir")
	mc, ok := s.reviewTarget(w, r, "", dir)
	if !ok {
		return
	}
	writeJSON(w, s.Review.Summary(mc, dir))
}

// GET /api/review/diff?machine=&dir=&path= → one file's unified diff
func (s *Server) handleReviewDiff(w http.ResponseWriter, r *http.Request) {
	dir, path := r.URL.Query().Get("dir"), r.URL.Query().Get("path")
	mc, ok := s.reviewTarget(w, r, "", dir)
	if !ok {
		return
	}
	// relative, inside the folder, and not something git would read as an
	// option (a file named "--output=…" is a thing an agent can create)
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

// POST /api/review/seen {machine, dir, verdict?} → the baseline moves to
// now; the verdict ("approved", "changes", or none for a silent look) is
// what the page told the agent, kept for the next look's heading
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
