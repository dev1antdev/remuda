package server

import (
	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/worktrees"
)

// A new worktree's setup script, run in the background once the create has answered: in the worktree's own terminal
// before its agent, or in a Setup tab beside it; and the workspace a branch checked out already stands on.

// setupThenAgent types the setup into the worktree's own terminal and, once it has ended, starts the agent there.
func (s *Server) setupThenAgent(mc *machines.Machine, client *herdr.Client, ref events.PaneRef, setup, root, path, branch, agent string, start sessions.Start, prompt string) {
	line, err := worktrees.WriteSetup(mc, setup, root, path)
	if err == nil {
		err = s.Input.TypeLine(client, ref, line)
	}
	if err != nil {
		s.setupNote(ref, branch, "The setup of "+branch+" did not start", err)
		return
	}
	if agent == "" || agent == "shell" {
		return
	}
	if _, err := worktrees.AwaitSetup(client, ref.Pane); err != nil {
		s.setupNote(ref, branch, "The agent in "+branch+" was not started", err)
		return
	}
	if err := sessions.StartAgentWith(client, ref.Pane, agent, start); err != nil {
		s.setupNote(ref, branch, "The agent in "+branch+" did not start after its setup", err)
		return
	}
	if prompt != "" {
		_ = s.firstPrompt(client, ref, prompt, branch)
	}
}

// setupInTab runs the setup in a Setup tab of the new workspace, the agent's tab staying in front.
func (s *Server) setupInTab(mc *machines.Machine, client *herdr.Client, workspace, setup, root, path, branch string) {
	tab, err := client.TabCreateIn(workspace, "Setup", path, nil)
	if err != nil {
		s.setupNote(events.PaneRef{Machine: mc.ID()}, branch, "The setup of "+branch+" did not start", err)
		return
	}
	ref := events.PaneRef{Machine: mc.ID(), Pane: tab.RootPane.PaneID}
	line, err := worktrees.WriteSetup(mc, setup, root, path)
	if err == nil {
		err = s.Input.TypeLine(client, ref, line)
	}
	if err != nil {
		s.setupNote(ref, branch, "The setup of "+branch+" did not start", err)
	}
	s.Watch.Refresh(mc)
}

// setupNote tells the person, in the activity and a push, what of a new worktree's setup did not happen.
func (s *Server) setupNote(ref events.PaneRef, branch, title string, err error) {
	if s.Push == nil {
		return
	}
	s.Push.Notify(push.Payload{
		Machine: ref.Machine, Pane: ref.Pane, Kind: "input",
		Title: title, Body: err.Error(), Tag: "setup|" + branch,
		Actions: []push.Action{{Action: "open", Title: "Open"}},
	})
}

// standingOn is the workspace whose checkout is the folder, nil when none has it open.
func standingOn(s *Server, mc *machines.Machine, folder string) *herdr.WorkspaceInfo {
	snap, _ := s.Watch.Snapshot(mc.ID())
	if snap == nil {
		return nil
	}
	style := rpath.For(folder)
	for i := range snap.Workspaces {
		ws := &snap.Workspaces[i]
		if dir := snap.WorkspaceDir(ws); dir != "" && style.Equal(dir, folder) {
			return ws
		}
	}
	return nil
}

// firstPaneOf is the pane a workspace shows first: its active tab's focused one, else its first.
func firstPaneOf(s *Server, mc *machines.Machine, workspaceID string) string {
	snap, _ := s.Watch.Snapshot(mc.ID())
	if snap == nil {
		return ""
	}
	ws := snap.Workspace(workspaceID)
	first := ""
	for i := range snap.Panes {
		p := &snap.Panes[i]
		if p.WorkspaceID != workspaceID {
			continue
		}
		if ws != nil && p.TabID == ws.ActiveTabID && p.Focused {
			return p.PaneID
		}
		if first == "" {
			first = p.PaneID
		}
	}
	return first
}
