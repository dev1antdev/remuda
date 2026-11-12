// Package concierge is the cockpit's one concierge: an ordinary Claude Code agent remuda starts in a folder of its own
// on the machine the person picks, whose only tools are remuda's own MCP door — it reads what the agents do and the
// task list, and files tasks — so the person can ask "what is going on?" in plain words. remuda keeps its workspace and
// its conversation per machine and brings the conversation back when the agent starts there again; nothing is ever
// sent to it but what the person types.
package concierge

import (
	"errors"
	"log"
	"sync"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/state"
	"github.com/dev1antdev/remuda/internal/text"
)

const (
	file = "concierge.json"
	// Label is what its workspace is called in herdr
	Label = "concierge"
	// Agent is the kind it runs
	Agent = "claude"
)

// Place is the concierge on one machine: its workspace in herdr while one stands, and its conversation, resumed when it
// starts there again.
type Place struct {
	Workspace string `json:"workspace,omitempty"`
	Session   string `json:"session,omitempty"`
}

type kept struct {
	Machine string           `json:"machine,omitempty"` // the person's pick; "" follows the default
	Places  map[string]Place `json:"places,omitempty"`  // machine id → the concierge there
}

// Service keeps the concierge's places in the state dir; one start or close runs at a time.
type Service struct {
	dir     string
	mu      sync.Mutex
	kept    kept
	ended   map[string]bool // machine|workspace of a concierge closed while remuda runs: a snapshot read before the close still has it
	running sync.Mutex      // held by a start and a close
}

// Open reads what is kept in dir; a damaged file reads as nothing kept, so the next start makes a fresh concierge.
func Open(dir string) *Service {
	s := &Service{dir: dir, ended: map[string]bool{}}
	if err := state.LoadJSON(dir, file, &s.kept); err != nil {
		log.Printf("%s is unreadable (%v): the concierge starts afresh", file, err)
		s.kept = kept{}
	}
	if s.kept.Places == nil {
		s.kept.Places = map[string]Place{}
	}
	return s
}

// Is says whether the workspace is the concierge's on that machine, the one standing or one closed since remuda started.
func (s *Service) Is(machine, workspace string) bool {
	if workspace == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.kept.Places[machine].Workspace == workspace || s.ended[machine+"|"+workspace]
}

// Picked is the machine the person picked, "" when the default is followed.
func (s *Service) Picked() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.kept.Machine
}

// Pick keeps the person's machine; "" goes back to the default.
func (s *Service) Pick(machine string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kept.Machine = machine
	return s.saveLocked()
}

// Place is the concierge on a machine as kept.
func (s *Service) Place(machine string) Place {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.kept.Places[machine]
}

// Standing lists the machines other than the one named where a concierge's workspace is kept: the ones to close.
func (s *Service) Standing(except string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for machine, place := range s.kept.Places {
		if machine != except && place.Workspace != "" {
			out = append(out, machine)
		}
	}
	return out
}

// update changes a machine's place and keeps it; a place left with nothing is dropped.
func (s *Service) update(machine string, change func(*Place)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	place := s.kept.Places[machine]
	was := place
	change(&place)
	if place == was {
		return
	}
	if place == (Place{}) {
		delete(s.kept.Places, machine)
	} else {
		s.kept.Places[machine] = place
	}
	if err := s.saveLocked(); err != nil {
		log.Printf("%s: %v", file, err)
	}
}

// Observe notes the conversation the concierge's agent is on, from a machine's snapshot: a new one after a /clear, the
// first once the agent has written it.
func (s *Service) Observe(machine string, snap *herdr.Snapshot) {
	if snap == nil {
		return
	}
	workspace := s.Place(machine).Workspace
	if workspace == "" {
		return
	}
	for i := range snap.Panes {
		pane := &snap.Panes[i]
		if pane.WorkspaceID != workspace || pane.Agent != Agent {
			continue
		}
		if session := sessionOf(pane); session != "" {
			s.update(machine, func(p *Place) {
				if p.Workspace == workspace {
					p.Session = session
				}
			})
		}
		return
	}
}

// Forget drops a removed machine's place; herdr there is not asked anything.
func (s *Service) Forget(machine string) {
	s.update(machine, func(p *Place) { *p = Place{} })
}

func (s *Service) saveLocked() error { return state.SaveJSON(s.dir, file, s.kept) }

// sessionOf is the pane's Claude Code session id, when herdr's hooks reported one that looks like an id.
func sessionOf(pane *herdr.PaneInfo) string {
	if pane.AgentSession == nil || pane.AgentSession.Kind != "id" || !text.SessionID(pane.AgentSession.Value) {
		return ""
	}
	return pane.AgentSession.Value
}

// Herdr is what the concierge asks of its machine's herdr.
type Herdr interface {
	sessions.Starter
	WorkspaceCreate(cwd, label string) (*herdr.WorkspaceCreateResult, error)
	WorkspaceClose(workspaceID string) error
	PaneRead(paneID string) (string, error)
	PaneSendKeys(paneID string, keys []string) error
}

// ErrNone is a close asked of a machine with no concierge standing.
var ErrNone = errors.New("no concierge is running on that machine")

// Close ends the concierge on a machine: its workspace closed while herdr still has it, and forgotten; fresh forgets
// its conversation too, so the next start is a new one (New conversation), else that start resumes it.
func (s *Service) Close(machine string, client Herdr, snap *herdr.Snapshot, fresh bool) error {
	s.running.Lock()
	defer s.running.Unlock()
	return s.closeLocked(machine, client, snap, fresh)
}

func (s *Service) closeLocked(machine string, client Herdr, snap *herdr.Snapshot, fresh bool) error {
	place := s.Place(machine)
	if place.Workspace == "" {
		if fresh {
			s.update(machine, func(p *Place) { p.Session = "" })
			return nil
		}
		return ErrNone
	}
	if snap == nil || snap.Workspace(place.Workspace) != nil {
		if err := client.WorkspaceClose(place.Workspace); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.ended[machine+"|"+place.Workspace] = true
	s.mu.Unlock()
	s.update(machine, func(p *Place) {
		p.Workspace = ""
		if fresh {
			p.Session = ""
		}
	})
	return nil
}
