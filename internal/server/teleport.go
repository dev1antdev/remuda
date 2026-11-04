package server

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/feed"
	"github.com/dev1antdev/remuda/internal/handover"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/recap"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/teleport"
)

// An agent's work moved to another machine: the checkout's branch, commits and uncommitted changes land in a new
// worktree of the same repository there, and an agent the person picks carries on with the conversation's gist.

func (s *Server) teleportRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/teleport/targets", s.handleTeleportTargets)
	api.HandleFunc("POST /api/teleport", s.handleTeleport)
	api.HandleFunc("GET /api/teleport/{job}", s.handleTeleportResult)
}

// teleportTarget is a connected machine and the checkouts the work may land in there, the same repository's first.
type teleportTarget struct {
	Machine   string              `json:"machine"`
	Label     string              `json:"label"`
	Checkouts []teleport.Checkout `json:"checkouts"`
	Error     string              `json:"error,omitempty"`
}

// teleported is a move as the page reads it once its job ended: where the agent now is.
type teleported struct {
	mu      sync.Mutex
	Machine string `json:"machine"`
	Pane    string `json:"pane,omitempty"`
	Dir     string `json:"dir,omitempty"`
	Branch  string `json:"branch,omitempty"`
}

// teleportSource is the pane asked for and the folder its workspace works in.
func (s *Server) teleportSource(mc *machines.Machine, paneID string) (*herdr.PaneInfo, string, error) {
	snap, err := s.Watch.Snapshot(mc.ID())
	if snap == nil {
		return nil, "", err
	}
	pane := snap.Pane(paneID)
	if pane == nil {
		return nil, "", fmt.Errorf("pane %s not found", paneID)
	}
	dir := pane.Dir()
	if ws := snap.Workspace(pane.WorkspaceID); ws != nil {
		if d := snap.WorkspaceDir(ws); d != "" {
			dir = d
		}
	}
	if dir == "" {
		return nil, "", errors.New("the pane's folder is not known")
	}
	return pane, dir, nil
}

