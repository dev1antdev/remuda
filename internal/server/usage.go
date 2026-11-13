package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/usage"
)

func (s *Server) usageRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/usage", s.handleUsage)
	api.HandleFunc("GET /api/usage/days", s.handleUsageDays)
}

// zoneOf is the viewer's zone the days are cut in, and its name for another machine ("" leaves it its own).
func zoneOf(r *http.Request) (*time.Location, string) {
	loc := time.Local
	tz := r.URL.Query().Get("tz")
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		} else {
			tz = ""
		}
	}
	if tz == "" && loc.String() != "Local" {
		tz = loc.String() // "Local" means nothing to another machine; empty leaves it its own
	}
	return loc, tz
}

// handleUsageDays is GET /api/usage/days?tz=: the last year's days on every machine for the heatmap, from what is kept,
// the days it lacks read in the background; a machine away is answered from what was kept.
func (s *Server) handleUsageDays(w http.ResponseWriter, r *http.Request) {
	loc, tz := zoneOf(r)
	var hosts []usage.YearHost
	for _, mc := range s.Machines.List() {
		state, _ := mc.State()
		hosts = append(hosts, usage.YearHost{Host: mc, Away: state != machines.StateConnected})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, s.UsageDays.Look(hosts, loc, tz))
}

// handleUsage is GET /api/usage?days=7&machine=<id|all>&tz=: what the agents cost over a period, per machine.
func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 {
		days = 7
	}
	if days > 366 {
		days = 366
	}
	loc, tz := zoneOf(r)
	now := time.Now().In(loc)
	since := time.Date(now.Year(), now.Month(), now.Day()-(days-1), 0, 0, 0, 0, loc)
	reports := usage.Collect(s.connectedHosts(r.URL.Query().Get("machine")), since, loc, tz)
	writeJSON(w, map[string]any{"since": since.UTC().Format(time.RFC3339), "days": days, "machines": reports})
}

// connectedHosts is the connected machines a fleet-wide question asks: one by id, or every one for "" and "all".
func (s *Server) connectedHosts(want string) []shell.Host {
	var hosts []shell.Host
	for _, mc := range s.Machines.List() {
		if want != "" && want != "all" && mc.ID() != want {
			continue
		}
		if state, _ := mc.State(); state != machines.StateConnected {
			continue
		}
		hosts = append(hosts, mc)
	}
	return hosts
}
