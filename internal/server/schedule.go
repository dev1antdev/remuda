package server

import (
	"net/http"

	"github.com/dev1antdev/herdr-ui/internal/schedule"
)

// Schedules: prompts sent to an agent on a timetable (internal/schedule).

func (s *Server) handleSchedules(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.Schedules.List())
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
	writeJSON(w, saved)
}

func (s *Server) handleScheduleDelete(w http.ResponseWriter, r *http.Request) {
	s.Schedules.Delete(r.PathValue("id"))
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
