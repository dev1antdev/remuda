package server

import (
	"errors"
	"net/http"

	"github.com/dev1antdev/remuda/internal/gitsync"
	"github.com/dev1antdev/remuda/internal/rpath"
)

// The Repositories place: every repository the fleet knows at a glance, and fetch, pull and push on one at the
// person's press. None of it is a viewer link's or an agent's key's.

func (s *Server) syncRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/repos", s.handleRepos)
	api.HandleFunc("PUT /api/repos/scan", s.handleRepoScan)
	api.HandleFunc("POST /api/repos/fetch", s.repoAction(gitsync.ActFetch))
	api.HandleFunc("POST /api/repos/pull", s.repoAction(gitsync.ActPull))
	api.HandleFunc("POST /api/repos/push", s.repoAction(gitsync.ActPush))
}

// handleRepos answers GET /api/repos[?fresh=1]: every machine of the fleet with its repositories, the connected ones
// read side by side.
func (s *Server) handleRepos(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"machines": s.Sync.List(r.URL.Query().Get("fresh") == "1")})
}

// handleRepoScan answers PUT /api/repos/scan {machine, folder, depth}: the machine's folder to scan for repositories,
// an empty one stopping the scan.
func (s *Server) handleRepoScan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string `json:"machine"`
		Folder  string `json:"folder"`
		Depth   int    `json:"depth"`
	}
	if !decode(w, r, &req) {
		return
	}
	mc, err := s.machineFor(r, req.Machine)
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	scan, err := s.Sync.SetScan(mc, gitsync.Scan{Folder: req.Folder, Depth: req.Depth})
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	writeJSON(w, map[string]any{"scan": scan})
}

// repoAction answers POST /api/repos/{fetch|pull|push} {machine, dir, anyway}: what the action came to, a refusal or
// git's failure included, with the checkout read again; anyway lets a push past the secret check.
func (s *Server) repoAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Machine string `json:"machine"`
			Dir     string `json:"dir"`
			Anyway  bool   `json:"anyway"`
		}
		if !decode(w, r, &req) {
			return
		}
		mc, err := s.machineFor(r, req.Machine)
		if err != nil {
			httpErr(w, 404, err)
			return
		}
		if !rpath.Absolute(req.Dir) {
			httpErr(w, 400, errors.New("an absolute folder is required"))
			return
		}
		var outcome gitsync.Outcome
		switch action {
		case gitsync.ActFetch:
			outcome = s.Sync.Fetch(mc.ID(), req.Dir)
		case gitsync.ActPull:
			outcome = s.Sync.Pull(mc.ID(), req.Dir)
		default:
			outcome = s.Sync.Push(mc.ID(), req.Dir, req.Anyway)
		}
		writeJSON(w, outcome)
	}
}
