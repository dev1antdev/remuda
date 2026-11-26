package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/dev1antdev/remuda/internal/herdrollback"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/restart"
)

// The chats a reboot stopped, started again on their sessions: one job per machine, pressed by the person; and on a
// machine moved to remuda-core, herdr's server stopped at the person's press, so the agents it ran there carry on.

func (s *Server) recoverRoutes(api *http.ServeMux) {
	api.HandleFunc("POST /api/panes/recover", s.handlePanesRecover)
	api.HandleFunc("POST /api/machines/{id}/herdr/stop", s.handleHerdrStop)
}

// handlePanesRecover is POST /api/panes/recover {machine, panes}: the chats the reboot stopped there in the panes picked
// resumed, every one for none; the answer is the job. herdr's agents while its server still answers there are a 409
// with herdr, for the page to offer its stop.
func (s *Server) handlePanesRecover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string   `json:"machine"`
		Panes   []string `json:"panes"`
	}
	if !decode(w, r, &req) {
		return
	}
	mc, _, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	job, err := s.Updates.RecoverChats(mc, req.Panes)
	var running *restart.HerdrRunning
	switch {
	case errors.As(err, &running):
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]any{"error": running.Error(), "herdr": true})
		return
	case err != nil:
		httpErr(w, 400, err)
		return
	}
	writeJSON(w, map[string]any{"job": job.ID})
}

// handleHerdrStop is POST /api/machines/{id}/herdr/stop: herdr's server stopped on a machine moved to remuda-core, at
// the person's press, every agent still running in it there stopping with it; herdr stays installed. A scratch
// instance stops none: herdr there may be the real remuda's.
func (s *Server) handleHerdrStop(w http.ResponseWriter, r *http.Request) {
	if !s.HerdrStops {
		httpErr(w, 409, errors.New("a scratch instance (REMUDA_STATE_DIR) stops no herdr server: herdr there may be the real remuda's"))
		return
	}
	mc, err := s.Machines.Get(r.PathValue("id"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	if !mc.OnCore() {
		httpErr(w, 409, fmt.Errorf("%s is reached through herdr: its server runs the agents remuda sees there", mc.Label()))
		return
	}
	if _, _, err := mc.Ready(); errors.Is(err, machines.ErrServerHost) { // the page's own host under -server is none of the fleet's
		httpErr(w, 409, err)
		return
	} else if err != nil {
		httpErr(w, 502, err)
		return
	}
	bin, socket := mc.MovedHerdr()
	if err := herdrollback.StopServing(mc, bin, socket); err != nil {
		httpErr(w, 502, err)
		return
	}
	if s.Health != nil {
		s.Health.Refresh(mc) // the ⓘ and the checkout guard read it at once: writes lift with the server
	}
	log.Printf("herdr's server on %s stopped at the person's press", mc.Label())
	writeJSON(w, map[string]any{"stopped": true})
}
