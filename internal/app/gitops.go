package app

import (
	"path/filepath"

	"github.com/dev1antdev/remuda/internal/gitops"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/undo"
	"github.com/dev1antdev/remuda/internal/watch"
)

// gitWrites is git's writes at the person's press, refused while an agent works or asks in the checkout as the watcher
// last saw it, or while the undo tracker still has a turn going or ending there.
func gitWrites(watcher *watch.Watcher, turns *undo.Tracker) *gitops.Service {
	return &gitops.Service{Busy: func(machineID, top string, worktrees []string) string {
		snap, _ := watcher.Snapshot(machineID)
		if snap == nil {
			return ""
		}
		inCheckout := func(dir string) bool { return checkoutOf(realOn(machineID, dir), top, worktrees) }
		for i := range snap.Panes {
			pane := &snap.Panes[i]
			if pane.Agent == "" || (pane.AgentStatus != herdr.StatusWorking && pane.AgentStatus != herdr.StatusBlocked) {
				continue
			}
			ws := snap.Workspace(pane.WorkspaceID)
			if ws == nil || !inCheckout(snap.WorkspaceDir(ws)) {
				continue
			}
			if pane.AgentStatus == herdr.StatusBlocked {
				return pane.AgentName() + " is waiting for your answer"
			}
			return pane.AgentName() + " is at work"
		}
		for _, dir := range snap.WorkspaceDirs() {
			if inCheckout(dir) && turns.Going(machineID, dir) {
				return "an agent's turn is still ending"
			}
		}
		return ""
	}}
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
