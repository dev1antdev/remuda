package core

import (
	"encoding/json"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// DocName is the document remuda keeps a machine's workspaces and tabs in: the core holds terminals only.
const DocName = "remuda"

// The labels that place a terminal: its workspace and its tab, by the document's ids.
const (
	LabelSpace = "remuda.space"
	LabelTab   = "remuda.tab"
)

// Document is the `remuda` document: the workspaces in their order, each with its tabs in theirs.
type Document struct {
	Workspaces []DocWorkspace `json:"workspaces"`
}

// DocWorkspace is one workspace: its folder, the tab in front, its tabs and the checkout it is.
type DocWorkspace struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"`
	Folder    string   `json:"folder"`
	ActiveTab string   `json:"active_tab,omitempty"`
	Tabs      []DocTab `json:"tabs"`
	// Worktree is the git checkout the folder is, as read once when the workspace was made; nil outside git.
	Worktree *herdr.WorktreeInfo `json:"worktree,omitempty"`
}

// DocTab is one tab and its pane tree, herdr's layout shape with a terminal's address as each pane's id.
type DocTab struct {
	ID     string            `json:"id"`
	Label  string            `json:"label"`
	Layout *herdr.LayoutNode `json:"layout,omitempty"`
}

// parseDocument reads the document's value; nothing written yet, or a value that is no document, is an empty one.
func parseDocument(value json.RawMessage) (Document, error) {
	var doc Document
	if len(value) == 0 || string(value) == "null" {
		return doc, nil
	}
	err := json.Unmarshal(value, &doc)
	return doc, err
}
