package server

import (
	"net/http"

	"github.com/dev1antdev/remuda/internal/schedule"
)

// Schedules: prompts, digests and fresh agents on a timetable (internal/schedule).

func (s *Server) scheduleRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/schedules", s.handleSchedules)
	api.HandleFunc("GET /api/schedules/next", s.handleScheduleNext)
	api.HandleFunc("PUT /api/schedules", s.handleSchedulePut)
	api.HandleFunc("DELETE /api/schedules/{id}", s.handleScheduleDelete)
	api.HandleFunc("POST /api/schedules/{id}/run", s.handleScheduleRun)
}

func (s *Server) handleSchedules(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.Schedules.List())
}

// handleScheduleNext answers a cron's next runs on this host's clock, for the form before it is saved; a bad cron is a 400 saying why.
func (s *Server) handleScheduleNext(w http.ResponseWriter, r *http.Request) {
	next, err := s.Schedules.Upcoming(r.URL.Query().Get("cron"), 3)
	if err != nil {
		httpErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"next": next})
}

func (s *Server) handleSchedulePut(w http.ResponseWriter, r *http.Request) {
	var req schedule.Schedule
	if !decode(w, r, &req) {
		return
	}
	saved, err := s.Schedules.Put(req)
	if err != nil {
		httpErr(w, http.StatusBadRequest, err)
		return
	}
	s.modelsMoved()
	writeJSON(w, saved)
}

func (s *Server) handleScheduleDelete(w http.ResponseWriter, r *http.Request) {
	s.Schedules.Delete(r.PathValue("id"))
	s.modelsMoved()
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleScheduleRun(w http.ResponseWriter, r *http.Request) {
	saved, err := s.Schedules.Run(r.PathValue("id"))
	if err != nil {
		httpErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, saved)
}
