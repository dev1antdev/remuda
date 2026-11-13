package app

import (
	"github.com/dev1antdev/remuda/internal/gitstage"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/undo"
	"github.com/dev1antdev/remuda/internal/watch"
)

// stageWrites is the Changes lens's git writes, refused while an agent works or asks in the checkout as the watcher last
// saw it, or while one's turn is still ending there (the undo's tracker).
func stageWrites(fleet *machines.Manager, watcher *watch.Watcher, turns *undo.Service) *gitstage.Service {
	return &gitstage.Service{Busy: func(machineID, dir, top string) string {
		snap, _ := watcher.Snapshot(machineID)
		if snap == nil {
			return ""
		}
		home := ""
		if mc, err := fleet.Get(machineID); err == nil {
			home = mc.Home()
		}
		style := rpath.For(home)
		inCheckout := func(folder string) bool {
			return folder != "" && (style.Within(top, folder) || style.Within(dir, folder) || style.Equal(folder, dir))
		}
		for i := range snap.Panes {
			pane := &snap.Panes[i]
			if pane.Agent == "" || (pane.AgentStatus != herdr.StatusWorking && pane.AgentStatus != herdr.StatusBlocked) {
				continue
			}
			workspaceDir := ""
			if ws := snap.Workspace(pane.WorkspaceID); ws != nil {
				workspaceDir = snap.WorkspaceDir(ws)
			}
			if inCheckout(workspaceDir) || inCheckout(pane.Dir()) {
				return pane.AgentName()
			}
		}
		if turns == nil || turns.Tracker == nil {
			return ""
		}
		for i := range snap.Workspaces {
			if folder := snap.WorkspaceDir(&snap.Workspaces[i]); inCheckout(folder) && turns.Tracker.Going(machineID, folder) {
				return "an agent"
			}
		}
		return ""
	}}
}
