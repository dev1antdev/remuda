package server

import (
	"net/http"

	"github.com/dev1antdev/remuda/internal/quick"
)

// Quick prompts: the saved list, replaced whole.

func (s *Server) quickRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/quick", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.Quick.List()) })
	api.HandleFunc("PUT /api/quick", s.handleQuickPut)
}

func (s *Server) handleQuickPut(w http.ResponseWriter, r *http.Request) {
	var list []quick.Prompt
	if !decode(w, r, &list) {
		return
	}
	out, err := s.Quick.Replace(list)
	if err != nil {
		httpErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, out)
}
