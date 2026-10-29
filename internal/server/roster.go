package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/dev1antdev/remuda/internal/awake"
	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/prstatus"
	"github.com/dev1antdev/remuda/internal/radar"
	"github.com/dev1antdev/remuda/internal/restart"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/transcript"
	"github.com/dev1antdev/remuda/internal/usage"
	buildversion "github.com/dev1antdev/remuda/internal/version"
)

// The roster: every machine with herdr's hierarchy, workspace → tabs → panes, as the sidebar draws it.

func (s *Server) rosterRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/roster", s.handleRoster)
}

type rosterPane struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Agent   string `json:"agent"`
	CWD     string `json:"cwd"`
	Focused bool   `json:"focused"`
	Status  string `json:"status"` // run | input | idle | done | shell
	Cols    int    `json:"cols"`
	Rows    int    `json:"rows"`
	// an agent's lifecycle as a person reads it (held done, held question, last activity); nil for a shell
	Radar *radar.Display `json:"radar,omitempty"`
	Usage *usage.Display `json:"usage,omitempty"`
	Said  string         `json:"said,omitempty"` // the first line of the last answer, once the agent finished
	PR    *rosterPR      `json:"pr,omitempty"`   // the last pull or merge request the agent named
}

// rosterPR is the request the agent named and, once gh on the machine read it, how it stands.
type rosterPR struct {
	transcript.Link
	Status *prstatus.Status `json:"status,omitempty"`
}

type rosterTab struct {
	ID     string       `json:"id"`
	Label  string       `json:"label"`
	Number int          `json:"number"`
	Named  bool         `json:"named"` // the user gave it a name (herdr labels unnamed tabs by number)
	Status string       `json:"status"`
	Panes  []rosterPane `json:"panes"`
}

type rosterReview struct {
	Changed int  `json:"changed"`         // uncommitted files in the folder: changed against HEAD, or untracked
	Fresh   int  `json:"fresh"`           // of those, moved since the reviewer looked
	Ready   bool `json:"ready,omitempty"` // rejected at the last look, and the agent has said it is ready for another
}

type rosterWorktree struct {
	RepoKey      string `json:"repo_key"`
	RepoName     string `json:"repo_name"`
	Linked       bool   `json:"linked"`
	CheckoutPath string `json:"checkout_path"`
}

type rosterSession struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Status      string          `json:"status"`
	ActiveTabID string          `json:"active_tab_id"`
	Worktree    *rosterWorktree `json:"worktree,omitempty"`
	Dir         string          `json:"dir,omitempty"`
	Review      *rosterReview   `json:"review,omitempty"`
	MutedUntil  *time.Time      `json:"muted_until,omitempty"` // the folder is silenced until then: no push, no webhook, no feed row
	Tabs        []rosterTab     `json:"tabs"`
}

type rosterMachine struct {
	ID       string            `json:"id"`
	Label    string            `json:"label"`
	Host     string            `json:"host,omitempty"`
	Source   string            `json:"source,omitempty"`
	State    string            `json:"state"` // connected | connecting | error
	Error    string            `json:"error,omitempty"`
	Sessions []rosterSession   `json:"sessions"`
	Health   *health.Report    `json:"health,omitempty"`
	Awake    *awake.Status     `json:"awake,omitempty"`   // the keep-awake switch and whether the hold stands
	Stopped  []restart.Stopped `json:"stopped,omitempty"` // the chats a reboot stopped: their panes are bare shells again, a Resume brings them back
	MAC      string            `json:"mac,omitempty"`     // the address a wake-up packet names, when the person set one
}

