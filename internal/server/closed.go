package server

import (
	"errors"
	"net/http"

	"github.com/dev1antdev/remuda/internal/restart"
)

// The agents whose panes went away, listed to be brought back on their own sessions.

func (s *Server) closedRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/closed", s.handleClosed)
	api.HandleFunc("POST /api/closed/reopen", s.handleReopen)
}

// handleClosed is GET /api/closed: every machine's closed agents, newest first, less the ones whose conversation a
// pane runs again (reopened by hand, or brought back by remuda's own restart of herdr).
func (s *Server) handleClosed(w http.ResponseWriter, r *http.Request) {
	live := map[string]map[string]bool{}
	running := func(machineID, session string) bool {
		sessions, ok := live[machineID]
		if !ok {
			sessions = map[string]bool{}
			if snap, err := s.Watch.Snapshot(machineID); err == nil && snap != nil {
				for i := range snap.Panes {
					if p := &snap.Panes[i]; p.AgentSession != nil {
						sessions[p.AgentSession.Value] = true
					}
				}
			}
			live[machineID] = sessions
		}
		return sessions[session]
	}
	list := []restart.Closed{}
	for _, c := range s.Updates.Restart.Closed() {
		if !running(c.Machine, c.Session) {
			list = append(list, c)
		}
	}
	writeJSON(w, map[string]any{"closed": list})
}

// handleReopen is POST /api/closed/reopen {machine, id}: the agent started again on its session, in a tab of the
// workspace on its folder or a new workspace; the answer names the pane to select.
func (s *Server) handleReopen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string `json:"machine"`
		ID      string `json:"id"`
	}
	if !decode(w, r, &req) {
		return
	}
	mc, _, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	place, err := s.Updates.Restart.Reopen(mc, req.ID)
	s.Watch.Refresh(mc)
	var open *restart.OpenAlready
	switch {
	case errors.As(err, &open):
		writeJSON(w, map[string]any{"pane": open.Pane, "workspace": place.Workspace, "open_already": true})
	case errors.Is(err, restart.ErrNoSuchClosed):
		httpErr(w, http.StatusNotFound, err)
	case err != nil && place.Pane != "":
		rpcErr(w, err) // the tab is there, the agent did not start: herdr's refusal
	case err != nil:
		httpErr(w, http.StatusConflict, err)
	default:
		writeJSON(w, map[string]any{"pane": place.Pane, "workspace": place.Workspace})
	}
}
