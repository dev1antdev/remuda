// Package sleep puts a workspace to sleep and wakes it, after Orca's hibernate mod: every agent of the workspace
// with a session is recorded in <state>/asleep.json and asked to leave the way a restart leaves one, its shell, pane
// and tab left standing; waking starts each again on its session in its own shell, a new tab for a pane gone, and
// never in a shell that runs something. The person's two switches — wake a workspace when it is opened, resume the
// chats a reboot stopped by themselves — are kept beside it.
package sleep

import (
	"cmp"
	"log"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/restart"
	"github.com/dev1antdev/remuda/internal/state"
)

const (
	asleepFile   = "asleep.json"
	settingsFile = "sleep-settings.json"
)

// Agent is one agent a sleep stopped: enough to bring it back on its session, where it stood.
type Agent struct {
	Pane    string `json:"pane"`
	Tab     string `json:"tab"`
	TabName string `json:"tab_name,omitempty"` // the tab's own name, when the person gave it one
	Kind    string `json:"kind"`
	Session string `json:"session"`
	Dir     string `json:"dir"`
	Title   string `json:"title,omitempty"`
}

// Workspace is a workspace asleep: its name and folder when it went to sleep, when, and the agents it stopped.
type Workspace struct {
	Name   string    `json:"name"`
	Dir    string    `json:"dir"`
	At     time.Time `json:"at"`
	Agents []Agent   `json:"agents"`
	// Core marks a workspace put to sleep on remuda-core; one without it, on a fleet moved to the core, slept in herdr
	Core bool `json:"core,omitempty"`
}

// Status is what the roster says of a workspace asleep: since when, and how many agents wait to be woken.
type Status struct {
	At     time.Time `json:"at"`
	Agents int       `json:"agents"`
}

// Settings are the person's two switches: wake a sleeping workspace when it is opened on the page, and resume the
// chats a reboot stopped without asking. Both off until switched on.
type Settings struct {
	WakeOnOpen          bool `json:"wake_on_open"`
	ResumeAutomatically bool `json:"resume_automatically"`
}

// Store keeps the workspaces asleep, by machine and workspace id, and the settings.
type Store struct {
	dir string
	// Core says the fleet is reached through remuda-core: a workspace asleep in herdr is then kept for Resume after
	// restart to carry its agents on (SleptInHerdr, Woke), whatever the core's snapshot shows.
	Core func() bool

	mu       sync.Mutex
	asleep   map[string]map[string]Workspace
	settings Settings
}

// Open reads what dir keeps; "" keeps it in memory alone. A damaged file is said once and read as nothing.
func Open(dir string) *Store {
	s := &Store{dir: dir, asleep: map[string]map[string]Workspace{}}
	if dir == "" {
		return s
	}
	if err := state.LoadJSON(dir, asleepFile, &s.asleep); err != nil {
		log.Printf("%s: %v — starting with no workspace asleep", asleepFile, err)
		s.asleep = map[string]map[string]Workspace{}
	}
	if s.asleep == nil {
		s.asleep = map[string]map[string]Workspace{}
	}
	if err := state.LoadJSON(dir, settingsFile, &s.settings); err != nil {
		log.Printf("%s: %v — the switches start off", settingsFile, err)
		s.settings = Settings{}
	}
	return s
}

// Of is the record of a workspace asleep.
func (s *Store) Of(machineID, workspaceID string) (Workspace, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.asleep[machineID][workspaceID]
	return w, ok
}

// Status is the roster's word on a workspace: nil while it is awake.
func (s *Store) Status(machineID, workspaceID string) *Status {
	w, ok := s.Of(machineID, workspaceID)
	if !ok {
		return nil
	}
	return &Status{At: w.At, Agents: len(w.Agents)}
}

// Holds says whether the pane is one a sleep stopped and a wake has not brought back yet.
func (s *Store) Holds(machineID, workspaceID, paneID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.asleep[machineID][workspaceID].Agents {
		if a.Pane == paneID {
			return true
		}
	}
	return false
}

// put records a workspace asleep; the agents a record of it held already stay, the newer word for a pane winning.
func (s *Store) put(machineID, workspaceID string, w Workspace) {
	w.Core = s.Core != nil && s.Core()
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.asleep[machineID][workspaceID]; ok {
		w.At = old.At
		w.Agents = merged(old.Agents, w.Agents)
	}
	if s.asleep[machineID] == nil {
		s.asleep[machineID] = map[string]Workspace{}
	}
	s.asleep[machineID][workspaceID] = w
	s.saveLocked()
}

