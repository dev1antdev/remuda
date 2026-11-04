// Package scratch is the floating terminal's shell: one plain-shell workspace per machine, made in the home folder on
// first use and found again by the id remuda kept for it, so it is never a workspace the person made.
package scratch

import (
	"errors"
	"log"
	"sync"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/state"
)

const (
	file = "scratch.json"
	// Label is what the workspace is called in herdr
	Label = "scratch"
)

// Herdr is what the store asks of a machine's herdr
type Herdr interface {
	WorkspaceCreate(cwd, label string) (*herdr.WorkspaceCreateResult, error)
	WorkspaceClose(workspaceID string) error
}

// Shell is a machine's scratch shell: its workspace, the pane the drawer attaches, and whether it was made just now
type Shell struct {
	Workspace string `json:"workspace"`
	Pane      string `json:"pane"`
	Made      bool   `json:"made"`
}

// Store keeps the scratch workspace's id per machine in the state dir.
type Store struct {
	dir string
	mu  sync.Mutex
	ids map[string]string // machine id → workspace id
}

// Open reads the ids kept in dir; a damaged file reads as none, so the next use makes a fresh shell.
func Open(dir string) *Store {
	s := &Store{dir: dir, ids: map[string]string{}}
	if err := state.LoadJSON(dir, file, &s.ids); err != nil {
		log.Printf("%s is unreadable (%v): the floating terminals start afresh", file, err)
		s.ids = map[string]string{}
	}
	return s
}

// Is says whether the workspace is the machine's scratch shell.
func (s *Store) Is(machine, workspace string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return workspace != "" && s.ids[machine] == workspace
}

// Ensure is the machine's scratch shell: the kept workspace while the snapshot still has it, else a new one in home.
func (s *Store) Ensure(machine string, client Herdr, snap *herdr.Snapshot, home string) (Shell, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id := s.ids[machine]; id != "" && snap != nil {
		if pane := paneOf(snap, id); pane != "" {
			return Shell{Workspace: id, Pane: pane}, nil
		}
	}
	made, err := client.WorkspaceCreate(home, Label)
	if err != nil {
		return Shell{}, err
	}
	s.ids[machine] = made.Workspace.WorkspaceID
	if err := s.save(); err != nil {
		log.Printf("the floating terminal's shell on %s was made but not kept: %v", machine, err)
	}
	return Shell{Workspace: made.Workspace.WorkspaceID, Pane: made.RootPane.PaneID, Made: true}, nil
}

// ErrNone is a close asked of a machine with no scratch shell.
var ErrNone = errors.New("no floating terminal is open on that machine")

// Close ends the machine's scratch shell and forgets it; a workspace herdr no longer has is only forgotten.
func (s *Store) Close(machine string, client Herdr, snap *herdr.Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.ids[machine]
	if id == "" {
		return ErrNone
	}
	if snap == nil || snap.Workspace(id) != nil {
		if err := client.WorkspaceClose(id); err != nil {
			return err
		}
	}
	delete(s.ids, machine)
	return s.save()
}

// Forget drops a removed machine's shell from the list; herdr there is not asked anything.
func (s *Store) Forget(machine string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.ids[machine]; !ok {
		return
	}
	delete(s.ids, machine)
	if err := s.save(); err != nil {
		log.Printf("%s: %v", file, err)
	}
}

func (s *Store) save() error { return state.SaveJSON(s.dir, file, s.ids) }

// paneOf is the pane of the workspace the drawer attaches: the active tab's focused one, else its first, else any
func paneOf(snap *herdr.Snapshot, workspaceID string) string {
	ws := snap.Workspace(workspaceID)
	if ws == nil {
		return ""
	}
	first, inTab := "", ""
	for i := range snap.Panes {
		p := &snap.Panes[i]
		if p.WorkspaceID != workspaceID {
			continue
		}
		if first == "" {
			first = p.PaneID
		}
		if p.TabID == ws.ActiveTabID {
			if p.Focused {
				return p.PaneID
			}
			if inTab == "" {
				inTab = p.PaneID
			}
		}
	}
	if inTab != "" {
		return inTab
	}
	return first
}
