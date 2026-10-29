package server

import (
	"fmt"
	"net/http"

	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/wake"
)

// Waking a sleeping machine: the magic packet from this remuda's own network, for a machine with a kept MAC address.

func (s *Server) wakeRoutes(api *http.ServeMux) {
	api.HandleFunc("POST /api/machines/{id}/wake", s.handleWakeMachine)
}

// handleWakeMachine is POST /api/machines/{id}/wake: the packet goes out on every broadcast address here, the answer
// says how many; a machine with no address is a 400, one that is connected a 409.
func (s *Server) handleWakeMachine(w http.ResponseWriter, r *http.Request) {
	mc, err := s.Machines.Get(r.PathValue("id"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	mac := s.Machines.MAC(mc.ID())
	if mac == "" {
		httpErr(w, 400, fmt.Errorf("no MAC address is known for %s: set one in the machine's menu first", mc.Label()))
		return
	}
	if state, _ := mc.State(); state == machines.StateConnected {
		httpErr(w, 409, fmt.Errorf("%s is connected; there is nothing to wake", mc.Label()))
		return
	}
	sent, err := wake.Send(mac)
	if err != nil {
		httpErr(w, 502, err)
		return
	}
	writeJSON(w, map[string]any{"sent": sent, "mac": mac})
}
