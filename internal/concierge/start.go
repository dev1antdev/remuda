package concierge

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/tasks"
)

// Host is the machine the concierge runs on, as a start needs it.
type Host struct {
	ID        string
	Home      string
	Client    Herdr
	Snapshot  func() (*herdr.Snapshot, error)       // a fresh one, read once no other start or close is under way
	Prepare   func() (folder string, err error)     // its folder on the machine with remuda's CLAUDE.md in it (FolderScript)
	Remuda    string                                // the remuda binary there, whose `mcp` is its one MCP server
	Resumable func(folder, session string) bool     // the conversation's file is still there to resume
	Lock      func(pane string) sync.Locker         // the pane's typing lock, held while its trust question is answered
	Status    func(pane string) string              // herdr's word on the pane's agent, from the watcher's last snapshot
	Started   func(machine, pane string, made bool) // told of a start, for the watcher to look again
}

// Running is the concierge standing on a machine.
type Running struct {
	Machine   string `json:"machine"`
	Workspace string `json:"workspace"`
	Pane      string `json:"pane"`
	Dir       string `json:"dir"`
	Started   bool   `json:"started"`           // the agent was started just now
	Resumed   bool   `json:"resumed,omitempty"` // and came back on the conversation kept
}

// Ensure is the concierge on the host: the one standing while its workspace runs Claude Code, its agent started again
// in its pane when herdr brought the pane back a bare shell, else a new workspace in its folder with the agent started
// in it — on the conversation kept for the machine while its file is there, else a fresh one.
func (s *Service) Ensure(h Host) (Running, error) {
	s.running.Lock()
	defer s.running.Unlock()
	snap, err := h.Snapshot()
	if snap == nil {
		if err == nil {
			err = errors.New("herdr sent no snapshot")
		}
		return Running{}, err
	}
	if workspace := s.Place(h.ID).Workspace; workspace != "" {
		if ws := snap.Workspace(workspace); ws != nil {
			if pane := paneOf(snap, workspace); pane != nil {
				run := Running{Machine: h.ID, Workspace: workspace, Pane: pane.PaneID, Dir: snap.WorkspaceDir(ws)}
				if pane.Agent != "" {
					return run, nil
				}
				folder, err := h.Prepare()
				if err != nil {
					return Running{}, err
				}
				run.Dir = folder
				return s.start(h, run, false)
			}
		}
		s.update(h.ID, func(p *Place) { p.Workspace = "" })
	}
	folder, err := h.Prepare()
	if err != nil {
		return Running{}, err
	}
	made, err := h.Client.WorkspaceCreate(folder, Label)
	if err != nil {
		return Running{}, err
	}
	s.update(h.ID, func(p *Place) { p.Workspace = made.Workspace.WorkspaceID })
	return s.start(h, Running{Machine: h.ID, Workspace: made.Workspace.WorkspaceID, Pane: made.RootPane.PaneID, Dir: folder}, true)
}

// Find is the concierge standing on a machine in its snapshot, without asking herdr anything.
func (s *Service) Find(machine string, snap *herdr.Snapshot) (Running, bool) {
	workspace := s.Place(machine).Workspace
	if snap == nil || workspace == "" {
		return Running{}, false
	}
	ws := snap.Workspace(workspace)
	pane := paneOf(snap, workspace)
	if ws == nil || pane == nil {
		return Running{}, false
	}
	return Running{Machine: machine, Workspace: workspace, Pane: pane.PaneID, Dir: snap.WorkspaceDir(ws)}, true
}

// start starts the agent in the concierge's pane, resuming the machine's conversation while its file is there; a
// conversation whose file is gone is forgotten, so the agent never starts on a resume that fails.
func (s *Service) start(h Host, run Running, made bool) (Running, error) {
	session := s.Place(h.ID).Session
	if session != "" && (h.Resumable == nil || !h.Resumable(run.Dir, session)) {
		s.update(h.ID, func(p *Place) { p.Session = "" })
		session = ""
	}
	if err := sessions.StartAgentWith(h.Client, run.Pane, Agent, sessions.Start{Resume: session, Args: Args(h.Remuda)}); err != nil {
		return Running{}, err
	}
	run.Started, run.Resumed = true, session != ""
	if h.Started != nil {
		h.Started(h.ID, run.Pane, made)
	}
	if !run.Resumed {
		go answerTrust(h, run.Pane, run.Dir, TrustWait, TrustEvery)
	}
	return run, nil
}

// TrustWait and TrustEvery pace the look for Claude Code's trust question after a fresh start: a read of the screen
// every so often until it is answered or the wait is over; TrustAtLeast is how long it looks whatever herdr says.
var (
	TrustWait    = 90 * time.Second
	TrustEvery   = 2 * time.Second
	TrustAtLeast = 10 * time.Second
)

// answerTrust answers Claude Code's "do you trust this folder" question for the concierge's own folder, the one remuda
// made — as a task's start does for a worktree it made — and nothing else on the screen; once herdr reads the agent as
// idle or at work (its hooks speak only in a trusted folder) and TrustAtLeast has passed, the look ends.
func answerTrust(h Host, pane, folder string, wait, every time.Duration) {
	began := time.Now()
	for deadline := began.Add(wait); time.Now().Before(deadline); {
		time.Sleep(every)
		screen, err := h.Client.PaneRead(pane)
		if err != nil || !tasks.TrustPrompt(screen, folder, h.Home) {
			if h.Status != nil && pastTrust(h.Status(pane)) && time.Since(began) >= TrustAtLeast {
				return
			}
			continue
		}
		if h.Lock != nil {
			lock := h.Lock(pane)
			lock.Lock()
			defer lock.Unlock()
		}
		if !tasks.CursorOnYes(screen) {
			_ = h.Client.PaneSendKeys(pane, []string{"down"})
			time.Sleep(150 * time.Millisecond)
		}
		if err := h.Client.PaneSendKeys(pane, []string{"enter"}); err != nil {
			log.Printf("the concierge's trust question on %s could not be answered: %v", h.ID, err)
		}
		return
	}
}

// pastTrust says whether herdr's word on the agent means it is up.
func pastTrust(status string) bool {
	return status == herdr.StatusIdle || status == herdr.StatusWorking || status == herdr.StatusDone
}

// paneOf is the concierge's pane in its workspace: the active tab's focused one, else its first, else any.
func paneOf(snap *herdr.Snapshot, workspaceID string) *herdr.PaneInfo {
	ws := snap.Workspace(workspaceID)
	if ws == nil {
		return nil
	}
	var first, inTab *herdr.PaneInfo
	for i := range snap.Panes {
		p := &snap.Panes[i]
		if p.WorkspaceID != workspaceID {
			continue
		}
		if first == nil {
			first = p
		}
		if p.TabID == ws.ActiveTabID {
			if p.Focused {
				return p
			}
			if inTab == nil {
				inTab = p
			}
		}
	}
	if inTab != nil {
		return inTab
	}
	return first
}
