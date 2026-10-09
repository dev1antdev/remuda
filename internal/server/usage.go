package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/herdr-ui/internal/machines"
	"github.com/dev1antdev/herdr-ui/internal/usage"
)

// GET /api/usage?days=7&machine=<id|all>&tz= — what the agents cost over
// a period, per machine: locally a scan of ~/.claude/projects, remotely
// the same scan by the herdr-ui deployed there.
func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 {
		days = 7
	}
	if days > 366 {
		days = 366
	}
	loc := time.Local
	tz := r.URL.Query().Get("tz")
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		} else {
			tz = "" // an unknown zone: every machine falls back the same way
		}
	}
	if tz == "" && loc.String() != "Local" {
		tz = loc.String() // "Local" means nothing to another machine; empty leaves it its own
	}
	// the period starts at a local midnight, so the first day is whole
	now := time.Now().In(loc)
	since := time.Date(now.Year(), now.Month(), now.Day()-(days-1), 0, 0, 0, 0, loc)
	want := r.URL.Query().Get("machine")

	type perMachine struct {
		ID     string        `json:"id"`
		Label  string        `json:"label"`
		Report *usage.Report `json:"report,omitempty"`
		Error  string        `json:"error,omitempty"`
	}
	var out []perMachine
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, mc := range s.Machines.List() {
		if want != "" && want != "all" && mc.Cfg.ID != want {
			continue
		}
		if st, _ := mc.State(); st != machines.StateConnected {
			continue
		}
		wg.Add(1)
		go func(mc *machines.Machine) {
			defer wg.Done()
			pm := perMachine{ID: mc.Cfg.ID, Label: mc.Label()}
			if mc.Local {
				rep, err := usage.Scan(mc.Home()+"/.claude/projects", since, loc)
				if err != nil {
					pm.Error = err.Error()
				} else {
					pm.Report = &rep
				}
			} else {
				raw, err := mc.Shell(usage.Script(mc.Home(), since, tz), 90*time.Second)
				var rep usage.Report
				switch {
				case err != nil:
					pm.Error = strings.TrimSpace(err.Error())
				case json.Unmarshal([]byte(raw), &rep) != nil:
					pm.Error = "the machine's herdr-ui gave no report"
				default:
					pm.Report = &rep
				}
			}
			mu.Lock()
			out = append(out, pm)
			mu.Unlock()
		}(mc)
	}
	wg.Wait()
	if out == nil {
		out = []perMachine{}
	}
	writeJSON(w, map[string]any{"since": since.UTC().Format(time.RFC3339), "days": days, "machines": out})
}
