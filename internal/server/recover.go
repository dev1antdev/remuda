package server

import (
	"net/http"
)

// The chats a reboot stopped, started again on their sessions: one job per machine, pressed by the person.

func (s *Server) recoverRoutes(api *http.ServeMux) {
	api.HandleFunc("POST /api/panes/recover", s.handlePanesRecover)
}

// handlePanesRecover is POST /api/panes/recover {machine}: every chat the reboot stopped there resumed; the answer is the job.
func (s *Server) handlePanesRecover(w http.ResponseWriter, r *http.Request) {
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
	job, err := s.Updates.RecoverChats(mc)
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	writeJSON(w, map[string]any{"job": job.ID})
}
