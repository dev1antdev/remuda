package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/search"
	"github.com/dev1antdev/remuda/internal/shell"
)

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
	want := r.URL.Query().Get("machine")
	var hosts []shell.Host
	for _, mc := range s.Machines.List() {
		if want != "" && want != "all" && mc.Cfg.ID != want {
			continue
		}
		if st, _ := mc.State(); st != machines.StateConnected {
			continue
		}
		hosts = append(hosts, mc)
	}
	writeJSON(w, search.Search(hosts, q, r.URL.Query().Get("dir"), since))
}
