package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/dev1antdev/remuda/internal/gitgraph"
	"github.com/dev1antdev/remuda/internal/gitops"
	"github.com/dev1antdev/remuda/internal/machines"
)

// The Git lens: a checkout's history as a graph, where it stands, one commit's files and one file's patch — a viewer's
// to read inside the workspaces' folders — and git's writes on it, the person's alone.

func (s *Server) gitRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/git/log", s.handleGitLog)
	api.HandleFunc("GET /api/git/state", s.handleGitState)
	api.HandleFunc("GET /api/git/commit", s.handleGitCommit)
	api.HandleFunc("GET /api/git/diff", s.handleGitDiff)
	api.HandleFunc("POST /api/git/op", s.handleGitOp)
}

var errViewerOutsideCheckout = errors.New("the folder is not one of the workspaces")

// gitTarget resolves the machine and the absolute folder; a viewer reads only a workspace's own folder.
func (s *Server) gitTarget(w http.ResponseWriter, r *http.Request, bodyMachine, dir string) (*machines.Machine, bool) {
	mc, ok := s.reviewTarget(w, r, bodyMachine, dir)
	if !ok {
		return nil, false
	}
	if folders, viewer := s.viewerFolders(r.Context(), mc); viewer && !withinFolders(dir, folders, mc.Local()) {
		httpErr(w, http.StatusForbidden, errViewerOutsideCheckout)
		return nil, false
	}
	return mc, true
}

// handleGitLog answers GET /api/git/log?machine=&dir=&all=1&remotes=1&skip=&limit= with a page of the history.
func (s *Server) handleGitLog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dir := q.Get("dir")
	mc, ok := s.gitTarget(w, r, "", dir)
	if !ok {
		return
	}
	skip, _ := strconv.Atoi(q.Get("skip"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	log, err := gitgraph.ReadLog(mc, dir, gitgraph.Query{All: q.Get("all") == "1", Remotes: q.Get("remotes") == "1", Skip: skip, Limit: limit})
	if err != nil {
		httpErr(w, 502, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, log)
}

// handleGitState answers GET /api/git/state?machine=&dir= with where the checkout stands: cheap, read every few seconds.
func (s *Server) handleGitState(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("dir")
	mc, ok := s.gitTarget(w, r, "", dir)
	if !ok {
		return
	}
	state, err := gitgraph.ReadState(mc, dir)
	if err != nil {
		httpErr(w, 502, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, state)
}

// handleGitCommit answers GET /api/git/commit?machine=&dir=&sha= with a commit's message, committer and files.
func (s *Server) handleGitCommit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dir := q.Get("dir")
	mc, ok := s.gitTarget(w, r, "", dir)
	if !ok {
		return
	}
	detail, err := gitgraph.ReadCommit(mc, dir, q.Get("sha"))
	if err != nil {
		gitReadErr(w, err)
		return
	}
	writeJSON(w, detail)
}

// handleGitDiff answers GET /api/git/diff?machine=&dir=&sha=&path=&from= with one file's patch in the commit.
func (s *Server) handleGitDiff(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dir := q.Get("dir")
	mc, ok := s.gitTarget(w, r, "", dir)
	if !ok {
		return
	}
	patch, err := gitgraph.ReadPatch(mc, dir, q.Get("sha"), q.Get("path"), q.Get("from"))
	if err != nil {
		gitReadErr(w, err)
		return
	}
	writeJSON(w, map[string]string{"diff": patch})
}

func gitReadErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, gitgraph.ErrInput):
		httpErr(w, 400, err)
	case errors.Is(err, gitgraph.ErrNoCommit):
		httpErr(w, 404, err)
	default:
		httpErr(w, 502, err)
	}
}

// handleGitOp answers POST /api/git/op {machine, dir, op, …, head, branch}: one of git's writes on the checkout, at the
// person's press. A viewer link and an agent's key never reach it (internal/auth); git's own refusal comes back as
// {ok: false, output}, with the operation it left in progress when it stopped on conflicts.
func (s *Server) handleGitOp(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string `json:"machine"`
		Dir     string `json:"dir"`
		gitops.Request
	}
	if !decode(w, r, &req) {
		return
	}
	mc, ok := s.reviewTarget(w, r, req.Machine, req.Dir)
	if !ok {
		return
	}
	ops := s.GitOps
	if ops == nil {
		ops = &gitops.Service{}
	}
	result, err := ops.Run(mc, req.Dir, req.Request)
	var invalid *gitops.InvalidError
	var busy *gitops.BusyError
	switch {
	case errors.As(err, &invalid):
		httpErr(w, 400, err)
		return
	case errors.As(err, &busy), errors.Is(err, gitops.ErrStale):
		httpErr(w, http.StatusConflict, err)
		return
	case errors.Is(err, gitops.ErrNotRepository):
		httpErr(w, 404, err)
		return
	case err != nil:
		httpErr(w, 502, err)
		return
	}
	go func() { // the sidebar's branch and the folder's uncommitted count follow what the checkout holds now
		if s.Review != nil {
			s.Review.Badges(mc, []string{req.Dir})
		}
		s.rosterMoved()
	}()
	writeJSON(w, result)
}
