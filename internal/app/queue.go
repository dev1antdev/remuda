package app

import (
	"fmt"
	"path"
	"strings"

	"github.com/dev1antdev/remuda/internal/budget"
	"github.com/dev1antdev/remuda/internal/chat"
	"github.com/dev1antdev/remuda/internal/limits"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/radar"
	"github.com/dev1antdev/remuda/internal/tasks"
	"github.com/dev1antdev/remuda/internal/watch"
)

// taskQueue is the projects' queue with its view of the world: the machines, the workspace on a repository folder,
// the radar's reading of a pane, the agents' plan windows, the budgets, and the launcher a start goes through.
func taskQueue(store *tasks.Store, fleet *machines.Manager, watcher *watch.Watcher, launcher *tasks.Launcher, tracker *radar.Tracker,
	chats *chat.Service, planLimits *limits.Service, budgets *budget.Service, notify func(push.Payload)) *tasks.Queue {
	return &tasks.Queue{
		Store: store,
		Machine: func(id string) (string, bool) {
			mc, err := fleet.Get(id)
			if err != nil {
				return id, false
			}
			state, _ := mc.State()
			return mc.Label(), state == machines.StateConnected
		},
		Unusable: func(id string) string {
			if fleet.Server() && (id == machines.LocalID || id == "") {
				return "this remuda only serves the page, so its own machine runs no agents: pick another in the project's settings"
			}
			return ""
		},
		Workspace: func(machineID, dir string) string {
			snap, _ := watcher.Snapshot(machineID)
			if snap == nil {
				return ""
			}
			for i := range snap.Workspaces {
				ws := &snap.Workspaces[i]
				if ws.Worktree != nil && ws.Worktree.IsLinkedWorktree {
					continue
				}
				if path.Clean(snap.WorkspaceDir(ws)) == dir {
					return ws.WorkspaceID
				}
			}
			return ""
		},
		PaneState: func(machineID, paneID string) (string, bool) {
			pane, snap := chats.Pane(machineID, paneID)
			if snap == nil {
				return "unknown", true
			}
			if pane == nil {
				return "", false
			}
			return tracker.Display(machineID, pane).State, true
		},
		Spent: func(machineID, agent string) string {
			for _, w := range planLimits.Of(machineID).Windows {
				if w.Agent == agent && limits.Level(w.Used) >= 100 {
					return fmt.Sprintf("%s's %s window is spent until %s", agentName(agent), windowWords(w.Name), w.ResetsAt.Local().Format("Jan 2 15:04"))
				}
			}
			return ""
		},
		Crossed: func(machineID, dir string) string {
			for _, st := range budgets.Status() {
				b := st.Budget
				covers := (b.Machine == "" || b.Machine == machineID) && (b.Dir == "" || path.Clean(b.Dir) == dir)
				if covers && st.Collected != "" && b.Limit > 0 && st.Spent >= b.Limit {
					return fmt.Sprintf("the budget %s is spent: $%.2f of $%.2f this %s", budgetScope(b), st.Spent, b.Limit, b.Period)
				}
			}
			return ""
		},
		Start: func(machineID string, task tasks.Task, req tasks.StartRequest) error {
			mc, err := fleet.Get(machineID)
			if err != nil {
				return err
			}
			client, _, err := mc.Ready()
			if err != nil {
				return err
			}
			_, err = launcher.Start(mc, client, task, req)
			return err
		},
		Notify: notify,
	}
}

func agentName(kind string) string {
	switch kind {
	case "claude":
		return "Claude"
	case "codex":
		return "Codex"
	}
	return kind
}

func windowWords(name string) string {
	switch name {
	case "5h":
		return "5 h"
	case "week":
		return "weekly"
	}
	return name
}

func budgetScope(b budget.Budget) string {
	switch {
	case strings.TrimSpace(b.Note) != "":
		return "“" + strings.TrimSpace(b.Note) + "”"
	case b.Dir != "":
		return "for " + path.Base(b.Dir)
	case b.Machine != "":
		return "for " + b.Machine
	}
	return "for everything"
}
