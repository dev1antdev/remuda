package server

import (
	"errors"
	"net/http"
)

// What the composer offers at the caret: the slash commands the pane's agent knows in its folder, or the folder's
// files for an @, read on the machine and kept a minute (internal/complete).

func (s *Server) completeRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/complete", s.handleComplete)
}

// handleComplete is GET /api/complete?machine&dir&kind=commands|paths[&agent]: {commands} or {paths, cut}.
func (s *Server) handleComplete(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dir := q.Get("dir")
	mc, ok := s.reviewTarget(w, r, "", dir)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	switch q.Get("kind") {
	case "commands":
		list, err := s.Complete.Commands(mc, dir, q.Get("agent"))
		if err != nil {
			httpErr(w, 502, err)
			return
		}
		writeJSON(w, map[string]any{"commands": list})
	case "paths":
		files, err := s.Complete.Files(mc, dir)
		if err != nil {
			httpErr(w, 502, err)
			return
		}
		writeJSON(w, files)
	default:
		httpErr(w, 400, errors.New("kind must be commands or paths"))
	}
}
