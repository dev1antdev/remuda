package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/ghpr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/prstatus"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/transcript"
	"github.com/dev1antdev/remuda/internal/worktrees"
)

// The pull request an agent named, followed to its end: a red CI's log or the reviewers' comments handed to the agent
// as a message.

func (s *Server) prRoutes(api *http.ServeMux) {
	api.HandleFunc("POST /api/pr/failure", s.prHandoff(prstatus.Failure))
	api.HandleFunc("POST /api/pr/review", s.prHandoff(prstatus.ReviewComments))
	api.HandleFunc("POST /api/pr/refresh", s.handlePRRefresh)
}

// handlePRRefresh is POST {machine, panes}: a hover card opened on a workspace's requests, each read again on the
// machine when its last read is prstatus.FreshFor old; the roster carries the read under way and what it came to.
func (s *Server) handlePRRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string   `json:"machine"`
		Panes   []string `json:"panes"`
	}
	if !decode(w, r, &req) {
		return
	}
	if len(req.Panes) == 0 || len(req.Panes) > 32 {
		httpErr(w, 400, fmt.Errorf("one to 32 panes required"))
		return
	}
	mc, err := s.machineFor(r, req.Machine)
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	reading := 0
	for _, pane := range req.Panes {
		if s.PRs.Fresh(mc.ID(), pane) {
			reading++
		}
	}
	writeJSON(w, map[string]any{"reading": reading})
}

// prHandoff is POST {machine, pane}: what gh (glab for a merge request) reads of the pane's request, composed by compose, sent to the pane's agent
// as the chat sends a message. 409 when there is nothing to send or the agent waits for an answer. The pull request
// panel names the request itself ({url}, a GitHub pull request the pane's agent need not have named) and, for one
// failed check's Fix with agent, that check ({check}).
func (s *Server) prHandoff(compose func(host shell.Shell, label, url string) (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Machine string `json:"machine"`
			Pane    string `json:"pane"`
			URL     string `json:"url"`
			Check   string `json:"check"`
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
		if req.URL != "" {
			_, n, github := ghpr.FromURL(req.URL)
			if !github {
				httpErr(w, 400, fmt.Errorf("%s is not a GitHub pull request", req.URL))
				return
			}
			link, ok = transcript.Link{URL: req.URL, Label: fmt.Sprintf("PR #%d", n)}, true
		}
		write := compose // the handler's own: one request's check never reaches the next
		if req.Check != "" {
			name := req.Check
			write = func(host shell.Shell, label, url string) (string, error) {
				return prstatus.FailureOfCheck(host, label, url, name)
			}
		}
		if !ok {
			httpErr(w, 404, fmt.Errorf("the agent has named no pull request"))
			return
		}
		if !prstatus.Readable(link.URL) {
			httpErr(w, 400, fmt.Errorf("%s is neither a GitHub pull request nor a GitLab merge request; gh and glab cannot read it", link.Label))
			return
		}
		text, err := write(mc, link.Label, link.URL)
		if errors.Is(err, prstatus.ErrNothingFailed) || errors.Is(err, prstatus.ErrNoReview) {
			httpErr(w, http.StatusConflict, err)
			return
		}
		if err != nil {
			httpErr(w, http.StatusBadGateway, fmt.Errorf("%s on %s: %w", prstatus.Tool(link.URL), mc.Label(), err))
			return
		}
		if err := s.Input.Submit(client, events.PaneRef{Machine: mc.ID(), Pane: req.Pane}, text); err != nil {
			if sessions.NotDelivered(err) {
				httpErr(w, http.StatusConflict, err)
				return
			}
			rpcErr(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "label": link.Label, "chars": len(text)})
	}
}

// landedIn is the pull or merge requests an agent of the workspace named that gh or glab read as merged.
func (s *Server) landedIn(mc *machines.Machine, workspaceID string) []worktrees.Landed {
	snap, _ := s.Watch.Snapshot(mc.ID())
	if snap == nil {
		return nil
	}
	var landed []worktrees.Landed
	for i := range snap.Panes {
		pane := &snap.Panes[i]
		if pane.WorkspaceID != workspaceID || pane.Agent == "" {
			continue
		}
		if _, ok := s.Said.Link(mc.ID(), pane.PaneID); !ok {
			continue
		}
		if status, ok := s.PRs.Status(mc.ID(), pane.PaneID); ok && status.State == "MERGED" {
			landed = append(landed, worktrees.Landed{Branch: status.HeadBranch, Commit: status.HeadCommit})
		}
	}
	return landed
}
