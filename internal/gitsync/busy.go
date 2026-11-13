package gitsync

import (
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/rpath"
)

// What an agent in a checkout is doing that holds a pull back.
const (
	DoingWork   = "working"
	DoingAsk    = "asking"
	DoingEnding = "ending" // its turn has stopped but is not read to its end yet
)

// AgentIn is the agent of the snapshot at work or asking in the checkout, and which of the two; "" for none. An agent
// counts when herdr names its workspace's checkout and that is this one, or, where herdr names none, when its folder
// lies in it: a linked worktree is a checkout of its own, so an agent in one does not hold its repository's main
// folder back. going says a turn is still ending in a workspace's folder.
func AgentIn(snap *herdr.Snapshot, style rpath.Style, checkout string, going func(dir string) bool) (agent, doing string) {
	if snap == nil {
		return "", ""
	}
	inCheckout := func(ws *herdr.WorkspaceInfo, dir string) bool {
		if ws != nil && ws.Worktree != nil && ws.Worktree.CheckoutPath != "" {
			return style.Equal(ws.Worktree.CheckoutPath, checkout)
		}
		return dir != "" && style.Within(checkout, dir)
	}
	for i := range snap.Panes {
		pane := &snap.Panes[i]
		if pane.Agent == "" || (pane.AgentStatus != herdr.StatusWorking && pane.AgentStatus != herdr.StatusBlocked) {
			continue
		}
		if inCheckout(snap.Workspace(pane.WorkspaceID), pane.Dir()) {
			if pane.AgentStatus == herdr.StatusBlocked {
				return pane.Agent, DoingAsk
			}
			return pane.Agent, DoingWork
		}
	}
	if going == nil {
		return "", ""
	}
	for i := range snap.Workspaces {
		ws := &snap.Workspaces[i]
		dir := snap.WorkspaceDir(ws)
		if dir == "" || !inCheckout(ws, dir) || !going(dir) {
			continue
		}
		if pane := snap.AgentPaneIn(dir); pane != nil {
			return pane.Agent, DoingEnding
		}
		return "agent", DoingEnding
	}
	return "", ""
}
