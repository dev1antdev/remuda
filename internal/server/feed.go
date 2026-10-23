package server

import (
	"net/http"
	"strconv"
)

// The activity panel: what happened across the agents, newest first.

func (s *Server) feedRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/feed", s.handleFeed)
}

// handleFeed is GET /api/feed?limit=N (default 100): the newest entries.
func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 300 {
			limit = n
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"entries": s.Feed.List(limit)})
}