// merged is the agents of both lists, one per pane, the later list's word for a pane kept.
func merged(old, now []Agent) []Agent {
	out := append([]Agent(nil), now...)
	have := map[string]bool{}
	for _, a := range now {
		have[a.Pane] = true
	}
	for _, a := range old {
		if !have[a.Pane] {
			out = append(out, a)
		}
	}
	return out
}

// keep leaves the workspace asleep with these agents alone; none wakes it.
func (s *Store) keep(machineID, workspaceID string, agents []Agent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.asleep[machineID][workspaceID]
	if !ok {
		return
	}
	if len(agents) == 0 {
		s.dropLocked(machineID, workspaceID)
	} else {
		w.Agents = agents
		s.asleep[machineID][workspaceID] = w
	}
	s.saveLocked()
}

// awake takes these panes off the workspace's record, the others staying asleep; none left wakes it.
func (s *Store) awake(machineID, workspaceID string, panes map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.awakeLocked(machineID, workspaceID, panes)
}

func (s *Store) awakeLocked(machineID, workspaceID string, panes map[string]bool) {
	w, ok := s.asleep[machineID][workspaceID]
	if !ok {
		return
	}
	var kept []Agent
	for _, a := range w.Agents {
		if !panes[a.Pane] {
			kept = append(kept, a)
		}
	}
	if len(kept) == 0 {
		s.dropLocked(machineID, workspaceID)
	} else {
		w.Agents = kept
		s.asleep[machineID][workspaceID] = w
	}
	s.saveLocked()
}

func (s *Store) dropLocked(machineID, workspaceID string) {
	delete(s.asleep[machineID], workspaceID)
	if len(s.asleep[machineID]) == 0 {
		delete(s.asleep, machineID)
	}
}

// Observe forgets a workspace asleep that a watched snapshot no longer shows: it was closed. A machine's first
// snapshot after it was away decides nothing, since herdr may still be restoring what it had. On a fleet moved to
// remuda-core a workspace that slept in herdr is no snapshot's to forget: its agents wait in Resume after restart.
func (s *Store) Observe(e events.Snapshot) bool {
	if e.Snap == nil || e.First {
		return false
	}
	onCore := s.Core != nil && s.Core()
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for workspaceID, w := range s.asleep[e.Machine] {
		if e.Snap.Workspace(workspaceID) == nil && (w.Core || !onCore) {
			s.dropLocked(e.Machine, workspaceID)
			changed = true
		}
	}
	if changed {
		s.saveLocked()
	}
	return changed
}

// SleptInHerdr is, by pane, the agents of the machine's workspaces put to sleep in herdr — the records without the
// core mark — for Resume after restart to carry on once the fleet is on remuda-core (restart.Memory.Sleepers).
func (s *Store) SleptInHerdr(machineID string) map[string]restart.Chat {
	s.mu.Lock()
	defer s.mu.Unlock()
	chats := map[string]restart.Chat{}
	for _, w := range s.asleep[machineID] {
		if w.Core {
			continue
		}
		for _, a := range w.Agents {
			chats[a.Pane] = restart.Chat{Kind: a.Kind, Session: a.Session, Dir: a.Dir, Title: cmp.Or(w.Name, a.Title)}
		}
	}
	return chats
}

// Woke takes a pane off the machine's records once its agent is back, or can never come back; its workspace wakes with
// its last agent (restart.Memory.Woken).
func (s *Store) Woke(machineID, paneID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for workspaceID, w := range s.asleep[machineID] {
		for _, a := range w.Agents {
			if a.Pane == paneID {
				s.awakeLocked(machineID, workspaceID, map[string]bool{paneID: true})
				return
			}
		}
	}
}

// Forget drops a machine's workspaces asleep (it left the registry).
func (s *Store) Forget(machineID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.asleep[machineID]; ok {
		delete(s.asleep, machineID)
		s.saveLocked()
	}
}

func (s *Store) saveLocked() {
	if s.dir == "" {
		return
	}
	if err := state.SaveJSON(s.dir, asleepFile, s.asleep); err != nil {
		log.Printf("%s: %v", asleepFile, err)
	}
}

// Settings are the two switches as they stand.
func (s *Store) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

// SetSettings keeps both switches.
func (s *Store) SetSettings(settings Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir != "" {
		if err := state.SaveJSON(s.dir, settingsFile, settings); err != nil {
			return err
		}
	}
	s.settings = settings
	return nil
}
