package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/ports"
	"github.com/dev1antdev/remuda/internal/preview"
	"github.com/dev1antdev/remuda/internal/resources"
)

// The status bar's right side: what the pane's machine's herdr panes weigh on it (internal/resources), the ports its
// processes listen on (internal/ports), and a port's process stopped at the person's word. A GET here is the person's
// and their devices': a viewer link reaches neither, an agent's key none.

func (s *Server) resourceRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/resources", s.handleResources)
	api.HandleFunc("GET /api/ports", s.handlePorts)
	api.HandleFunc("POST /api/ports/stop", s.handlePortStop)
}

// handleResources is GET /api/resources?machine=: the machine's load as its herdr panes make it, by workspace. The
// status bar asks it every few seconds, so the ports are read at the quicker pace meanwhile.
func (s *Server) handleResources(w http.ResponseWriter, r *http.Request) {
	if s.Resources == nil {
		httpErr(w, http.StatusNotFound, errors.New("the load is not read on this remuda"))
		return
	}
	mc, _, _, ok := s.ready(w, r, "")
	if !ok {
		return
	}
	s.Ports.Hurry()
	if mc.Windows() {
		httpErr(w, http.StatusConflict, errors.New(resources.NotOnWindows))
		return
	}
	usage, err := s.Resources.Read(mc.ID(), mc)
	if err != nil {
		httpErr(w, http.StatusBadGateway, fmt.Errorf("the load of %s: %w", mc.Label(), err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, usage)
}

// handlePorts is GET /api/ports?machine=: the machine's listening ports as last read, the ones outside every
// workspace too; read says whether they were read at all yet.
func (s *Server) handlePorts(w http.ResponseWriter, r *http.Request) {
	mc, err := s.machineFor(r, "")
	if err != nil {
		httpErr(w, http.StatusNotFound, err)
		return
	}
	s.Ports.Hurry()
	if mc.Windows() {
		writeJSON(w, map[string]any{"ports": []ports.Port{}, "read": false, "unsupported": preview.NotOnWindows})
		return
	}
	reading, read := s.Ports.Of(mc.ID())
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"at": reading.At, "ports": reading.Ports, "error": reading.Error, "read": read})
}

// handlePortStop is POST /api/ports/stop {machine, pid, port}: SIGTERM to that very process, once a fresh look finds
// it listening there among the ones the person may stop; the page asked first, naming the process and the port.
func (s *Server) handlePortStop(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string `json:"machine"`
		PID     int    `json:"pid"`
		Port    int    `json:"port"`
	}
	if !decode(w, r, &req) {
		return
	}
	mc, _, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	if mc.Windows() {
		httpErr(w, http.StatusConflict, errors.New(preview.NotOnWindows))
		return
	}
	stopped, err := ports.Stop(mc, s.PreviewSkip(), req.PID, req.Port)
	if errors.Is(err, ports.ErrNotListening) {
		httpErr(w, http.StatusNotFound, fmt.Errorf("nothing of yours listens on port %d of %s with pid %d now", req.Port, mc.Label(), req.PID))
		return
	}
	if err != nil {
		httpErr(w, http.StatusBadGateway, fmt.Errorf("stopping %s on port %d of %s: %w", stopped.Command, req.Port, mc.Label(), err))
		return
	}
	s.Ports.Again(mc.ID())
	writeJSON(w, map[string]any{"stopped": stopped})
}

// PortTargets is every connected machine whose ports the tracker reads — a Windows machine names no listener's folder
// — with its workspaces' folders, the floating terminal's and the concierge's left out.
func (s *Server) PortTargets() []ports.Target {
	var out []ports.Target
	for _, mc := range s.Machines.List() {
		if state, _ := mc.State(); state != machines.StateConnected || mc.Windows() {
			continue
		}
		out = append(out, ports.Target{ID: mc.ID(), Host: mc, Folders: s.previewFolders(mc.ID())})
	}
	return out
}

// previewFolders is the machine's workspaces' folders by id, as its last snapshot shows them, remuda's own left out
func (s *Server) previewFolders(machineID string) map[string]string {
	folders := map[string]string{}
	if snap, _ := s.Watch.Snapshot(machineID); snap != nil {
		for i := range snap.Workspaces {
			ws := &snap.Workspaces[i]
			if s.ownWorkspace(machineID, ws.WorkspaceID) {
				continue
			}
			if dir := snap.WorkspaceDir(ws); dir != "" {
				folders[ws.WorkspaceID] = dir
			}
		}
	}
	return folders
}
