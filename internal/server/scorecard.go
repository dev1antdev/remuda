package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/dev1antdev/remuda/internal/auth"
)

func (s *Server) scorecardRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/scorecard", s.handleScorecard)
}

// handleScorecard is GET /api/scorecard?days=30&tz=: per agent kind and model, what the period's task runs, reviews,
// pull requests and arenas came to. A viewer link reads it as it reads the usage report, without the tasks it would name.
func (s *Server) handleScorecard(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 {
		days = 30
	}
	days = min(days, 366)
	loc := time.Local
	if tz := r.URL.Query().Get("tz"); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	card, err := s.Scorecard.Card(days, loc)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, err)
		return
	}
	if auth.IsViewer(r.Context()) {
		card = card.WithoutTasks()
	}
	writeJSON(w, card)
}
