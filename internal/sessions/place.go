package sessions

import (
	"fmt"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/watch"
)

// Placer is what making a place for an agent needs of herdr's client: a tab in a workspace, or a workspace on a folder.
type Placer interface {
	Starter
	WorkspaceCreate(cwd, label string) (*herdr.WorkspaceCreateResult, error)
	TabCreate(workspaceID, label string) (*herdr.TabCreateResult, error)
}

// Place is where an agent was started: its pane, its tab, its workspace, and whether the workspace was made for it.
type Place struct {
	Pane          string `json:"pane"`
	Tab           string `json:"tab"`
	Workspace     string `json:"workspace"`
	MadeWorkspace bool   `json:"made_workspace"`
}

// StartIn starts an agent in a folder: a new tab of the folder's workspace when the snapshot has one, else a new
// workspace on the folder; the place comes back with the agent's refusal when the tab is there but the agent is not.
func StartIn(client Placer, snap *herdr.Snapshot, dir, label, kind string, start Start) (Place, error) {
	if kind == "" || kind == "shell" || !KnownAgent(kind) {
		return Place{}, fmt.Errorf("an agent is required, not %q", kind)
	}
	var place Place
	if workspaceID := workspaceIn(snap, dir); workspaceID != "" {
		res, err := client.TabCreate(workspaceID, label)
		if err != nil {
			return Place{}, err
		}
		place = Place{Pane: res.RootPane.PaneID, Tab: res.Tab.TabID, Workspace: workspaceID}
	} else {
		res, err := client.WorkspaceCreate(dir, "")
		if err != nil {
			return Place{}, err
		}
		place = Place{Pane: res.RootPane.PaneID, Tab: res.RootPane.TabID, Workspace: res.Workspace.WorkspaceID, MadeWorkspace: true}
	}
	if err := StartAgentWith(client, place.Pane, kind, start); err != nil {
		return place, fmt.Errorf("the tab is there, but the agent did not start: %w", err)
	}
	return place, nil
}

// workspaceIn is the id of the first workspace whose folder is dir, "" for none.
func workspaceIn(snap *herdr.Snapshot, dir string) string {
	if snap == nil {
		return ""
	}
	for i := range snap.Workspaces {
		if snap.WorkspaceDir(&snap.Workspaces[i]) == dir {
			return snap.Workspaces[i].WorkspaceID
		}
	}
	return ""
}

// Launcher starts an agent in a folder of a machine and hands it a first prompt in the background: the schedules' door.
type Launcher struct {
	Machines *machines.Manager
	Watch    *watch.Watcher
	Input    *Input
	Notify   func(push.Payload)
}

// StartInFolder makes the place and starts the agent on the machine, then delivers the prompt once the agent is up;
// an undelivered prompt is a note to the person with the pane to open.
func (l *Launcher) StartInFolder(machineID, dir, label, kind string, start Start, prompt string) (Place, error) {
	mc, err := l.Machines.Get(machineID)
	if err != nil {
		return Place{}, err
	}
	client, _, err := mc.Ready()
	if err != nil {
		return Place{}, err
	}
	snap, _ := l.Watch.Snapshot(machineID)
	place, err := StartIn(client, snap, dir, label, kind, start)
	l.Watch.Refresh(mc)
	if err != nil {
		return place, err
	}
	if prompt != "" {
		go l.firstPrompt(client, events.PaneRef{Machine: machineID, Pane: place.Pane}, prompt, label)
	}
	return place, nil
}

func (l *Launcher) firstPrompt(client Prompter, ref events.PaneRef, prompt, label string) {
	err := l.Input.FirstPrompt(client, ref, prompt, FirstPromptTimeout)
	if err == nil || l.Notify == nil {
		return
	}
	l.Notify(push.Payload{
		Machine: ref.Machine, Pane: ref.Pane, Kind: "schedule",
		Title:   "Schedule “" + label + "”: the prompt was not delivered",
		Body:    err.Error(),
		Tag:     "first|" + ref.Pane,
		Actions: []push.Action{{Action: "open", Title: "Open"}},
	})
}

// Close takes the place down through herdr: the workspace when it was made for the agent, else the tab alone.
func (l *Launcher) Close(machineID string, place Place) error {
	mc, err := l.Machines.Get(machineID)
	if err != nil {
		return err
	}
	client, _, err := mc.Ready()
	if err != nil {
		return err
	}
	if place.MadeWorkspace {
		err = client.WorkspaceClose(place.Workspace)
	} else {
		err = client.TabClose(place.Tab)
	}
	l.Watch.Refresh(mc)
	return err
}
