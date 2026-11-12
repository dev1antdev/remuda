package app

import (
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/undo"
	"github.com/dev1antdev/remuda/internal/watch"
)

// turnUndo is the agents' turns kept to be undone: a checkpoint of the folder at each turn's start and end over the
// machine's shell, and the undo itself, refused while an agent works in the folder as the watcher last saw it.
func turnUndo(dir string, fleet *machines.Manager, watcher *watch.Watcher, record bool) *undo.Service {
	connected := func(machineID string) (*machines.Machine, error) {
		mc, err := fleet.Get(machineID)
		if err != nil {
			return nil, err
		}
		if _, _, err := mc.Ready(); err != nil {
			return nil, err
		}
		return mc, nil
	}
	store := undo.OpenStore(dir)
	tracker := undo.NewTracker(store, undo.Inputs{Host: func(machineID string) (shell.Host, error) {
		mc, err := connected(machineID)
		if err != nil {
			return nil, err
		}
		return mc, nil
	}})
	if !record {
		tracker.Scratch()
	}
	return &undo.Service{
		Store:   store,
		Tracker: tracker,
		Host: func(machineID string) (undo.Host, error) {
			mc, err := connected(machineID)
			if err != nil {
				return nil, err
			}
			return mc, nil
		},
		Working: func(machineID, folder string) string {
			snap, _ := watcher.Snapshot(machineID)
			if snap == nil {
				return ""
			}
			for i := range snap.Panes {
				pane := &snap.Panes[i]
				if pane.Agent == "" || (pane.AgentStatus != herdr.StatusWorking && pane.AgentStatus != herdr.StatusBlocked) {
					continue
				}
				if ws := snap.Workspace(pane.WorkspaceID); ws != nil && snap.WorkspaceDir(ws) == folder {
					return pane.AgentName()
				}
			}
			return ""
		},
	}
}