// handleTeleportTargets is GET /api/teleport/targets?machine&pane: the work in the pane's checkout as it stands, and
// every connected machine's checkouts it could land in.
func (s *Server) handleTeleportTargets(w http.ResponseWriter, r *http.Request) {
	mc, _, _, ok := s.ready(w, r, "")
	if !ok {
		return
	}
	_, dir, err := s.teleportSource(mc, r.URL.Query().Get("pane"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	src, err := teleport.Inspect(mc, dir)
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	var connected []*machines.Machine
	for _, m := range s.Machines.List() {
		if state, _ := m.State(); state == machines.StateConnected {
			connected = append(connected, m)
		}
	}
	targets := make([]teleportTarget, len(connected))
	var wg sync.WaitGroup
	for i, m := range connected {
		wg.Add(1)
		go func() {
			defer wg.Done()
			target := teleportTarget{Machine: m.ID(), Label: m.Label(), Checkouts: []teleport.Checkout{}}
			var dirs []string
			if snap, _ := s.Watch.Snapshot(m.ID()); snap != nil {
				dirs = snap.WorkspaceDirs()
			}
			found, err := teleport.Checkouts(m, dirs, src)
			if err != nil {
				target.Error = err.Error()
			}
			for _, c := range found {
				if m.ID() != mc.ID() || c.Dir != src.Root {
					target.Checkouts = append(target.Checkouts, c)
				}
			}
			targets[i] = target
		}()
	}
	wg.Wait()
	writeJSON(w, map[string]any{"source": src, "words": src.Words(), "targets": targets})
}

type teleportReq struct {
	Machine string `json:"machine"`
	Pane    string `json:"pane"`
	Target  string `json:"target"` // the machine the work goes to
	Folder  string `json:"folder"` // a checkout of the same repository there
	Agent   string `json:"agent"`
	Model   string `json:"model"`
	Args    string `json:"args"`
	Turns   *int   `json:"turns"` // the last exchanges carried in full; 3 when unsaid
}

// handleTeleport is POST /api/teleport: the move runs as a job on the target machine, every step in its log; the
// answer is the job, and GET /api/teleport/{job} says where the agent is once it ended.
func (s *Server) handleTeleport(w http.ResponseWriter, r *http.Request) {
	var req teleportReq
	if !decode(w, r, &req) {
		return
	}
	if req.Agent == "" || req.Agent == "shell" || !sessions.KnownAgent(req.Agent) {
		httpErr(w, 400, fmt.Errorf("an agent to carry on is required, not %q", req.Agent))
		return
	}
	if !path.IsAbs(req.Folder) {
		httpErr(w, 400, errors.New("the folder on the target must be an absolute path"))
		return
	}
	start, err := (&createReq{Agent: req.Agent, Model: req.Model, Args: req.Args}).start()
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	if s.Updates == nil || s.Updates.Jobs == nil {
		httpErr(w, 503, errors.New("no job runner"))
		return
	}
	mc, _, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	pane, dir, err := s.teleportSource(mc, req.Pane)
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	target, err := s.Machines.Get(req.Target)
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	client, _, err := target.Ready()
	if err != nil {
		httpErr(w, 502, err)
		return
	}
	lastN := 3
	if req.Turns != nil {
		lastN = *req.Turns
	}
	gist := s.teleportGist(mc, pane, lastN)
	to := agentLabel(req.Agent)
	result := &teleported{Machine: target.ID()}
	move := teleport.Move{Source: mc, SourceDir: dir, Target: target, TargetDir: req.Folder, Start: func(landed teleport.Landed, src teleport.Source) error {
		snap, _ := s.Watch.Snapshot(target.ID())
		place, err := sessions.StartIn(client, snap, landed.Dir, to, req.Agent, start)
		s.Watch.Refresh(target)
		if err != nil {
			return err
		}
		result.mu.Lock()
		result.Pane = place.Pane
		result.mu.Unlock()
		prompt := teleport.Preamble(mc.Label(), dir, src, landed, target.Label())
		if gist != "" {
			prompt += "\n" + gist
		}
		go s.firstPrompt(client, events.PaneRef{Machine: target.ID(), Pane: place.Pane}, prompt, landed.Branch)
		return nil
	}}
	title := fmt.Sprintf("Move the work in %s to %s", path.Base(dir), target.Label())
	command := fmt.Sprintf("# from %s (%s) to %s (%s):\n# the branch, its commits and the uncommitted work into a new worktree there, then %s with the handover", mc.Label(), dir, target.Label(), req.Folder, to)
	job := s.Updates.Jobs.Run(target, title, command, func(say func(string)) error {
		if gist == "" && pane.Agent != "" {
			say("no conversation to carry: the agent there starts with where the work came from")
		}
		landed, err := move.Run(say)
		result.mu.Lock()
		result.Dir, result.Branch = landed.Dir, landed.Branch
		result.mu.Unlock()
		if err != nil {
			return err
		}
		say(fmt.Sprintf("done: %s carries on in %s on %s; the agent on %s keeps running until you close it", to, landed.Dir, target.Label(), mc.Label()))
		if s.Feed != nil {
			s.Feed.Add(feed.Entry{Kind: "teleport", Machine: target.ID(), Pane: result.Pane, Title: "Moved " + path.Base(dir) + " to " + target.Label(), Body: landed.Branch + " · " + to})
		}
		return nil
	})
	s.teleports.Store(job.ID, result)
	writeJSON(w, map[string]any{"job": job.ID})
}

// teleportGist is the pane's conversation summed up as the handover's text; "" for a shell or one that cannot be read.
func (s *Server) teleportGist(mc *machines.Machine, pane *herdr.PaneInfo, lastN int) string {
	if pane.Agent == "" || s.Chat == nil {
		return ""
	}
	conv, err := s.Chat.Open(mc, pane.PaneID, "", "", false)
	if err != nil || conv.Path == "" {
		return ""
	}
	turns, err := conv.Turns()
	if err != nil {
		return ""
	}
	return handover.Text(pane.AgentName(), recap.Of(turns, time.Time{}), turns, lastN)
}

// handleTeleportResult is GET /api/teleport/{job}: where the agent carrying on is, as far as the move got.
func (s *Server) handleTeleportResult(w http.ResponseWriter, r *http.Request) {
	value, ok := s.teleports.Load(r.PathValue("job"))
	if !ok {
		httpErr(w, 404, errors.New("no such move"))
		return
	}
	result := value.(*teleported)
	result.mu.Lock()
	defer result.mu.Unlock()
	writeJSON(w, result)
}
