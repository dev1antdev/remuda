package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/prstatus"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/worktrees"
)

// The pull request an agent named, followed to its end: a red CI's log or the reviewers' comments handed to the agent
// as a message, and a merged worktree taken down with its branch.

func (s *Server) prRoutes(api *http.ServeMux) {
	api.HandleFunc("POST /api/pr/failure", s.prHandoff(prstatus.Failure))
	api.HandleFunc("POST /api/pr/review", s.prHandoff(prstatus.ReviewComments))
	api.HandleFunc("DELETE /api/worktrees/{id}", s.handleWorktreeRemove)
}

// prHandoff is POST {machine, pane}: what gh reads of the pane's request, composed by compose, sent to the pane's agent
// as the chat sends a message. 409 when there is nothing to send or the agent waits for an answer.
func (s *Server) prHandoff(compose func(host shell.Shell, label, url string) (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Machine string `json:"machine"`
			Pane    string `json:"pane"`
		}
		if !decode(w, r, &req) {
			return
		}
		if req.Pane == "" {
			httpErr(w, 400, fmt.Errorf("pane required"))
			return
		}
		mc, client, _, ok := s.ready(w, r, req.Machine)
		if !ok {
			return
		}
		link, ok := s.Said.Link(mc.ID(), req.Pane)
		if !ok {
			httpErr(w, 404, fmt.Errorf("the agent has named no pull request"))
			return
		}
		if !prstatus.GitHub(link.URL) {
			httpErr(w, 400, fmt.Errorf("%s is not a GitHub pull request; gh cannot read it", link.Label))
			return
		}
		text, err := compose(mc, link.Label, link.URL)
		if errors.Is(err, prstatus.ErrNothingFailed) || errors.Is(err, prstatus.ErrNoReview) {
			httpErr(w, http.StatusConflict, err)
			return
		}
		if err != nil {
			httpErr(w, http.StatusBadGateway, fmt.Errorf("gh on %s: %w", mc.Label(), err))
			return
		}
		if err := s.Input.Submit(client, events.PaneRef{Machine: mc.ID(), Pane: req.Pane}, text); err != nil {
			if errors.Is(err, sessions.ErrAgentBusy) {
				httpErr(w, http.StatusConflict, err)
				return
			}
			rpcErr(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "label": link.Label, "chars": len(text)})
	}
}

// handleWorktreeRemove is DELETE /api/worktrees/{id}?machine=: a linked worktree's workspace closed, then the worktree
// removed from its repository and its branch deleted, on the machine as the person. A worktree with uncommitted changes
// is refused before anything closes; the branch goes past git's own merge check only when the request it carried is merged.
func (s *Server) handleWorktreeRemove(w http.ResponseWriter, r *http.Request) {
	mc, client, _, ok := s.ready(w, r, "")
	if !ok {
		return
	}
	snap, err := s.Watch.Snapshot(mc.ID())
	if snap == nil {
		httpErr(w, 502, err)
		return
	}
	id := r.PathValue("id")
	ws := snap.Workspace(id)
	if ws == nil {
		httpErr(w, 404, fmt.Errorf("no workspace %s", id))
		return
	}
	if ws.Worktree == nil || !ws.Worktree.IsLinkedWorktree || ws.Worktree.CheckoutPath == "" {
		httpErr(w, 400, fmt.Errorf("%q is not a linked worktree", ws.Label))
		return
	}
	info, err := worktrees.Inspect(mc, ws.Worktree.CheckoutPath)
	if err != nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	if info.Dirty != "" {
		httpErr(w, http.StatusConflict, fmt.Errorf("the worktree has uncommitted changes (%s); commit or discard them first", info.Dirty))
		return
	}
	merged := s.mergedIn(mc, id)
	if err := client.WorkspaceClose(id); err != nil {
		rpcErr(w, err)
		return
	}
	s.Watch.Refresh(mc)
	if err := worktrees.Remove(mc, info, merged); err != nil {
		httpErr(w, http.StatusBadGateway, fmt.Errorf("the workspace is closed; %s", err))
		return
	}
	writeJSON(w, map[string]any{"ok": true, "branch": info.Branch, "repo": info.Repo})
}

// mergedIn says whether an agent of the workspace named a pull request gh read as merged.
func (s *Server) mergedIn(mc *machines.Machine, workspaceID string) bool {
	snap, _ := s.Watch.Snapshot(mc.ID())
	if snap == nil {
		return false
	}
	for i := range snap.Panes {
		pane := &snap.Panes[i]
		if pane.WorkspaceID != workspaceID || pane.Agent == "" {
			continue
		}
		if _, ok := s.Said.Link(mc.ID(), pane.PaneID); !ok {
			continue
		}
		if status, ok := s.PRs.Status(mc.ID(), pane.PaneID); ok && status.State == "MERGED" {
			return true
		}
	}
	return false
}
