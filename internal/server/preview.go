package server

import (
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/preview"
)

// Previews: the ports a machine's processes listen on, matched to its workspaces, and one opened on the cockpit at the
// person's word (internal/preview). Neither a viewer link nor an agent's key reaches these routes.

func (s *Server) previewRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/previews", s.handlePreviews)
	api.HandleFunc("POST /api/previews/open", s.handlePreviewOpen)
	api.HandleFunc("DELETE /api/previews", s.handlePreviewClose)
}

// previewSkip is the ports never offered: the cockpit's own, the door a remote machine's agents reach it by, a remuda's
// usual one (a peer), the code-server's
func (s *Server) previewSkip() map[int]bool {
	skip := map[int]bool{7317: true, machines.DoorPort: true}
	if s.Code != nil {
		skip[s.Code.Port] = true
	}
	if s.SelfPort != 0 {
		skip[s.SelfPort] = true
	}
	return skip
}

// listeners is the machine's ports worth a preview, each with the workspace whose folder holds its server
func (s *Server) listeners(mc *machines.Machine) ([]preview.Listener, error) {
	list, err := preview.Find(mc, s.previewSkip())
	if err != nil {
		return nil, err
	}
	folders := map[string]string{}
	if snap, _ := s.Watch.Snapshot(mc.ID()); snap != nil {
		for i := range snap.Workspaces {
			ws := &snap.Workspaces[i]
			if s.Scratch != nil && s.Scratch.Is(mc.ID(), ws.WorkspaceID) {
				continue
			}
			if dir := snap.WorkspaceDir(ws); dir != "" {
				folders[ws.WorkspaceID] = dir
			}
		}
	}
	return preview.Match(list, folders), nil
}

// handlePreviews is GET /api/previews?machine=: the machine's listeners and the previews open on it now.
func (s *Server) handlePreviews(w http.ResponseWriter, r *http.Request) {
	if s.Previews == nil {
		httpErr(w, http.StatusNotFound, errors.New("previews are off on this remuda"))
		return
	}
	mc, _, _, ok := s.ready(w, r, "")
	if !ok {
		return
	}
	list, err := s.listeners(mc)
	if err != nil {
		httpErr(w, http.StatusBadGateway, fmt.Errorf("the ports of %s: %w", mc.Label(), err))
		return
	}
	open := []preview.Preview{}
	for _, p := range s.Previews.List() {
		if p.Machine == mc.ID() {
			open = append(open, p)
		}
	}
	writeJSON(w, map[string]any{"listeners": list, "open": open})
}

type previewReq struct {
	Machine string `json:"machine"`
	Port    int    `json:"port"`
}

// handlePreviewOpen is POST /api/previews/open {machine, port}: a port the machine listens on now — never one the
// body names alone — opened on the cockpit, and its address.
func (s *Server) handlePreviewOpen(w http.ResponseWriter, r *http.Request) {
	var req previewReq
	if !decode(w, r, &req) {
		return
	}
	if s.Previews == nil {
		httpErr(w, http.StatusNotFound, errors.New("previews are off on this remuda"))
		return
	}
	mc, _, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	list, err := s.listeners(mc)
	if err != nil {
		httpErr(w, http.StatusBadGateway, fmt.Errorf("the ports of %s: %w", mc.Label(), err))
		return
	}
	var found *preview.Listener
	for i := range list {
		if list[i].Port == req.Port {
			found = &list[i]
		}
	}
	if found == nil {
		httpErr(w, http.StatusNotFound, fmt.Errorf("nothing of yours listens on port %d of %s now", req.Port, mc.Label()))
		return
	}
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	dial := func() (string, error) { return mc.ForwardHostAddr(found.Dial, found.Port) }
	p, err := s.Previews.Open(mc.ID(), found.Port, dial, host, scheme+"://"+r.Host)
	if errors.Is(err, preview.ErrNotReady) {
		httpErr(w, http.StatusServiceUnavailable, err)
		return
	}
	if err != nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, p)
}

// handlePreviewClose is DELETE /api/previews {machine, port}: the preview shut, its connections with it.
func (s *Server) handlePreviewClose(w http.ResponseWriter, r *http.Request) {
	var req previewReq
	if !decode(w, r, &req) {
		return
	}
	if s.Previews == nil {
		httpErr(w, http.StatusNotFound, errors.New("previews are off on this remuda"))
		return
	}
	mc, err := s.machineFor(r, req.Machine)
	if err != nil {
		httpErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, map[string]any{"closed": s.Previews.Close(mc.ID(), req.Port)})
}
