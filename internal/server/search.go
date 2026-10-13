package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/search"
)

func (s *Server) searchRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/search", s.handleSearch)
}

// handleSearch is GET /api/search?q=&machine=<id|all>&dir=&days=: full text over every connected machine's transcripts, newest first.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < 2 {
		httpErr(w, http.StatusBadRequest, errors.New("two characters or more"))
		return
	}
	var since time.Time
	if d, _ := strconv.Atoi(r.URL.Query().Get("days")); d > 0 {
		since = time.Now().AddDate(0, 0, -d)
	}
	hosts := s.connectedHosts(r.URL.Query().Get("machine"))
	writeJSON(w, search.Search(hosts, q, r.URL.Query().Get("dir"), since))
}
