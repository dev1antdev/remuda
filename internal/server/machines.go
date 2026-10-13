package server

import (
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// Machines: the registry and each machine's vitals.

func (s *Server) machineRoutes(api *http.ServeMux) {
	api.HandleFunc("POST /api/machines", s.handleAddMachine)
	api.HandleFunc("DELETE /api/machines/{id}", s.handleRemoveMachine)
	api.HandleFunc("PATCH /api/machines/{id}", s.handleRenameMachine)
	api.HandleFunc("POST /api/machines/{id}/health", s.handleMachineHealth)
}

func (s *Server) handleAddMachine(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Label string `json:"label"`
		Host  string `json:"host"`
	}
	if !decode(w, r, &req) {
		return
	}
	mc, err := s.Machines.Add(req.Label, req.Host)
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	writeJSON(w, map[string]any{"id": mc.ID()})
}

func (s *Server) handleRemoveMachine(w http.ResponseWriter, r *http.Request) {
	if err := s.Machines.Remove(r.PathValue("id")); err != nil {
		httpErr(w, 400, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleRenameMachine is PATCH /api/machines/{id} {label}: a name of the user's own for the machine; empty goes back to the registry's.
func (s *Server) handleRenameMachine(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		httpErr(w, 400, err)
		return
	}
	mc, err := s.Machines.Rename(r.PathValue("id"), req.Label)
	if err != nil {
		if mc == nil {
			httpErr(w, 404, err)
		} else {
			httpErr(w, 500, err)
		}
		return
	}
	writeJSON(w, map[string]any{"ok": true, "label": mc.Label()})
}

// handleMachineHealth is POST /api/machines/{id}/health: read the machine's vitals now.
func (s *Server) handleMachineHealth(w http.ResponseWriter, r *http.Request) {
	mc, err := s.Machines.Get(r.PathValue("id"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	s.Health.RefreshLatest(5 * time.Minute)
	s.Health.Refresh(mc)
	writeJSON(w, map[string]any{"health": s.Health.Report(mc.ID()), "latest": s.Health.Latest()})
}
