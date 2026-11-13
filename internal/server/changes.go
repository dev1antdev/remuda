package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/dev1antdev/remuda/internal/gitstage"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/rpath"
)

// The Changes lens as a git client: a commit's files, the branch's work against its base and a conflicted file read;
// staged and unstaged, discard, commit and amend written at the person's press — none of them a viewer's or a key's.

func (s *Server) changesRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/review/commit", s.handleReviewCommit)
	api.HandleFunc("GET /api/review/base", s.handleReviewBase)
	api.HandleFunc("GET /api/review/conflict", s.handleReviewConflict)
	api.HandleFunc("GET /api/review/last-commit", s.handleLastCommit)
	api.HandleFunc("POST /api/review/stage", s.indexWrite(true))
	api.HandleFunc("POST /api/review/unstage", s.indexWrite(false))
	api.HandleFunc("POST /api/review/discard", s.handleDiscard)
	api.HandleFunc("POST /api/review/commit", s.handleCommit)
}

// relativePath is a path of the folder's own, never one git would take for an option or that climbs out.
func relativePath(path string) bool {
	return path != "" && !rpath.Absolute(path) && !strings.HasPrefix(path, "-") && !strings.Contains(path, "..")
}

// handleReviewCommit answers GET /api/review/commit?machine=&dir=&sha= with the commit's message and files.
func (s *Server) handleReviewCommit(w http.ResponseWriter, r *http.Request) {
	dir, sha := r.URL.Query().Get("dir"), r.URL.Query().Get("sha")
	mc, ok := s.reviewTarget(w, r, "", dir)
	if !ok {
		return
	}
	if !review.CommitOK(sha) {
		httpErr(w, 400, errors.New("a commit's hash is required"))
		return
	}
	view, err := review.ReadCommit(mc, dir, sha)
	if err != nil {
		httpErr(w, 502, err)
		return
	}
	writeJSON(w, view)
}

// handleReviewBase answers GET /api/review/base?machine=&dir=&base= with the branch's work against the base it was cut
// from, or against the one the person pinned.
func (s *Server) handleReviewBase(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("dir")
	mc, ok := s.reviewTarget(w, r, "", dir)
	if !ok {
		return
	}
	view, err := s.Review.Base(mc, dir, r.URL.Query().Get("base"))
	if err != nil {
		httpErr(w, 502, err)
		return
	}
	writeJSON(w, view)
}

// handleReviewConflict answers GET /api/review/conflict?machine=&dir=&path= with a conflicted file's stages and text.
func (s *Server) handleReviewConflict(w http.ResponseWriter, r *http.Request) {
	dir, path := r.URL.Query().Get("dir"), r.URL.Query().Get("path")
	mc, ok := s.reviewTarget(w, r, "", dir)
	if !ok {
		return
	}
	if !relativePath(path) {
		httpErr(w, 400, errors.New("a path relative to the folder is required"))
		return
	}
	read, err := review.Conflict(mc, dir, path)
	if err != nil {
		httpErr(w, 502, err)
		return
	}
	writeJSON(w, read)
}

// handleLastCommit answers GET /api/review/last-commit?machine=&dir= with HEAD and its message, which Amend loads.
func (s *Server) handleLastCommit(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("dir")
	mc, ok := s.reviewTarget(w, r, "", dir)
	if !ok {
		return
	}
	head, message, err := gitstage.LastCommit(mc, dir)
	if err != nil {
		stageErr(w, err)
		return
	}
	writeJSON(w, map[string]string{"head": head, "message": message})
}

// indexWrite answers POST /api/review/stage and /unstage {machine, dir, paths, bulk}: bulk passes over a conflict.
func (s *Server) indexWrite(stage bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Machine string   `json:"machine"`
			Dir     string   `json:"dir"`
			Paths   []string `json:"paths"`
			Bulk    bool     `json:"bulk"`
		}
		if !decode(w, r, &req) {
			return
		}
		mc, ok := s.writeTarget(w, r, req.Machine, req.Dir)
		if !ok {
			return
		}
		write := s.Stage.Unstage
		if stage {
			write = s.Stage.Stage
		}
		done, err := write(mc, req.Dir, req.Paths, req.Bulk)
		s.afterWrite(mc, req.Dir, err)
		if err != nil {
			stageErr(w, err)
			return
		}
		writeJSON(w, done)
	}
}

// handleDiscard answers POST /api/review/discard {machine, dir, path, kind}: unstaged, all, or untracked.
func (s *Server) handleDiscard(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string `json:"machine"`
		Dir     string `json:"dir"`
		Path    string `json:"path"`
		Kind    string `json:"kind"`
	}
	if !decode(w, r, &req) {
		return
	}
	mc, ok := s.writeTarget(w, r, req.Machine, req.Dir)
	if !ok {
		return
	}
	err := s.Stage.Discard(mc, req.Dir, req.Path, req.Kind)
	s.afterWrite(mc, req.Dir, err)
	if err != nil {
		stageErr(w, err)
		return
	}
	writeJSON(w, map[string]string{"path": req.Path})
}

// handleCommit answers POST /api/review/commit {machine, dir, message, stage?, amend?, head?} with the commit made.
func (s *Server) handleCommit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string `json:"machine"`
		Dir     string `json:"dir"`
		gitstage.CommitRequest
	}
	if !decode(w, r, &req) {
		return
	}
	mc, ok := s.writeTarget(w, r, req.Machine, req.Dir)
	if !ok {
		return
	}
	made, err := s.Stage.Commit(mc, req.Dir, req.CommitRequest)
	s.afterWrite(mc, req.Dir, err)
	if err != nil {
		stageErr(w, err)
		return
	}
	writeJSON(w, made)
}

// writeTarget is reviewTarget for a write, which needs the writes' service.
func (s *Server) writeTarget(w http.ResponseWriter, r *http.Request, bodyMachine, dir string) (*machines.Machine, bool) {
	if s.Stage == nil {
		httpErr(w, 503, errors.New("this remuda writes nothing into a checkout"))
		return nil, false
	}
	return s.reviewTarget(w, r, bodyMachine, dir)
}

// afterWrite reads the folder's badge again and moves the roster, once git may have changed the checkout.
func (s *Server) afterWrite(mc *machines.Machine, dir string, err error) {
	if gitstage.IsRefusal(err) {
		return
	}
	go func() {
		if s.Review != nil {
			s.Review.Badges(mc, []string{dir})
		}
		s.rosterMoved()
	}()
}

// stageErr is a refusal of remuda's own as a 409 with the files it names, git's own words as a 502.
func stageErr(w http.ResponseWriter, err error) {
	var refusal *gitstage.Refusal
	if errors.As(err, &refusal) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(refusal)
		return
	}
	httpErr(w, 502, err)
}
