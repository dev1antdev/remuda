package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/dev1antdev/remuda/internal/updates"
)

// Updates from the page: herdr, the agents and remuda itself, each a job the person first sees as a command, then watches run.

func (s *Server) updateRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/machines/{id}/updates", s.handleMachineUpdates)
	api.HandleFunc("POST /api/machines/{id}/update", s.handleMachineUpdate)
	api.HandleFunc("GET /api/machines/{id}/jobs", s.handleMachineJobs)
	api.HandleFunc("GET /api/jobs/{id}", s.handleJob)
	api.HandleFunc("POST /api/panes/restart", s.handlePaneRestart)
}

// handlePaneRestart is POST /api/panes/restart {machine, pane}: one chat started again on its session; the answer is the job.
func (s *Server) handlePaneRestart(w http.ResponseWriter, r *http.Request) {
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
	mc, _, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	job, err := s.Updates.RestartPane(mc, req.Pane)
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	writeJSON(w, map[string]any{"job": job.ID})
}

// handleMachineUpdates is GET /api/machines/{id}/updates: what this machine can update now.
func (s *Server) handleMachineUpdates(w http.ResponseWriter, r *http.Request) {
	mc, err := s.Machines.Get(r.PathValue("id"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	available := s.Updates.Available(mc)
	if available == nil {
		available = []updates.Update{}
	}
	writeJSON(w, map[string]any{"updates": available})
}

// handleMachineUpdate is POST /api/machines/{id}/update {what, command}: starts it; the answer is the job.
func (s *Server) handleMachineUpdate(w http.ResponseWriter, r *http.Request) {
	mc, err := s.Machines.Get(r.PathValue("id"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	var req struct {
		What    string `json:"what"`
		Command string `json:"command"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpErr(w, 400, err)
		return
	}
	job, err := s.Updates.Start(mc, req.What, req.Command)
	var stale *updates.StaleError
	switch {
	case errors.As(err, &stale):
		httpErr(w, http.StatusConflict, err)
		return
	case err != nil:
		httpErr(w, 400, err)
		return
	}
	writeJSON(w, map[string]any{"job": job.ID})
}

// handleMachineJobs is GET /api/machines/{id}/jobs: recent jobs there, newest first, and the restarts still booked.
func (s *Server) handleMachineJobs(w http.ResponseWriter, r *http.Request) {
	mc, err := s.Machines.Get(r.PathValue("id"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	list, pending := s.Updates.Recent(mc)
	writeJSON(w, map[string]any{"jobs": list, "pending": pending})
}

// handleJob is GET /api/jobs/{id}.
func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	job, ok := s.Updates.Job(r.PathValue("id"))
	if !ok {
		httpErr(w, 404, fmt.Errorf("no such job"))
		return
	}
	writeJSON(w, job)
}
