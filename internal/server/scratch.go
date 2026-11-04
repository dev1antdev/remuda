package server

import (
	"errors"
	"net/http"

	"github.com/dev1antdev/remuda/internal/scratch"
)

// The floating terminal's shell: one plain-shell workspace per machine, made in the home folder the first time the
// drawer opens there and attached like any pane; closing it ends the shell.

func (s *Server) scratchRoutes(api *http.ServeMux) {
	api.HandleFunc("POST /api/scratch", s.handleScratch)
	api.HandleFunc("DELETE /api/scratch", s.handleScratchClose)
}

type scratchReq struct {
	Machine string `json:"machine"`
}

// handleScratch is the machine's scratch shell, made when it has none: {workspace, pane, made}.
func (s *Server) handleScratch(w http.ResponseWriter, r *http.Request) {
	var req scratchReq
	if !decode(w, r, &req) {
		return
	}
	mc, client, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	snap, err := s.Watch.Refresh(mc) // the kept workspace is looked for in a fresh snapshot, not one from before it was made
	if snap == nil {
		httpErr(w, 502, err)
		return
	}
	shell, err := s.Scratch.Ensure(mc.ID(), client, snap, mc.Home())
	if err != nil {
		rpcErr(w, err)
		return
	}
	if shell.Made {
		s.Watch.Refresh(mc)
	}
	writeJSON(w, shell)
}

// handleScratchClose ends the machine's scratch shell.
func (s *Server) handleScratchClose(w http.ResponseWriter, r *http.Request) {
	var req scratchReq
	if !decode(w, r, &req) {
		return
	}
	mc, client, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	snap, _ := s.Watch.Snapshot(mc.ID())
	if err := s.Scratch.Close(mc.ID(), client, snap); err != nil {
		if errors.Is(err, scratch.ErrNone) {
			httpErr(w, 404, err)
			return
		}
		rpcErr(w, err)
		return
	}
	s.Watch.Refresh(mc)
	writeJSON(w, map[string]any{"ok": true})
}
