package herdr

import "strings"

// The machine as remuda reads it: the snapshot, a pane, a workspace, a tab, the layout, the statuses. herdr's model,
// which remuda-core's adapter fills (internal/core) — the other half of the package's name: the dictionary of a machine.

// herdr's agent statuses, and the coarser vocabulary the UI shows.
const (
	StatusWorking = "working"
	StatusBlocked = "blocked"
	StatusDone    = "done"
	StatusIdle    = "idle"
	StatusUnknown = "unknown"

	UIRun   = "run"
	UIInput = "input"
	UIDone  = "done"
	UIIdle  = "idle"
	UIShell = "shell"
)

// UIStatus maps a herdr status to the UI's; no agent at all is "shell".
func UIStatus(status string, hasAgent bool) string {
	switch status {
	case StatusWorking:
		return UIRun
	case StatusBlocked:
		return UIInput
	case StatusDone:
		return UIDone
	case StatusIdle:
		return UIIdle
	}
	if !hasAgent {
		return UIShell
	}
	return UIIdle
}

type AgentSession struct {
	Agent string `json:"agent"`
	Kind  string `json:"kind"` // "id" | "path"
	Value string `json:"value"`
}

type PaneInfo struct {
	PaneID        string        `json:"pane_id"`
	TabID         string        `json:"tab_id"`
	WorkspaceID   string        `json:"workspace_id"`
	TerminalID    string        `json:"terminal_id"`
	Agent         string        `json:"agent"`
	DisplayAgent  string        `json:"display_agent"`
	AgentStatus   string        `json:"agent_status"` // blocked|done|idle|unknown|working
	AgentSession  *AgentSession `json:"agent_session"`
	CWD           string        `json:"cwd"`
	ForegroundCWD string        `json:"foreground_cwd"`
	Label         string        `json:"label"`
	Title         string        `json:"title"`
	TitleStripped string        `json:"terminal_title_stripped"`
	Focused       bool          `json:"focused"`
	Scroll        Scroll        `json:"scroll"` // where the viewport stands; zero rows when herdr said nothing
	// the display values clients reported for the pane (pane.report_metadata), every source's, the newest per name
	Tokens      map[string]string `json:"tokens,omitempty"`
	StateLabels map[string]string `json:"state_labels,omitempty"`
	// Restored is remuda-core's alone: the pane's shell was lost and the core started a new one in its place, so
	// whatever ran there did not leave at the person's word.
	Restored bool `json:"-"`
}

// Scroll is where a pane's viewport stands in its scrollback: rows above the bottom, the most it can be, and the
// viewport's height — herdr draws the pane, so the viewport is one for every viewer.
type Scroll struct {
	Offset int `json:"offset_from_bottom"`
	Max    int `json:"max_offset_from_bottom"`
	Rows   int `json:"viewport_rows"`
}

// Dir is the pane's folder: the shell's cwd, else the foreground process's.
func (p *PaneInfo) Dir() string {
	if p.CWD != "" {
		return p.CWD
	}
	return p.ForegroundCWD
}

// AgentName is what to call the agent in the pane; "" for a plain shell.
func (p *PaneInfo) AgentName() string {
	if p.DisplayAgent != "" {
		return p.DisplayAgent
	}
	return p.Agent
}

// DisplayTitle is the user's label, else the terminal title, else the agent.
func (p *PaneInfo) DisplayTitle() string {
	for _, t := range []string{p.Label, p.TitleStripped, p.Title, p.AgentName()} {
		if strings.TrimSpace(t) != "" {
			return t
		}
	}
	return ""
}

type TabInfo struct {
	TabID       string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
	Number      int    `json:"number"`
	Label       string `json:"label"`
	AgentStatus string `json:"agent_status"`
}

type WorktreeInfo struct {
	CheckoutPath     string `json:"checkout_path"`
	IsLinkedWorktree bool   `json:"is_linked_worktree"`
	RepoKey          string `json:"repo_key"`
	RepoName         string `json:"repo_name"`
	RepoRoot         string `json:"repo_root"` // the repository's main checkout, a linked worktree's too
}

type WorkspaceInfo struct {
	WorkspaceID string            `json:"workspace_id"`
	Label       string            `json:"label"`
	AgentStatus string            `json:"agent_status"`
	Focused     bool              `json:"focused"`
	ActiveTabID string            `json:"active_tab_id"`
	Worktree    *WorktreeInfo     `json:"worktree"`
	Folder      string            `json:"-"`                // the folder remuda placed it on, where herdr names none (remuda-core's)
	Tokens      map[string]string `json:"tokens,omitempty"` // workspace.report_metadata's values, every source's
}

type Rect struct {
	Width  int `json:"width"` // cells
	Height int `json:"height"`
}

type LayoutPane struct {
	PaneID string `json:"pane_id"`
	Rect   Rect   `json:"rect"`
}

type Layout struct {
	Panes []LayoutPane `json:"panes"`
}

type Snapshot struct {
	Version       string          `json:"version"`
	FocusedPaneID string          `json:"focused_pane_id"`
	FocusedTabID  string          `json:"focused_tab_id"`
	Workspaces    []WorkspaceInfo `json:"workspaces"`
	Tabs          []TabInfo       `json:"tabs"`
	Panes         []PaneInfo      `json:"panes"`
	Layouts       []Layout        `json:"layouts"`
}

// PaneSize returns a pane's grid size in cells from the layouts, or 0,0.
func (s *Snapshot) PaneSize(paneID string) (cols, rows int) {
	for _, l := range s.Layouts {
		for _, p := range l.Panes {
			if p.PaneID == paneID {
				return p.Rect.Width, p.Rect.Height
			}
		}
	}
	return 0, 0
}

// Screen is a pane's visible screen as herdr renders it: the rows as text with their SGR colours.
type Screen struct {
	Text      string
	Truncated bool
}

// AgentInfo is an agent as herdr reports it: its kind and its status.
type AgentInfo struct {
	Agent  string `json:"agent"`
	Status string `json:"agent_status"`
}

// WorkspaceCreateResult is a workspace made, with the pane it opened on.
type WorkspaceCreateResult struct {
	Workspace WorkspaceInfo `json:"workspace"`
	RootPane  PaneInfo      `json:"root_pane"`
}

// TabCreateResult is a tab made, with the pane it opened on.
type TabCreateResult struct {
	Tab      TabInfo  `json:"tab"`
	RootPane PaneInfo `json:"root_pane"`
}

// WorktreeCreateResult is a worktree opened as a workspace: the checkout, and whether it already stood open, its panes
// the person's.
type WorktreeCreateResult struct {
	Workspace WorkspaceInfo `json:"workspace"`
	RootPane  PaneInfo      `json:"root_pane"`
	Worktree  struct {
		Path   string `json:"path"`
		Branch string `json:"branch"`
	} `json:"worktree"`
	AlreadyOpen bool `json:"already_open"`
}