// sessionsFromSnapshot keeps herdr's own hierarchy, workspace → tabs → panes, in herdr's order.
func (s *Server) sessionsFromSnapshot(machineID string, snap *herdr.Snapshot) []rosterSession {
	panesByTab := map[string][]rosterPane{}
	agentInTab := map[string]bool{}
	for i := range snap.Panes {
		pane := &snap.Panes[i]
		cols, rows := snap.PaneSize(pane.PaneID)
		entry := rosterPane{
			ID:      pane.PaneID,
			Title:   pane.DisplayTitle(),
			Agent:   pane.AgentName(),
			CWD:     pane.Dir(),
			Focused: pane.Focused || pane.PaneID == snap.FocusedPaneID,
			Status:  herdr.UIStatus(pane.AgentStatus, pane.Agent != ""),
			Cols:    cols,
			Rows:    rows,
		}
		if pane.Agent != "" {
			display := s.Radar.Display(machineID, pane)
			entry.Radar = &display
			entry.Said = s.Said.Line(machineID, pane.PaneID)
			if link, ok := s.Said.Link(machineID, pane.PaneID); ok {
				entry.PR = &rosterPR{Link: link}
				if status, ok := s.PRs.Status(machineID, pane.PaneID); ok {
					entry.PR.Status = &status
				}
			}
			if meter, ok := s.Usage.Display(machineID, pane.PaneID); ok {
				entry.Usage = &meter
			}
		}
		panesByTab[pane.TabID] = append(panesByTab[pane.TabID], entry)
		if pane.Agent != "" {
			agentInTab[pane.TabID] = true
		}
	}
	tabsByWS := map[string][]rosterTab{}
	agentInWS := map[string]bool{}
	for _, tab := range snap.Tabs {
		panes := panesByTab[tab.TabID]
		if panes == nil {
			panes = []rosterPane{}
		}
		tabsByWS[tab.WorkspaceID] = append(tabsByWS[tab.WorkspaceID], rosterTab{
			ID: tab.TabID, Label: tab.Label, Number: tab.Number,
			Named:  tab.Label != strconv.Itoa(tab.Number),
			Status: herdr.UIStatus(tab.AgentStatus, agentInTab[tab.TabID]), Panes: panes,
		})
		if agentInTab[tab.TabID] {
			agentInWS[tab.WorkspaceID] = true
		}
	}
	out := make([]rosterSession, 0, len(snap.Workspaces))
	for _, ws := range snap.Workspaces {
		tabs := tabsByWS[ws.WorkspaceID]
		if tabs == nil {
			tabs = []rosterTab{}
		}
		session := rosterSession{
			ID:          ws.WorkspaceID,
			Name:        ws.Label,
			Status:      herdr.UIStatus(ws.AgentStatus, agentInWS[ws.WorkspaceID]),
			ActiveTabID: ws.ActiveTabID,
			Tabs:        tabs,
		}
		if ws.Worktree != nil {
			session.Worktree = &rosterWorktree{
				RepoKey: ws.Worktree.RepoKey, RepoName: ws.Worktree.RepoName,
				Linked: ws.Worktree.IsLinkedWorktree, CheckoutPath: ws.Worktree.CheckoutPath,
			}
		}
		session.Dir = snap.WorkspaceDir(&ws)
		if session.Dir != "" && s.Push != nil {
			if until, muted := s.Push.Mutes.Until(machineID, session.Dir); muted {
				session.MutedUntil = &until
			}
		}
		if session.Dir != "" {
			badge, ok := s.Review.Badge(machineID, session.Dir)
			ready := s.Review.Ready(machineID, session.Dir)
			if (ok && badge.Changed > 0) || ready {
				session.Review = &rosterReview{Changed: badge.Changed, Fresh: badge.Fresh, Ready: ready}
			}
		}
		out = append(out, session)
	}
	return out
}

// handleRoster serves every machine from the watcher's cache, with an ETag so an unchanged roster is a 304.
func (s *Server) handleRoster(w http.ResponseWriter, r *http.Request) {
	s.PRs.Touch() // somebody is looking: the pull requests named are worth reading again
	body, etag := s.rosterBody()
	w.Header().Set("X-Remuda-Version", buildversion.Build) // a page older than the server offers a reload
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

// RosterETag is the roster's ETag as it stands: the live hub announces a change when it moves.
func (s *Server) RosterETag() string {
	_, etag := s.rosterBody()
	return etag
}

// rosterBody is the roster as served, with its ETag, from the caches alone.
func (s *Server) rosterBody() ([]byte, string) {
	list := s.Machines.List()
	out := make([]rosterMachine, 0, len(list))
	for _, mc := range list {
		state, errMsg := mc.State()
		entry := rosterMachine{
			ID: mc.ID(), Label: mc.Label(), Host: mc.Cfg.Host, Source: mc.Cfg.Source,
			State: state, Error: errMsg, Sessions: []rosterSession{},
		}
		if state == machines.StateConnected {
			snap, err := s.Watch.Snapshot(mc.ID())
			if snap != nil {
				entry.Sessions = s.sessionsFromSnapshot(mc.ID(), snap)
			}
			if err != nil {
				entry.State, entry.Error = machines.StateError, err.Error()
			}
		}
		entry.Health = s.Health.Report(mc.ID())
		if s.Awake != nil {
			status := s.Awake.Status(mc.ID())
			entry.Awake = &status
		}
		if s.Updates != nil && s.Updates.Restart != nil {
			entry.Stopped = s.Updates.Restart.Stopped(mc.ID())
		}
		entry.MAC = s.Machines.MAC(mc.ID())
		out = append(out, entry)
	}
	roster := map[string]any{"machines": out, "agents": sessions.Agents, "version": buildversion.Build, "latest": s.Health.Latest()}
	if s.Feed != nil {
		roster["feed"] = s.Feed.LastID() // the newest activity entry: the bell knows there is news without asking
	}
	if s.Tailscale != nil {
		roster["tailscale"] = s.Tailscale()
	}
	if s.Budgets != nil {
		roster["budgets"] = s.Budgets.Status() // from the cache: the pill and the rows know the limits without another poll
	}
	body, _ := json.Marshal(roster)
	sum := sha256.Sum256(body)
	return body, `"` + hex.EncodeToString(sum[:12]) + `"`
}
