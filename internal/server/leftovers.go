package server

import (
	"errors"
	"net/http"

	"github.com/dev1antdev/remuda/internal/worktrees"
)

// Leftover worktrees: the ones under a machine's ~/.herdr/worktrees no workspace has open — closed in herdr itself, or
// before closes took their worktrees down — listed, and the clean ones removed at the person's word.

func (s *Server) leftoverRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/machines/{id}/leftovers", s.handleLeftovers)
	api.HandleFunc("POST /api/machines/{id}/leftovers/remove", s.handleLeftoversRemove)
}

// openFolders is every folder a workspace or pane of the machine has open now.
func (s *Server) openFolders(machineID string) ([]string, error) {
	snap, err := s.Watch.Snapshot(machineID)
	if snap == nil {
		if err == nil {
			err = errors.New("no snapshot of the machine yet")
		}
		return nil, err
	}
	open := snap.WorkspaceDirs()
	for i := range snap.Workspaces {
		if tree := snap.Workspaces[i].Worktree; tree != nil && tree.CheckoutPath != "" {
			open = append(open, tree.CheckoutPath)
		}
	}
	for i := range snap.Panes {
		open = append(open, snap.Panes[i].CWD, snap.Panes[i].ForegroundCWD)
	}
	return open, nil
}

// handleLeftovers is GET /api/machines/{id}/leftovers: the worktrees no workspace has open, each with its branch, its
// first uncommitted change and its size.
func (s *Server) handleLeftovers(w http.ResponseWriter, r *http.Request) {
	mc, _, _, ok := s.ready(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	open, err := s.openFolders(mc.ID())
	if err != nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	list, err := worktrees.Leftovers(mc, open)
	if err != nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, map[string]any{"root": worktrees.Root(mc.Home()), "leftovers": list})
}

// handleLeftoversRemove is POST /api/machines/{id}/leftovers/remove {paths}: each path still a leftover is taken down
// as a close takes one down; a path that is no leftover now — opened again, or never one — is left alone and said so.
func (s *Server) handleLeftoversRemove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths []string `json:"paths"`
	}
	if !decode(w, r, &req) {
		return
	}
	mc, _, _, ok := s.ready(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	open, err := s.openFolders(mc.ID())
	if err != nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	list, err := worktrees.Leftovers(mc, open)
	if err != nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	current := map[string]worktrees.Leftover{}
	for _, left := range list {
		current[left.Path] = left
	}
	outcomes := []worktrees.Outcome{}
	for _, path := range req.Paths {
		left, ok := current[path]
		switch {
		case !ok:
			outcomes = append(outcomes, worktrees.Outcome{Path: path, Kept: "not a leftover worktree now"})
		case left.Broken != "":
			outcomes = append(outcomes, worktrees.Outcome{Path: path, Kept: left.Broken})
		default:
			outcomes = append(outcomes, worktrees.Tidy(mc, path, false))
		}
	}
	writeJSON(w, map[string]any{"outcomes": outcomes})
}
