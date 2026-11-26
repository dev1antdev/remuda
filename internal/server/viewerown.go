package server

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/coreapi"
	"github.com/dev1antdev/remuda/internal/feed"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/scratch"
	"github.com/dev1antdev/remuda/internal/timelapse"
	"github.com/dev1antdev/remuda/internal/usage"
)

// The workspace remuda keeps for itself — the floating terminal's shell, where a token may stand on the screen — is the
// person's alone: to a viewer it does not exist.

// viewerOwnGuard answers a viewer who names a pane or a folder of those workspaces as it answers an unknown one.
func (s *Server) viewerOwnGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth.IsViewer(r.Context()) && s.viewerBarred(r) {
			httpErr(w, http.StatusNotFound, errors.New("not found"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func conversationRoute(path string) bool { return path == "/api/chat" || path == "/api/recap" }

func (s *Server) viewerBarred(r *http.Request) bool {
	q := r.URL.Query()
	if r.URL.Path == "/ws/pane" { // the live terminal names its pane as id
		mc, err := s.machineFor(r, "")
		return err == nil && !s.viewerSeesPane(mc.ID(), q.Get("id"))
	}
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		return false // the page's own address names a pane too, and one gone since must still load it
	}
	pane, dir, session := q.Get("pane"), q.Get("dir"), q.Get("session")
	if pane == "" && dir == "" {
		return false
	}
	mc, err := s.machineFor(r, "")
	if err != nil {
		return false // the route answers for a machine it does not know
	}
	if dir != "" && s.ownFolder(mc.ID(), dir) {
		return true
	}
	// a session is found by its id in every project folder and nothing says whose it was: one a pane on the viewer's
	// roster is in is read through that pane, the very file it is writing, and no other is read at all
	if conversationRoute(r.URL.Path) && (session != "" || pane == "") {
		pane = s.viewerSessionPane(mc, pane, dir, session)
		if pane == "" {
			return true
		}
		q.Set("pane", pane)
		q.Del("dir")
		q.Del("session")
		r.URL.RawQuery = q.Encode()
	}
	if pane == "" {
		return false
	}
	if strings.HasPrefix(r.URL.Path, "/api/timelapse/") {
		return s.ownPane(mc.ID(), pane) // a recording outlives its pane; the listing itself leaves out the person's
	}
	return !s.viewerSeesPane(mc.ID(), pane) // the routes read the pane by its id: one the roster has not shown stays shut
}

// viewerSessionPane is the pane on a viewer's roster that is in the session, as herdr names it or, after a compaction, by
// the file the conversation went on in: the pane named, else one in the folder or its workspace; "" when none is.
func (s *Server) viewerSessionPane(mc *machines.Machine, paneID, dir, session string) string {
	snap := s.snapshotOf(mc.ID())
	if snap == nil || session == "" {
		return ""
	}
	style := mc.Paths()
	for i := range snap.Panes {
		pane := &snap.Panes[i]
		if pane.AgentSession == nil || s.viewerOwn(mc.ID(), pane.WorkspaceID) || (paneID != "" && paneID != pane.PaneID) {
			continue
		}
		if pane.AgentSession.Kind == "id" && pane.AgentSession.Value == session {
			return pane.PaneID
		}
		if ws := snap.Workspace(pane.WorkspaceID); paneID == "" && !style.Equal(pane.Dir(), dir) && (ws == nil || !style.Equal(snap.WorkspaceDir(ws), dir)) {
			continue // finding the file below asks the machine: only for the panes the request is about
		}
		if _, live, ok := s.Chat.SessionFile(mc.ID(), pane); ok && strings.TrimSuffix(style.Base(live), ".jsonl") == session {
			return pane.PaneID
		}
	}
	return ""
}

func (s *Server) snapshotOf(machineID string) *coreapi.Snapshot {
	if s.Watch == nil {
		return nil
	}
	snap, _ := s.Watch.Snapshot(machineID)
	return snap
}

// paneOf is the pane as the watcher's last snapshot has it, nil when it does not.
func (s *Server) paneOf(machineID, paneID string) (*coreapi.Snapshot, *coreapi.PaneInfo) {
	snap := s.snapshotOf(machineID)
	if snap == nil {
		return nil, nil
	}
	return snap, snap.Pane(paneID)
}

// viewerOwn says a viewer does not get the workspace: one of remuda's own, or a floating terminal just ended that the
// watcher's last snapshot still shows.
func (s *Server) viewerOwn(machineID, workspaceID string) bool {
	return s.ownWorkspace(machineID, workspaceID) || (s.Scratch != nil && s.Scratch.Ended(machineID, workspaceID))
}

// viewerSeesPane says the pane is on the roster a viewer gets.
func (s *Server) viewerSeesPane(machineID, paneID string) bool {
	_, pane := s.paneOf(machineID, paneID)
	return pane != nil && !s.viewerOwn(machineID, pane.WorkspaceID)
}

// ownPane says the pane stands in one of remuda's own workspaces now.
func (s *Server) ownPane(machineID, paneID string) bool {
	_, pane := s.paneOf(machineID, paneID)
	return pane != nil && s.viewerOwn(machineID, pane.WorkspaceID)
}

// ownFolder says only remuda's own workspaces stand in the folder: the floating terminal's is the home, which a
// workspace of the person's may share.
func (s *Server) ownFolder(machineID, dir string) bool {
	snap := s.snapshotOf(machineID)
	mc, err := s.Machines.Get(machineID)
	if snap == nil || err != nil {
		return false
	}
	same := func(folder string) bool { return mc.Paths().Equal(folder, dir) }
	return slices.ContainsFunc(snap.WorkspaceDirs(), same) && !slices.ContainsFunc(s.foldersBut(machineID, snap, s.viewerOwn), same)
}

// ownRecording says a timelapse is of remuda's own workspaces, as marked when it began. One made before the mark is
// shown only from a folder a viewer reaches now, under no name those workspaces are made with.
func (s *Server) ownRecording(m timelapse.Meta) bool {
	if m.Own != nil {
		return *m.Own
	}
	if m.Workspace == scratch.Label || s.ownPane(m.Machine, m.Pane) {
		return true
	}
	mc, err := s.Machines.Get(m.Machine)
	return err != nil || !s.viewerReach(mc).Allows(m.Dir)
}

// viewerFeed is the activity without remuda's own workspaces, by the mark a row was written with; a row without one, of
// a pane that cannot be told apart any more, is kept from a viewer too.
func (s *Server) viewerFeed(entries []feed.Entry) []feed.Entry {
	return slices.DeleteFunc(entries, func(e feed.Entry) bool {
		switch {
		case e.Kind == "digest": // a standup digest kept from before they went: it quotes the agents' answers, and says not whose
			return true
		case e.Pane == "":
			return false
		case e.Own != nil:
			return *e.Own
		}
		return !s.viewerSeesPane(e.Machine, e.Pane)
	})
}

// viewerUsage keeps the folders within a viewer's reach and no session: what it would open one by is not a viewer's.
func (s *Server) viewerUsage(reports []usage.MachineReport) []usage.MachineReport {
	for i := range reports {
		report := reports[i].Report
		if report == nil {
			continue
		}
		report.Sessions = []usage.SessionTotals{}
		mc, err := s.Machines.Get(reports[i].ID)
		if err != nil {
			report.Projects = []usage.ProjectTotals{}
			continue
		}
		reach := s.viewerReach(mc)
		report.Projects = slices.DeleteFunc(report.Projects, func(p usage.ProjectTotals) bool { return !reach.Allows(p.Dir) })
	}
	return reports
}
