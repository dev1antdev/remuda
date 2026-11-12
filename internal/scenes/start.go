package scenes

import (
	"errors"
	"fmt"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/recipes"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/worktrees"
)

// Herdr is what a scene asks of one machine's herdr.
type Herdr interface {
	sessions.Placer
	sessions.Prompter
	TabRename(tabID, label string) error
	LayoutExport(tabID string) (*herdr.LayoutDescription, error)
	LayoutApply(workspaceID, tabID, tabLabel string, root herdr.LayoutNode) (*herdr.LayoutDescription, error)
	PaneClose(paneID string) error
	PaneAtPrompt(paneID string) (bool, error)
}

// Machine is what a scene needs of one machine of the fleet.
type Machine interface {
	ID() string
	Label() string
	Herdr() (Herdr, error)     // the machine's herdr, or why it cannot be reached now
	Snapshot() *herdr.Snapshot // the watcher's last, nil before the first
	Refresh()                  // the watcher reads the machine again
	Leftovers(open []string) ([]worktrees.Leftover, error)
	Tidy(folder string) worktrees.Outcome
	Folder(dir string) bool // whether the folder is there: herdr opens a missing one in the home folder instead
}

// launch is what a step starts once its recipe is read: the agent's kind ("shell" for none), its choices, its prompt.
type launch struct {
	kind   string
	start  sessions.Start
	prompt string
}

// launchOf is a step's agent, model, arguments and prompt: its own, or those of the recipe it names as the cookbook
// holds it now.
func launchOf(step Step, recipe func(id string) (recipes.Recipe, bool)) (launch, error) {
	agent, model, args, prompt := step.Agent, step.Model, step.Args, step.Prompt
	if step.Recipe != "" {
		r, ok := recipe(step.Recipe)
		if !ok {
			return launch{}, errors.New("the recipe it names is gone")
		}
		agent, model, args, prompt = r.Agent, r.Model, r.Args, r.Prompt
	}
	if err := recipes.CheckStart(agent, model, args, prompt); err != nil {
		return launch{}, err
	}
	if agent == "" || agent == "shell" {
		return launch{kind: "shell"}, nil
	}
	split, err := sessions.SplitArgs(args)
	if err != nil {
		return launch{}, err
	}
	return launch{kind: agent, start: sessions.Start{Model: model, Args: split}, prompt: prompt}, nil
}

// placeStep makes the step's tab: a new tab of the workspace open on its folder, else a new workspace on the folder;
// laid out as the step's layout when it has one, named as the step says. The place's pane is the one its agent goes in.
func placeStep(client Herdr, snap *herdr.Snapshot, step Step) (sessions.Place, error) {
	workspace := sessions.WorkspaceIn(snap, step.Dir)
	if step.Layout == nil {
		if workspace != "" {
			res, err := client.TabCreate(workspace, step.Tab)
			if err != nil {
				return sessions.Place{}, err
			}
			return sessions.Place{Pane: res.RootPane.PaneID, Tab: res.Tab.TabID, Workspace: workspace}, nil
		}
		res, err := client.WorkspaceCreate(step.Dir, "")
		if err != nil {
			return sessions.Place{}, err
		}
		place := sessions.Place{Pane: res.RootPane.PaneID, Tab: res.RootPane.TabID, Workspace: res.Workspace.WorkspaceID, MadeWorkspace: true}
		if step.Tab != "" {
			if err := client.TabRename(place.Tab, step.Tab); err != nil {
				return place, fmt.Errorf("the workspace is there, but its tab kept herdr's name: %w", err)
			}
		}
		return place, nil
	}
	tree := step.Layout.Tree(step.Dir)
	if workspace != "" {
		laid, err := client.LayoutApply(workspace, "", step.Tab, tree)
		if err != nil {
			return sessions.Place{}, err
		}
		return sessions.Place{Pane: step.Layout.AgentPane(&laid.Root), Tab: laid.TabID, Workspace: workspace}, nil
	}
	res, err := client.WorkspaceCreate(step.Dir, "")
	if err != nil {
		return sessions.Place{}, err
	}
	place := sessions.Place{Pane: res.RootPane.PaneID, Tab: res.RootPane.TabID, Workspace: res.Workspace.WorkspaceID, MadeWorkspace: true}
	// the new workspace's first tab, a shell just made, is laid out in place
	laid, err := client.LayoutApply("", place.Tab, step.Tab, tree)
	if err != nil {
		return place, fmt.Errorf("the workspace is there, but not its layout: %w", err)
	}
	place.Pane, place.Tab = step.Layout.AgentPane(&laid.Root), laid.TabID
	return place, nil
}

// runStep is one step of a start scene: its machine reached, its place made, its agent started and its prompt handed
// over in the background; the item says how it went.
func (s *Service) runStep(sceneName string, step Step, item *Item) {
	mc, err := s.Machine(step.Machine)
	if err != nil {
		item.State, item.Note = StateAway, err.Error()
		return
	}
	client, err := mc.Herdr()
	if err != nil {
		item.State, item.Note = StateAway, mc.Label()+": "+err.Error()
		return
	}
	what, err := launchOf(step, s.Recipe)
	if err != nil {
		item.State, item.Note = StateRefused, err.Error()
		return
	}
	if !mc.Folder(step.Dir) {
		item.State, item.Note = StateRefused, "the folder "+step.Dir+" is not there on "+mc.Label()
		return
	}
	place, err := placeStep(client, mc.Snapshot(), step)
	defer mc.Refresh()
	item.Pane = place.Pane
	if err != nil {
		item.State, item.Note = StateRefused, err.Error()
		return
	}
	if what.kind != "shell" {
		if err := sessions.StartAgentWith(client, place.Pane, what.kind, what.start); err != nil {
			item.State, item.Note = StateRefused, "the tab is there, but the agent did not start: "+err.Error()
			return
		}
		if what.prompt != "" {
			go s.firstPrompt(client, mc.ID(), place.Pane, what.prompt, sceneName)
		}
	}
	item.State, item.MadeWorkspace = StateStarted, place.MadeWorkspace
}
