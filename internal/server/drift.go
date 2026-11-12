package server

import "net/http"

// Fleet setup: how every machine of the fleet is set up, side by side, the odd one out marked.

func (s *Server) driftRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/fleet-setup", s.handleFleetSetup)
}

// handleFleetSetup is GET /api/fleet-setup[?fresh=1]: every machine read side by side, fresh=1 reading each at once
// and its vitals too; each read stands a minute otherwise.
func (s *Server) handleFleetSetup(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.Drift.Table(r.URL.Query().Get("fresh") == "1"))
}
