package server

import (
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/dev1antdev/remuda/internal/concierge"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
)

// The concierge: a Claude Code agent remuda keeps on the machine the person picks, to ask what the agents are doing and
// to file tasks; its tools are remuda's own MCP door alone. It starts when the person opens its place and is sent
// nothing but what they type there (through /api/submit, like any chat). The person's only: a viewer link and an
// agent's key never reach these routes.

func (s *Server) conciergeRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/concierge", s.handleConcierge)
	api.HandleFunc("PUT /api/concierge", s.handleConciergePick)
	api.HandleFunc("POST /api/concierge", s.handleConciergeStart)
	api.HandleFunc("POST /api/concierge/new", s.handleConciergeNew)
	api.HandleFunc("DELETE /api/concierge", s.handleConciergeStop)
}

// conciergeView is where the concierge runs or would, the machines it can run on, why it cannot now, and itself while it stands.
type conciergeView struct {
	Machine  string                `json:"machine"`
	Picked   string                `json:"picked,omitempty"`
	Machines []concierge.Candidate `json:"machines"`
	Trouble  string                `json:"trouble,omitempty"`
	Running  *concierge.Running    `json:"running,omitempty"`
}

// conciergeCandidates is every machine of the fleet as the choice sees it: connected or not, what its vitals say it has
func (s *Server) conciergeCandidates() []concierge.Candidate {
	var out []concierge.Candidate
	for _, mc := range s.Machines.List() {
		state, _ := mc.State()
		c := concierge.Candidate{ID: mc.ID(), Label: mc.Label(), Host: mc.Cfg.Host, Local: mc.Local(), Connected: state == machines.StateConnected}
		if r := s.Health.Report(mc.ID()); r != nil && r.Error == "" {
			c.Read, c.Claude, c.Remuda, c.Mac = true, r.Claude != "", r.Remuda != "", strings.HasPrefix(r.OS, "macOS")
		}
		out = append(out, c)
	}
	return out
}

func (s *Server) conciergeView() conciergeView {
	candidates := s.conciergeCandidates()
	picked := s.Concierge.Picked()
	view := conciergeView{Machine: concierge.Chosen(picked, candidates), Picked: picked, Machines: concierge.Hosts(candidates)}
	view.Trouble = concierge.Trouble(view.Machine, candidates)
	if view.Machines == nil {
		view.Machines = []concierge.Candidate{}
	}
	listed := false
	for _, c := range view.Machines {
		listed = listed || c.ID == view.Machine
	}
	for _, c := range candidates {
		if !listed && c.ID == view.Machine {
			view.Machines = append(view.Machines, c) // the pick, away or without Claude Code, stays in the picker
		}
	}
	if snap, _ := s.Watch.Snapshot(view.Machine); snap != nil {
		if run, ok := s.Concierge.Find(view.Machine, snap); ok {
			view.Running = &run
		}
	}
	return view
}

// handleConcierge is GET /api/concierge: where it runs, the picker's machines, and why not when it cannot.
func (s *Server) handleConcierge(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.conciergeView())
}

