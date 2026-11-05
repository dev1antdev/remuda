package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/opinion"
	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/sessions"
)

// The second opinion: another agent asked, in a new tab of the folder's workspace, to review what the Changes lens
// shows of it without changing anything; its answer comes back as comments on the diff's lines (internal/opinion).

func (s *Server) opinionRoutes(api *http.ServeMux) {
	api.HandleFunc("POST /api/opinion", s.handleOpinionAsk)
	api.HandleFunc("GET /api/opinion", s.handleOpinionGet)
	api.HandleFunc("POST /api/opinion/dismiss", s.handleOpinionDismiss)
	api.HandleFunc("DELETE /api/opinion", s.handleOpinionForget)
}

type opinionReq struct {
	Machine string `json:"machine"`
	Pane    string `json:"pane"` // the lens's pane, "" when the folder's workspace has none to name
	Dir     string `json:"dir"`
	Agent   string `json:"agent"`
	Model   string `json:"model"`
	Args    string `json:"args"`
	Close   *bool  `json:"close"` // the reviewer's tab closes once it has answered; true when unsaid
}

// handleOpinionAsk is POST /api/opinion: a tab "<Agent> review" in the folder's workspace, the reviewer started there
// and handed the prompt (opinion.Prompt) once it is up; the opinion it will answer is kept as running from now on,
// in place of the folder's last. Answers {opinion, tab, pane}, with agent_error when the agent would not start.
func (s *Server) handleOpinionAsk(w http.ResponseWriter, r *http.Request) {
	var req opinionReq
	if !decode(w, r, &req) {
		return
	}
	req.Dir = strings.TrimRight(req.Dir, "/")
	if req.Dir == "" || req.Agent == "" || req.Agent == "shell" {
		httpErr(w, 400, errors.New("a folder and an agent to review it are required"))
		return
	}
	if !sessions.KnownAgent(req.Agent) {
		httpErr(w, 400, fmt.Errorf("unknown agent %q", req.Agent))
		return
	}
	start, err := (&createReq{Agent: req.Agent, Model: req.Model, Args: req.Args}).start()
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	mc, client, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	snap, err := s.Watch.Snapshot(mc.ID())
	if snap == nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	workspace, err := reviewedWorkspace(snap, req.Pane, req.Dir)
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	if running, ok := s.Opinions.Running(mc.ID(), req.Dir); ok && snap.Pane(running.Pane) != nil {
		httpErr(w, http.StatusConflict, errors.New("a second opinion is on its way: wait for its answer, or close its tab"))
		return
	}
	summary := s.Review.Summary(mc, req.Dir)
	switch {
	case summary.Error != "":
		httpErr(w, http.StatusBadGateway, errors.New(summary.Error))
		return
	case summary.Repo == "":
		httpErr(w, 400, errors.New("the folder is not a git checkout"))
		return
	case len(summary.Files) == 0 && len(summary.Commits) == 0:
		httpErr(w, http.StatusConflict, errors.New("there is nothing to review: no uncommitted change and no commit since the last look"))
		return
	}
	head, blobs, err := review.Snapshot(mc, req.Dir)
	if err != nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	label := agentLabel(req.Agent) + " review"
	res, err := client.TabCreate(workspace, label)
	if err != nil {
		rpcErr(w, err)
		return
	}
	pane := res.RootPane.PaneID
	op := s.Opinions.Begin(opinion.Opinion{
		Machine: mc.ID(), Dir: req.Dir, Agent: req.Agent, Model: start.Model,
		Workspace: workspace, Tab: res.Tab.TabID, Pane: pane, Head: head, Blobs: blobs,
		Close: req.Close == nil || *req.Close,
	})
	resp := map[string]any{"tab": res.Tab.TabID, "pane": pane}
	if err := sessions.StartAgentWith(client, pane, req.Agent, start); err != nil {
		s.Opinions.Fail(mc.ID(), pane, "the reviewer did not start: "+err.Error())
		op, _ = s.Opinions.Get(mc.ID(), req.Dir)
		resp["agent_error"] = err.Error()
	} else {
		machine, prompt := mc.ID(), opinion.Prompt(scopeOf(summary), op.Ref)
		go func() {
			if err := s.firstPrompt(client, events.PaneRef{Machine: machine, Pane: pane}, prompt, label); err != nil {
				s.Opinions.Fail(machine, pane, "the request did not reach the reviewer: "+err.Error())
			}
		}()
	}
	resp["opinion"] = op
	s.Watch.Refresh(mc)
	writeJSON(w, resp)
}

