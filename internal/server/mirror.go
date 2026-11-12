package server

import (
	"errors"
	"net/http"

	"github.com/dev1antdev/remuda/internal/mirror"
)

// Inside herdr: whether remuda's figures go into herdr's sidebar and its alerts pop up as herdr's toasts, the snippet
// for herdr's config, and a test toast on every machine. The person's only: neither an agent key's nor a viewer's.

func (s *Server) mirrorRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/mirror", s.handleMirror)
	api.HandleFunc("PUT /api/mirror", s.handleMirrorPut)
	api.HandleFunc("POST /api/mirror/test", s.handleMirrorTest)
}

// handleMirror is GET /api/mirror: the two switches, the toasts' quiet hours, the snippet and the last toasts.
func (s *Server) handleMirror(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.Mirror.Status())
}

// handleMirrorPut is PUT /api/mirror {figures, toasts, quiet}: the switches whole; turning the figures off clears
// what remuda wrote on every machine.
func (s *Server) handleMirrorPut(w http.ResponseWriter, r *http.Request) {
	var settings mirror.Settings
	if !decode(w, r, &settings) {
		return
	}
	if err := s.Mirror.Put(settings); err != nil {
		httpErr(w, 500, err)
		return
	}
	writeJSON(w, s.Mirror.Status())
}

// handleMirrorTest is POST /api/mirror/test: a toast on every connected machine now, and how each went; 409 from a
// remuda that writes nothing into herdr.
func (s *Server) handleMirrorTest(w http.ResponseWriter, r *http.Request) {
	outcomes, err := s.Mirror.Test()
	if errors.Is(err, mirror.ErrClosed) {
		httpErr(w, http.StatusConflict, err)
		return
	}
	if outcomes == nil {
		outcomes = []mirror.Outcome{}
	}
	writeJSON(w, outcomes)
}