// handleConciergePick is PUT /api/concierge {machine}: the person's machine, "" for the default; the next start moves
// the concierge there and closes it where it stood.
func (s *Server) handleConciergePick(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string `json:"machine"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Machine != "" {
		if _, err := s.Machines.Get(req.Machine); err != nil || s.Machines.Server() && req.Machine == machines.LocalID {
			httpErr(w, 400, errors.New("the concierge runs on a machine of the fleet"))
			return
		}
	}
	if err := s.Concierge.Pick(req.Machine); err != nil {
		httpErr(w, 500, err)
		return
	}
	writeJSON(w, s.conciergeView())
}

// conciergeReady is the machine the concierge runs on, connected: a 409 with what to do when it cannot run there.
func (s *Server) conciergeReady(w http.ResponseWriter, r *http.Request) (*machines.Machine, *herdr.Client, bool) {
	view := s.conciergeView()
	if view.Trouble != "" {
		httpErr(w, http.StatusConflict, errors.New(view.Trouble))
		return nil, nil, false
	}
	mc, client, _, ok := s.ready(w, r, view.Machine)
	return mc, client, ok
}

// conciergeHost is the machine as a start needs it
func (s *Server) conciergeHost(mc *machines.Machine, client *herdr.Client) concierge.Host {
	return concierge.Host{
		ID: mc.ID(), Home: mc.Home(), Client: client, Remuda: concierge.RemudaOn(mc.Local(), mc.Home()),
		Snapshot: func() (*herdr.Snapshot, error) { return s.Watch.Refresh(mc) },
		Prepare:  func() (string, error) { return s.Concierge.Prepare(mc, mc.Local()) },
		Resumable: func(folder, session string) bool {
			_, err := mc.Transcripts.ResolveSession(concierge.Agent, folder, "id", session)
			return err == nil
		},
		Lock: func(pane string) sync.Locker { return s.Input.Lock(mc.ID(), pane) },
		Status: func(pane string) string {
			if snap, _ := s.Watch.Snapshot(mc.ID()); snap != nil {
				if p := snap.Pane(pane); p != nil {
					return p.AgentStatus
				}
			}
			return ""
		},
		Started: func(string, string, bool) { s.Watch.Refresh(mc) },
	}
}

// handleConciergeStart is POST /api/concierge: the concierge standing on its machine, started there when it is not.
func (s *Server) handleConciergeStart(w http.ResponseWriter, r *http.Request) {
	mc, client, ok := s.conciergeReady(w, r)
	if !ok {
		return
	}
	run, err := s.Concierge.Ensure(s.conciergeHost(mc, client))
	if err != nil {
		rpcErr(w, err)
		return
	}
	go s.closeOtherConcierges(mc.ID())
	writeJSON(w, run)
}

// handleConciergeNew is POST /api/concierge/new: a new conversation — the one standing closed and forgotten, a fresh
// agent started in its place.
func (s *Server) handleConciergeNew(w http.ResponseWriter, r *http.Request) {
	mc, client, ok := s.conciergeReady(w, r)
	if !ok {
		return
	}
	snap, _ := s.Watch.Snapshot(mc.ID())
	if err := s.Concierge.Close(mc.ID(), client, snap, true); err != nil {
		rpcErr(w, err)
		return
	}
	run, err := s.Concierge.Ensure(s.conciergeHost(mc, client))
	if err != nil {
		rpcErr(w, err)
		return
	}
	writeJSON(w, run)
}

// handleConciergeStop is DELETE /api/concierge: the concierge's workspace closed; its conversation comes back on the next start.
func (s *Server) handleConciergeStop(w http.ResponseWriter, r *http.Request) {
	view := s.conciergeView()
	mc, client, _, ok := s.ready(w, r, view.Machine)
	if !ok {
		return
	}
	snap, _ := s.Watch.Snapshot(mc.ID())
	if err := s.Concierge.Close(mc.ID(), client, snap, false); err != nil {
		if errors.Is(err, concierge.ErrNone) {
			httpErr(w, 404, err)
			return
		}
		rpcErr(w, err)
		return
	}
	s.Watch.Refresh(mc)
	writeJSON(w, map[string]any{"ok": true})
}

// closeOtherConcierges closes the concierge wherever else it still stands — the machine picked before, the default of
// an earlier day — on every one of those machines remuda reaches now; the others are closed on a later start.
func (s *Server) closeOtherConcierges(machineID string) {
	for _, id := range s.Concierge.Standing(machineID) {
		mc, err := s.Machines.Get(id)
		if err != nil {
			s.Concierge.Forget(id)
			continue
		}
		client, _, err := mc.Ready()
		if err != nil {
			continue
		}
		snap, _ := s.Watch.Snapshot(id)
		if s.Concierge.Close(id, client, snap, false) == nil {
			s.Watch.Refresh(mc)
		}
	}
}

// ownWorkspace says whether a workspace is one remuda keeps for itself — the floating terminal's shell, the concierge —
// which the editor's folders, the previews and the sidebar leave out.
func (s *Server) ownWorkspace(machineID, workspaceID string) bool {
	return (s.Scratch != nil && s.Scratch.Is(machineID, workspaceID)) || (s.Concierge != nil && s.Concierge.Is(machineID, workspaceID))
}