// reviewedWorkspace is the workspace the reviewer's tab goes into: the pane's, when its folder is the one reviewed,
// else the first standing on that folder.
func reviewedWorkspace(snap *herdr.Snapshot, paneID, dir string) (string, error) {
	if paneID != "" {
		pane := snap.Pane(paneID)
		if pane == nil {
			return "", fmt.Errorf("pane %s not found", paneID)
		}
		ws := snap.Workspace(pane.WorkspaceID)
		if ws != nil && (strings.TrimRight(snap.WorkspaceDir(ws), "/") == dir || strings.TrimRight(pane.CWD, "/") == dir) {
			return ws.WorkspaceID, nil
		}
		return "", fmt.Errorf("%s is not the folder of the pane's workspace", dir)
	}
	for i := range snap.Workspaces {
		if strings.TrimRight(snap.WorkspaceDir(&snap.Workspaces[i]), "/") == dir {
			return snap.Workspaces[i].WorkspaceID, nil
		}
	}
	return "", fmt.Errorf("no workspace stands on %s", dir)
}

// scopeOf is what the lens shows of the folder, as the reviewer is told it.
func scopeOf(summary review.Summary) opinion.Scope {
	scope := opinion.Scope{Branch: summary.Branch, Commits: len(summary.Commits)}
	if summary.CommitsTotal > scope.Commits {
		scope.Commits = summary.CommitsTotal
	}
	if summary.Baseline != nil && scope.Commits > 0 {
		scope.Base = summary.Baseline.Head
	}
	for _, file := range summary.Files {
		if file.Untracked {
			scope.Files = append(scope.Files, file.Path+" (new)")
		} else {
			scope.Files = append(scope.Files, fmt.Sprintf("%s (+%d −%d)", file.Path, file.Added, file.Removed))
		}
	}
	return scope
}

// handleOpinionGet is GET /api/opinion?machine=&dir=: the folder's opinion, null when it has none; with open
// comments, the files of theirs that changed since the reviewer was asked (`moved`: their lines may have moved).
func (s *Server) handleOpinionGet(w http.ResponseWriter, r *http.Request) {
	mc, err := s.machineFor(r, "")
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	dir := strings.TrimRight(r.URL.Query().Get("dir"), "/")
	if dir == "" {
		httpErr(w, 400, errors.New("dir required"))
		return
	}
	op, ok := s.Opinions.Get(mc.ID(), dir)
	if !ok {
		writeJSON(w, map[string]any{"opinion": nil})
		return
	}
	if op.State == opinion.Done && op.Open() > 0 {
		if _, blobs, err := review.Snapshot(mc, dir); err == nil {
			op.Moved = opinion.Moved(op, blobs)
		}
	}
	writeJSON(w, map[string]any{"opinion": op})
}

// handleOpinionDismiss is POST /api/opinion/dismiss {machine, dir, ids}: those comments set aside — added to the
// person's review or not wanted — every one of them for no ids.
func (s *Server) handleOpinionDismiss(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string `json:"machine"`
		Dir     string `json:"dir"`
		IDs     []int  `json:"ids"`
	}
	if !decode(w, r, &req) {
		return
	}
	mc, err := s.machineFor(r, req.Machine)
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	op, err := s.Opinions.Dismiss(mc.ID(), strings.TrimRight(req.Dir, "/"), req.IDs)
	if errors.Is(err, opinion.ErrNone) {
		httpErr(w, 404, err)
		return
	}
	writeJSON(w, map[string]any{"opinion": op})
}

// handleOpinionForget is DELETE /api/opinion?machine=&dir=: the folder's opinion gone; its reviewer's tab, if still
// open, stays as it is.
func (s *Server) handleOpinionForget(w http.ResponseWriter, r *http.Request) {
	mc, err := s.machineFor(r, "")
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	s.Opinions.Forget(mc.ID(), strings.TrimRight(r.URL.Query().Get("dir"), "/"))
	writeJSON(w, map[string]any{"ok": true})
}
