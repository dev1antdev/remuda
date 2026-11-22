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
