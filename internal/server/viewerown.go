package server

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/concierge"
	"github.com/dev1antdev/remuda/internal/feed"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/scratch"
	"github.com/dev1antdev/remuda/internal/sent"
	"github.com/dev1antdev/remuda/internal/timelapse"
)

// The workspaces remuda keeps for itself — the floating terminal's shell, where a token may stand on the screen, and
// the concierge — are the person's alone: to a viewer they do not exist.

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

func (s *Server) viewerBarred(r *http.Request) bool {
	q := r.URL.Query()
	pane, dir := q.Get("pane"), q.Get("dir")
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
	if pane == "" {
		return false
	}
	if strings.HasPrefix(r.URL.Path, "/api/timelapse/") {
		return s.ownPane(mc.ID(), pane) // a recording outlives its pane; the listing itself leaves out the person's
	}
	return !s.viewerSeesPane(mc.ID(), pane) // the routes read the pane by its id: one the roster has not shown stays shut
}

func (s *Server) snapshotOf(machineID string) *herdr.Snapshot {
	if s.Watch == nil {
		return nil
	}
	snap, _ := s.Watch.Snapshot(machineID)
	return snap
}

// viewerSeesPane says the pane is on the roster a viewer gets.
func (s *Server) viewerSeesPane(machineID, paneID string) bool {
	snap := s.snapshotOf(machineID)
	if snap == nil {
		return false
	}
	pane := snap.Pane(paneID)
	return pane != nil && !s.ownWorkspace(machineID, pane.WorkspaceID)
}

// ownPane says the pane stands in one of remuda's own workspaces now.
func (s *Server) ownPane(machineID, paneID string) bool {
	snap := s.snapshotOf(machineID)
	if snap == nil {
		return false
	}
	pane := snap.Pane(paneID)
	return pane != nil && s.ownWorkspace(machineID, pane.WorkspaceID)
}

// ownFolder says only remuda's own workspaces stand in the folder: the floating terminal's is the home, which a
// workspace of the person's may share.
func (s *Server) ownFolder(machineID, dir string) bool {
	snap := s.snapshotOf(machineID)
	mc, err := s.Machines.Get(machineID)
	if snap == nil || err != nil {
		return false
	}
	own := false
	for i := range snap.Workspaces {
		ws := &snap.Workspaces[i]
		if !mc.Paths().Equal(snap.WorkspaceDir(ws), dir) {
			continue
		}
		if !s.ownWorkspace(machineID, ws.WorkspaceID) {
			return false
		}
		own = true
	}
	return own
}

// ownRecording says a timelapse is of remuda's own workspaces: by the name they are made under, which outlasts them
// and every recording made before this check, or by the pane and folder they hold now.
func (s *Server) ownRecording(m timelapse.Meta) bool {
	return m.Workspace == scratch.Label || m.Workspace == concierge.Label || s.ownPane(m.Machine, m.Pane) || s.ownFolder(m.Machine, m.Dir)
}

// viewerFeed is the activity without those workspaces. What was typed is kept from a viewer also for a pane no longer
// there to be told apart.
func (s *Server) viewerFeed(entries []feed.Entry) []feed.Entry {
	return slices.DeleteFunc(entries, func(e feed.Entry) bool {
		if e.Pane == "" {
			return false
		}
		return s.ownPane(e.Machine, e.Pane) || (e.Kind == sent.Kind && !s.viewerSeesPane(e.Machine, e.Pane))
	})
}
