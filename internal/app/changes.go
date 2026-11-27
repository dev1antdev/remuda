package app

import (
	"fmt"

	"github.com/dev1antdev/remuda/internal/coreapi"
	"github.com/dev1antdev/remuda/internal/gitstage"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/undo"
	"github.com/dev1antdev/remuda/internal/watch"
)

// stageWrites is the Changes lens's git writes, refused while an agent works or asks in the checkout as the watcher last
// saw it, or while one's turn is still ending there (the undo's tracker).
func stageWrites(fleet *machines.Manager, watcher *watch.Watcher, turns *undo.Service) *gitstage.Service {
	return &gitstage.Service{Check: func(host shell.Host, dir string) (string, error) {
		scan, err := review.ScanWork(host, dir, false)
		if err != nil {
			return "", err
		}
		switch {
		case len(scan.Findings) > 0:
			return fmt.Sprintf("the secret check finds %d secret(s) in what this commit would carry: look at them, then commit anyway", len(scan.Findings)), nil
		case scan.More > 0:
			return fmt.Sprintf("the secret check finds more secrets than it reads (%d more): look at them, then commit anyway", scan.More), nil
		case scan.Incomplete:
			return "the secret check could not read all of it: look, then commit anyway", nil
		}
		return "", nil
	}, Busy: func(machineID, dir, top string) string {
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
			if pane.Agent == "" || (pane.AgentStatus != coreapi.StatusWorking && pane.AgentStatus != coreapi.StatusBlocked) {
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
