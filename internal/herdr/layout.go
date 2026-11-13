package herdr

// LayoutNode is a tab's pane tree as herdr's layout.export answers it and layout.apply takes it: a pane, or a split of
// two nodes. herdr starts a pane with no command as a shell in its folder.
type LayoutNode struct {
	Type      string            `json:"type"` // pane | split
	PaneID    string            `json:"pane_id,omitempty"`
	CWD       string            `json:"cwd,omitempty"`
	Label     string            `json:"label,omitempty"`
	Command   []string          `json:"command,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Direction string            `json:"direction,omitempty"` // right | down
	Ratio     float64           `json:"ratio,omitempty"`     // the first node's share of the split
	First     *LayoutNode       `json:"first,omitempty"`
	Second    *LayoutNode       `json:"second,omitempty"`
}

// Panes are the tree's pane nodes, left to right and top to bottom.
func (n *LayoutNode) Panes() []*LayoutNode {
	if n == nil {
		return nil
	}
	if n.Type != "split" {
		return []*LayoutNode{n}
	}
	return append(n.First.Panes(), n.Second.Panes()...)
}

// LayoutDescription is one tab's layout: where it lives, its focused pane and its tree.
type LayoutDescription struct {
	WorkspaceID   string     `json:"workspace_id"`
	TabID         string     `json:"tab_id"`
	FocusedPaneID string     `json:"focused_pane_id"`
	Root          LayoutNode `json:"root"`
}

// LayoutExport is a tab's layout: its splits with their ratios and every pane's folder, label and command.
func (c *Client) LayoutExport(tabID string) (*LayoutDescription, error) {
	var out struct {
		Layout LayoutDescription `json:"layout"`
	}
	if err := c.Call("layout.export", map[string]any{"tab_id": tabID}, &out); err != nil {
		return nil, err
	}
	return &out.Layout, nil
}

// LayoutApply lays a tree out as a tab: a new tab of the workspace, or, with a tab id, in place of that tab — whose
// panes, and whatever ran in them, go — under the label given ("" keeps herdr's own). The focus stays where it is.
func (c *Client) LayoutApply(workspaceID, tabID, tabLabel string, root LayoutNode) (*LayoutDescription, error) {
	params := map[string]any{"root": root, "focus": false}
	if workspaceID != "" {
		params["workspace_id"] = workspaceID
	}
	if tabID != "" {
		params["tab_id"] = tabID
	}
	if tabLabel != "" {
		params["tab_label"] = tabLabel
	}
	var out struct {
		Layout LayoutDescription `json:"layout"`
	}
	if err := c.Call("layout.apply", params, &out); err != nil {
		return nil, err
	}
	return &out.Layout, nil
}

// PaneClose closes a pane and what runs in it; herdr closes its tab with its last pane, and the workspace with its last tab.
func (c *Client) PaneClose(paneID string) error {
	return c.Call("pane.close", map[string]any{"pane_id": paneID}, nil)
}

// PaneAtPrompt says whether the pane's shell is at its prompt: the shell's own process group holds the terminal, so
// nothing the person started runs in the foreground. A pane herdr cannot say that of is not at its prompt.
func (c *Client) PaneAtPrompt(paneID string) (bool, error) {
	info, err := c.processInfo(paneID)
	if err != nil {
		return false, err
	}
	shell, foreground := info.ShellPID, info.Foreground
	return shell != nil && foreground != nil && *shell == *foreground, nil
}

// PaneShellPID is the pid of the shell the pane's terminal runs — the root of every process started in the pane — 0
// when herdr names none.
func (c *Client) PaneShellPID(paneID string) (int, error) {
	info, err := c.processInfo(paneID)
	if err != nil || info.ShellPID == nil {
		return 0, err
	}
	return *info.ShellPID, nil
}

// processInfo is herdr's pane.process_info: the pane's shell and the process group holding its terminal
type processInfo struct {
	ShellPID   *int `json:"shell_pid"`
	Foreground *int `json:"foreground_process_group_id"`
}

func (c *Client) processInfo(paneID string) (processInfo, error) {
	var out struct {
		Info processInfo `json:"process_info"`
	}
	err := c.Call("pane.process_info", map[string]any{"pane_id": paneID}, &out)
	return out.Info, err
}
