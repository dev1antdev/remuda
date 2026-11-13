package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/worktrees"
)

// Removal is what the remove dialog shows of a workspace's linked worktree: the look, who else uses the checkout,
// whether an agent works there, and whether its branch reads as merged.
type Removal struct {
	worktrees.Look
	Workspace string   `json:"workspace"`
	Name      string   `json:"name"`
	RepoName  string   `json:"repo_name"`
	Made      bool     `json:"made"`             // under the machine's ~/.herdr/worktrees, where herdr and remuda make them
	Shared    []string `json:"shared,omitempty"` // the other workspaces standing on it or with a pane in it: then only this one closes
	Working   string   `json:"working,omitempty"`
	MergedPR  bool     `json:"merged_pr,omitempty"` // a pull request its agents named was read as merged
	Stamp     string   `json:"stamp"`
	Home      string   `json:"home"` // the machine's, for the paths the dialog says with ~
}

// removal looks at the workspace's worktree; a workspace on no linked worktree is a 400.
func (s *Server) removal(mc *machines.Machine, id string) (*Removal, int, error) {
	snap, err := s.Watch.Snapshot(mc.ID())
	if snap == nil {
		return nil, http.StatusBadGateway, err
	}
	ws := snap.Workspace(id)
	if ws == nil {
		return nil, http.StatusNotFound, fmt.Errorf("no workspace %s", id)
	}
	if ws.Worktree == nil || !ws.Worktree.IsLinkedWorktree || ws.Worktree.CheckoutPath == "" {
		return nil, http.StatusBadRequest, fmt.Errorf("%q stands on no linked worktree", ws.Label)
	}
	tree := ws.Worktree.CheckoutPath
	look, err := worktrees.LookAt(mc, tree)
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	rm := &Removal{Look: look, Workspace: id, Name: ws.Label, RepoName: ws.Worktree.RepoName, Made: worktrees.Made(mc.Home(), tree), Shared: sharers(snap, id, tree), Home: mc.Home()}
	rm.Working = s.busyIn(mc.ID(), tree)
	rm.MergedPR = s.mergedIn(mc, id)
	rm.Stamp = look.Stamp()
	return rm, 0, nil
}

// sharers names the other workspaces that stand on the worktree or have a pane inside it.
func sharers(snap *herdr.Snapshot, id, tree string) []string {
	style := rpath.For(tree)
	inside := func(dir string) bool { return dir != "" && style.Within(tree, dir) }
	seen := map[string]bool{}
	var names []string
	add := func(ws string) {
		if ws == id || seen[ws] {
			return
		}
		seen[ws] = true
		names = append(names, snap.WorkspaceLabel(ws))
	}
	for i := range snap.Workspaces {
		other := &snap.Workspaces[i]
		if other.Worktree != nil && inside(other.Worktree.CheckoutPath) {
			add(other.WorkspaceID)
		}
	}
	for i := range snap.Panes {
		pane := &snap.Panes[i]
		if inside(pane.CWD) || inside(pane.ForegroundCWD) {
			add(pane.WorkspaceID)
		}
	}
	return names
}

// handleWorktreeRemoval is GET /api/worktrees/{id}/removal?machine: the remove dialog's table.
func (s *Server) handleWorktreeRemoval(w http.ResponseWriter, r *http.Request) {
	mc, _, _, ok := s.ready(w, r, "")
	if !ok {
		return
	}
	rm, code, err := s.removal(mc, r.PathValue("id"))
	if err != nil {
		httpErr(w, code, err)
		return
	}
	writeJSON(w, rm)
}

// handleWorktreeRemoveAsShown is POST /api/worktrees/{id}/remove {machine, delete_branch, stamp, keep_folder}: the
// workspace closed and its worktree removed as the dialog showed it — --force past uncommitted work the person was told
// of, its branch with it when they flipped it to Deleted — looked at again first: an agent at work there refuses it,
// and so does risk the person was not shown (409, the dialog reads it again). Another workspace on the checkout, or
// keep_folder, closes this workspace alone.
func (s *Server) handleWorktreeRemoveAsShown(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine      string `json:"machine"`
		DeleteBranch bool   `json:"delete_branch"`
		Stamp        string `json:"stamp"`
		KeepFolder   bool   `json:"keep_folder"`
	}
	if !decode(w, r, &req) {
		return
	}
	mc, client, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	id := r.PathValue("id")
	rm, code, err := s.removal(mc, id)
	if err != nil {
		httpErr(w, code, err)
		return
	}
	if rm.Working != "" && len(rm.Shared) == 0 && !req.KeepFolder {
		httpErr(w, http.StatusConflict, fmt.Errorf("%s: let it finish before the worktree goes", rm.Working))
		return
	}
	if len(rm.Shared) > 0 || req.KeepFolder {
		if err := client.WorkspaceClose(id); err != nil {
			rpcErr(w, err)
			return
		}
		s.Watch.Refresh(mc)
		writeJSON(w, map[string]any{"closed": true, "kept": true})
		return
	}
	if req.Stamp != rm.Stamp {
		httpErr(w, http.StatusConflict, errors.New("it changed since this opened; look again, and remove it if that is still what you want"))
		return
	}
	if err := client.WorkspaceClose(id); err != nil {
		rpcErr(w, err)
		return
	}
	s.Watch.Refresh(mc)
	writeJSON(w, map[string]any{"closed": true, "worktree": worktrees.RemoveAsShown(mc, rm.Look, req.DeleteBranch)})
}
