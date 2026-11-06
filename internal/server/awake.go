package server

import (
	"errors"
	"net/http"
)

func (s *Server) awakeRoutes(api *http.ServeMux) {
	api.HandleFunc("PATCH /api/machines/{id}/awake", s.handleAwake)
}

// handleAwake flips a machine's "keep awake while agents work" switch and answers its hold as it stands.
func (s *Server) handleAwake(w http.ResponseWriter, r *http.Request) {
	var req struct {
		On bool `json:"on"`
	}
	if !decode(w, r, &req) {
		return
	}
	mc, err := s.Machines.Get(r.PathValue("id"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	if s.Awake == nil {
		httpErr(w, 503, errors.New("this remuda keeps no machine awake"))
		return
	}
	if err := s.Awake.Set(mc.ID(), req.On); err != nil {
		httpErr(w, 500, err)
		return
	}
	writeJSON(w, s.Awake.Status(mc.ID()))
}
