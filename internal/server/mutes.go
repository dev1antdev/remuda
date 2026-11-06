package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Mutes: a workspace folder silenced for a while — set with the time to unmute at, lifted, listed. The roster
// carries each workspace's muted_until, so the sidebar and the Wall show the mark without a request of their own.

func (s *Server) muteRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/mutes", s.handleMutes)
	api.HandleFunc("PUT /api/mutes", s.handleMute)
	api.HandleFunc("DELETE /api/mutes", s.handleUnmute)
}

type muteReq struct {
	Machine string `json:"machine"`
	Dir     string `json:"dir"`
	Until   string `json:"until"` // RFC 3339, the moment the silence ends
}

// muteTarget reads the machine and folder a mute names, refusing an empty one and a machine the fleet does not know.
func (s *Server) muteTarget(w http.ResponseWriter, req muteReq) (string, string, bool) {
	machine := strings.TrimSpace(req.Machine)
	dir := strings.TrimSpace(req.Dir)
	if machine == "" || dir == "" {
		httpErr(w, 400, fmt.Errorf("a machine and a folder are required"))
		return "", "", false
	}
	if s.Machines != nil {
		if _, err := s.Machines.Get(machine); err != nil {
			httpErr(w, 404, fmt.Errorf("unknown machine %q", machine))
			return "", "", false
		}
	}
	return machine, dir, true
}

func (s *Server) handleMutes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"mutes": s.Push.Mutes.All()})
}

func (s *Server) handleMute(w http.ResponseWriter, r *http.Request) {
	var req muteReq
	if !decode(w, r, &req) {
		return
	}
	machine, dir, ok := s.muteTarget(w, req)
	if !ok {
		return
	}
	until, err := time.Parse(time.RFC3339, strings.TrimSpace(req.Until))
	if err != nil {
		httpErr(w, 400, fmt.Errorf("until must be an RFC 3339 time: %w", err))
		return
	}
	if err := s.Push.Mutes.Mute(machine, dir, until); err != nil {
		httpErr(w, 400, err)
		return
	}
	s.rosterMoved()
	writeJSON(w, map[string]any{"muted_until": until})
}

func (s *Server) handleUnmute(w http.ResponseWriter, r *http.Request) {
	var req muteReq
	if !decode(w, r, &req) {
		return
	}
	machine, dir, ok := s.muteTarget(w, req)
	if !ok {
		return
	}
	if err := s.Push.Mutes.Unmute(machine, dir); err != nil {
		httpErr(w, 500, err)
		return
	}
	s.rosterMoved()
	writeJSON(w, map[string]any{"ok": true})
}

// rosterMoved tells every open page the roster changed, when there is an event stream to tell.
func (s *Server) rosterMoved() {
	if s.Live != nil {
		s.Live.RosterChanged()
	}
}
