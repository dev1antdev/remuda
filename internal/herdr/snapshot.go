package herdr

// The questions remuda asks of a snapshot, beside PaneSize: a pane by id, a
// workspace's folder, the agent that works in a folder.

// Pane finds a pane by id, or nil.
func (s *Snapshot) Pane(paneID string) *PaneInfo {
	for i := range s.Panes {
		if s.Panes[i].PaneID == paneID {
			return &s.Panes[i]
		}
	}
	return nil
}

// Workspace finds a workspace by id, or nil.
func (s *Snapshot) Workspace(workspaceID string) *WorkspaceInfo {
	for i := range s.Workspaces {
		if s.Workspaces[i].WorkspaceID == workspaceID {
			return &s.Workspaces[i]
		}
	}
	return nil
}

// WorkspaceLabel is the label of the workspace with that id, "" for none.
func (s *Snapshot) WorkspaceLabel(workspaceID string) string {
	for i := range s.Workspaces {
		if s.Workspaces[i].WorkspaceID == workspaceID {
			return s.Workspaces[i].Label
		}
	}
	return ""
}

// WorkspaceDir is the folder a workspace lives in: its git checkout when herdr knows one, else the cwd of its first pane.
func (s *Snapshot) WorkspaceDir(ws *WorkspaceInfo) string {
	if ws.Worktree != nil && ws.Worktree.CheckoutPath != "" {
		return ws.Worktree.CheckoutPath
	}
	for i := range s.Panes {
		if s.Panes[i].WorkspaceID == ws.WorkspaceID {
			if dir := s.Panes[i].Dir(); dir != "" {
				return dir
			}
		}
	}
	return ""
}

// WorkspaceDirs lists the folders of the snapshot's workspaces, each once.
func (s *Snapshot) WorkspaceDirs() []string {
	var out []string
	seen := map[string]bool{}
	for i := range s.Workspaces {
		if dir := s.WorkspaceDir(&s.Workspaces[i]); dir != "" && !seen[dir] {
			seen[dir] = true
			out = append(out, dir)
		}
	}
	return out
}

// AgentPaneIn is the first agent pane of the first workspace in a folder, or nil.
func (s *Snapshot) AgentPaneIn(dir string) *PaneInfo {
	for i := range s.Workspaces {
		ws := &s.Workspaces[i]
		if s.WorkspaceDir(ws) != dir {
			continue
		}
		for j := range s.Panes {
			if pane := &s.Panes[j]; pane.WorkspaceID == ws.WorkspaceID && pane.Agent != "" {
				return pane
			}
		}
	}
	return nil
}
