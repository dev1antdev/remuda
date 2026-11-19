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
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/scratch"
	"github.com/dev1antdev/remuda/internal/timelapse"
	"github.com/dev1antdev/remuda/internal/transcript"
	"github.com/dev1antdev/remuda/internal/usage"
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
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		return false // the page's own address names a pane too, and one gone since must still load it
	}
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
		conversation := r.URL.Path == "/api/chat" || r.URL.Path == "/api/recap"
		return conversation && !s.viewerReadsSession(mc, dir, q.Get("session"))
	}
	if strings.HasPrefix(r.URL.Path, "/api/timelapse/") {
		return s.ownPane(mc.ID(), pane) // a recording outlives its pane; the listing itself leaves out the person's
	}
	return !s.viewerSeesPane(mc.ID(), pane) // the routes read the pane by its id: one the roster has not shown stays shut
}

// viewerReadsSession says a viewer may read a conversation by folder and id: the folder is within the workspaces it
// reaches, and so is the transcript, since a session is looked for in every project folder whatever folder is named.
func (s *Server) viewerReadsSession(mc *machines.Machine, dir, session string) bool {
	reach := s.viewerReach(mc)
	if !viewerMay(mc, reach, dir) {
		return false
	}
	if !sessionIDRe.MatchString(session) {
		return true // the route refuses it
	}
	found, err := mc.Transcripts.ResolveSession("", dir, "id", session)
	if err != nil || transcript.FormatOf(found) != transcript.Claude {
		return true // none to read, or a format whose files are not kept by folder
	}
	style := mc.Paths()
	return reach.HoldsProject(style.Base(style.Dir(found)), transcript.EncodePath)
}

func (s *Server) snapshotOf(machineID string) *herdr.Snapshot {
	if s.Watch == nil {
		return nil
	}
	snap, _ := s.Watch.Snapshot(machineID)
	return snap
}

// paneOf is the pane as the watcher's last snapshot has it, nil when it does not.
func (s *Server) paneOf(machineID, paneID string) (*herdr.Snapshot, *herdr.PaneInfo) {
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

// ownMark is whether the pane stands in one of remuda's own workspaces, for what is written about it; nil when the
// snapshot does not know the pane.
func (s *Server) ownMark(machineID, paneID string) *bool {
	_, pane := s.paneOf(machineID, paneID)
	if pane == nil {
		return nil
	}
	return new(s.viewerOwn(machineID, pane.WorkspaceID))
}

// ownRecording says a timelapse is of remuda's own workspaces, as marked when it began; one made before the mark goes
// by the name they are made under, the pane and folder they hold now, or a folder no viewer reads.
func (s *Server) ownRecording(m timelapse.Meta) bool {
	if m.Own != nil {
		return *m.Own
	}
	if m.Workspace == scratch.Label || m.Workspace == concierge.Label || s.ownPane(m.Machine, m.Pane) || s.ownFolder(m.Machine, m.Dir) {
		return true
	}
	mc, err := s.Machines.Get(m.Machine)
	return err == nil && s.viewerReach(mc).Closes(m.Dir) // the concierge's folder lies in a state dir
}

// viewerFeed is the activity without remuda's own workspaces, by the mark a row was written with; a row without one, of
// a pane that cannot be told apart any more, is kept from a viewer too.
func (s *Server) viewerFeed(entries []feed.Entry) []feed.Entry {
	return slices.DeleteFunc(entries, func(e feed.Entry) bool {
		switch {
		case e.Pane == "":
			return false
		case e.Own != nil:
			return *e.Own
		}
		return !s.viewerSeesPane(e.Machine, e.Pane)
	})
}

// viewerUsage keeps the folders within a viewer's reach, and with them the sessions a chat could be opened by.
func (s *Server) viewerUsage(reports []usage.MachineReport) []usage.MachineReport {
	for i := range reports {
		mc, err := s.Machines.Get(reports[i].ID)
		if err != nil || reports[i].Report == nil {
			continue
		}
		reach := s.viewerReach(mc)
		reports[i].Report.WithoutFolders(func(dir string) bool { return !reach.Allows(dir) })
	}
	return reports
}
