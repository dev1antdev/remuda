package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/dev1antdev/remuda/internal/sleep"
)

// A workspace put to sleep and woken, and the two switches beside them: the person's only, never a viewer's or a key's.

func (s *Server) sleepRoutes(api *http.ServeMux) {
	api.HandleFunc("POST /api/sessions/{id}/sleep", s.handleSleep)
	api.HandleFunc("POST /api/sessions/{id}/wake", s.handleWake)
	api.HandleFunc("GET /api/sleep/settings", s.handleSleepSettings)
	api.HandleFunc("PUT /api/sleep/settings", s.handlePutSleepSettings)
}

// handleSleep is POST /api/sessions/{id}/sleep {machine, anyway}: the workspace's agents recorded and asked to leave
// in a job; agents at work or asking are a 409 naming them, until the person says anyway.
func (s *Server) handleSleep(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string `json:"machine"`
		Anyway  bool   `json:"anyway"`
	}
	if !decode(w, r, &req) {
		return
	}
	mc, _, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	started, err := s.Sleep.Sleep(mc, r.PathValue("id"), req.Anyway)
	var busy *sleep.BusyError
	switch {
	case errors.As(err, &busy):
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]any{"error": busy.Error(), "busy": busy.Busy})
		return
	case errors.Is(err, sleep.ErrNoWorkspace):
		httpErr(w, 404, err)
		return
	case err != nil:
		httpErr(w, 409, err)
		return
	}
	s.rosterMoved()
	writeJSON(w, started)
}

// handleWake is POST /api/sessions/{id}/wake {machine}: every agent a sleep stopped there started again on its
// session, in a job.
func (s *Server) handleWake(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string `json:"machine"`
	}
	if !decode(w, r, &req) {
		return
	}
	mc, _, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	started, err := s.Sleep.Wake(mc, r.PathValue("id"))
	switch {
	case errors.Is(err, sleep.ErrNotAsleep):
		httpErr(w, 400, err)
		return
	case err != nil:
		httpErr(w, 409, err)
		return
	}
	s.rosterMoved()
	writeJSON(w, started)
}

// handleSleepSettings is GET /api/sleep/settings: wake a sleeping workspace when it is opened, resume the chats a
// reboot stopped by themselves.
func (s *Server) handleSleepSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.Sleep.Store.Settings())
}

// handlePutSleepSettings is PUT /api/sleep/settings, both switches whole.
func (s *Server) handlePutSleepSettings(w http.ResponseWriter, r *http.Request) {
	var req sleep.Settings
	if !decode(w, r, &req) {
		return
	}
	if err := s.Sleep.Store.SetSettings(req); err != nil {
		httpErr(w, 500, err)
		return
	}
	writeJSON(w, s.Sleep.Store.Settings())
}
