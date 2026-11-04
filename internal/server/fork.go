package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/text"
)

// A conversation forked: the pane's agent started on a copy of it in a new tab of the same workspace, the original
// going on as it was.

func (s *Server) forkRoutes(api *http.ServeMux) {
	api.HandleFunc("POST /api/fork", s.handleFork)
}

type forkReq struct {
	Machine string `json:"machine"`
	Pane    string `json:"pane"`
	Session string `json:"session"` // the conversation shown, "" for the pane's live one
}

// handleFork is POST /api/fork {machine, pane, session?}: a new tab of the pane's workspace named "<agent> fork", the
// pane's agent started there on a copy of the conversation; answers {id, pane} with agent_error as the tab route does.
func (s *Server) handleFork(w http.ResponseWriter, r *http.Request) {
	var req forkReq
	if !decode(w, r, &req) {
		return
	}
	if req.Pane == "" {
		httpErr(w, 400, errors.New("a pane to fork is required"))
		return
	}
	if req.Session != "" && !text.SessionID(req.Session) {
		httpErr(w, 400, errors.New("bad session id"))
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
	pane := snap.Pane(req.Pane)
	if pane == nil {
		httpErr(w, 404, fmt.Errorf("pane %s not found", req.Pane))
		return
	}
	session, err := forkSession(pane, req.Session)
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	label := agentLabel(pane.Agent) + " fork"
	res, err := client.TabCreate(pane.WorkspaceID, label)
	if err != nil {
		rpcErr(w, err)
		return
	}
	newPane := res.RootPane.PaneID
	resp := map[string]any{"id": res.Tab.TabID, "pane": newPane}
	if err := sessions.StartAgentWith(client, newPane, pane.Agent, sessions.Start{Resume: session, Fork: true}); err != nil {
		resp["agent_error"] = err.Error()
	}
	s.Watch.Refresh(mc)
	writeJSON(w, resp)
}

// forkSession is the conversation a fork copies — the one asked for, else the pane's live one — or why there is none.
func forkSession(pane *herdr.PaneInfo, asked string) (string, error) {
	if pane.Agent == "" {
		return "", errors.New("a shell pane has no conversation to fork")
	}
	if !sessions.CanFork(pane.Agent) {
		return "", fmt.Errorf("%s cannot start on a copy of a conversation", agentLabel(pane.Agent))
	}
	if asked != "" {
		return asked, nil
	}
	if live := pane.AgentSession; live != nil && live.Kind == "id" && text.SessionID(live.Value) {
		return live.Value, nil
	}
	return "", fmt.Errorf("%s has no session id yet: say something to it first", agentLabel(pane.Agent))
}
