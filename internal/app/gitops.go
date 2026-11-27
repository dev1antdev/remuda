package app

import (
	"path/filepath"

	"github.com/dev1antdev/remuda/internal/coreapi"
	"github.com/dev1antdev/remuda/internal/gitops"
	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/undo"
	"github.com/dev1antdev/remuda/internal/watch"
)

// gitWrites is git's writes at the person's press, refused while an agent works or asks in the checkout as the watcher
// last saw it, or while the undo tracker still has a turn going or ending there.
func gitWrites(watcher *watch.Watcher, turns *undo.Tracker) *gitops.Service {
	return &gitops.Service{Busy: func(machineID, top string, worktrees []string) string {
		snap, _ := watcher.Snapshot(machineID)
		real := func(dir string) string { return realOn(machineID, dir) }
		return busyCheckout(snap, top, worktrees, real, func(dir string) bool { return turns.Going(machineID, dir) })
	}}
}

// busyCheckout names who holds git's writes on the checkout at top back in the snapshot: an agent working or asking
// there — its workspace's folder in it, or its own pane's — else a turn still ending in a workspace folder of it; ""
// for nobody.
func busyCheckout(snap *coreapi.Snapshot, top string, worktrees []string, real func(string) string, going func(string) bool) string {
	if snap == nil {
		// no word yet on what its agents do — after a restart, a reconnect: the write waits for the next snapshot rather
		// than landing under an agent's feet
		return "the machine has not said what its agents do yet: try again in a moment"
	}
	inCheckout := func(dir string) bool {
		if checkoutOf(real(dir), top, worktrees) {
			return true
		}
		// an agent whose folder holds the checkout (~/code beside ~/code/repo) can write in it too
		if dir == "" {
			return false
		}
		style := rpath.For(top)
		return style.Within(real(dir), top)
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
		if !inCheckout(workspaceDir) && !inCheckout(pane.Dir()) {
			continue
		}
		if pane.AgentStatus == coreapi.StatusBlocked {
			return pane.AgentName() + " is waiting for your answer"
		}
		return pane.AgentName() + " is at work"
	}
	for _, dir := range snap.WorkspaceDirs() {
		if inCheckout(dir) && going(dir) {
			return "an agent's turn is still ending"
		}
	}
	return ""
}

// realOn is a folder of this machine with its links resolved, as git names a checkout's top (/tmp is /private/tmp on a
// Mac); another machine's folder as it is.
func realOn(machineID, dir string) string {
	if machineID == "local" && dir != "" {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			return real
		}
	}
	return dir
}

// checkoutOf says whether a workspace's folder lies in the checkout at top and in no other worktree of its repository
// nested inside it: a worktree under the main checkout is a checkout of its own.
func checkoutOf(dir, top string, worktrees []string) bool {
	style := rpath.For(top)
	if dir == "" || !style.Within(top, dir) {
		return false
	}
	for _, other := range worktrees {
		if !style.Equal(other, top) && style.Within(top, other) && style.Within(other, dir) {
			return false
		}
	}
	return true
}
