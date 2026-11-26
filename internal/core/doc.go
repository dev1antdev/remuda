package core

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/dev1antdev/remuda/internal/coreapi"
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
	rest       kept
}

// DocWorkspace is one workspace: its folder, the tab in front, its tabs and the checkout it is.
type DocWorkspace struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"`
	Folder    string   `json:"folder"`
	ActiveTab string   `json:"active_tab,omitempty"`
	Tabs      []DocTab `json:"tabs"`
	// Worktree is the git checkout the folder is, as read once when the workspace was made; nil outside git.
	Worktree *coreapi.WorktreeInfo `json:"worktree,omitempty"`
	rest     kept
}

// DocTab is one tab and its pane tree, herdr's layout shape with a terminal's address as each pane's id.
type DocTab struct {
	ID     string              `json:"id"`
	Label  string              `json:"label"`
	Layout *coreapi.LayoutNode `json:"layout,omitempty"`
	rest   kept
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

// kept is what an object of the document holds that this build does not know — a newer remuda's on the same core —
// written back as it was read, so a rewrite by this remuda drops none of it.
type kept map[string]json.RawMessage

// keep reads b into known, a pointer to the struct whose tags name the fields this build knows, and answers the rest.
func keep(b []byte, known any) (kept, error) {
	if err := json.Unmarshal(b, known); err != nil {
		return nil, err
	}
	var all kept
	if err := json.Unmarshal(b, &all); err != nil {
		return nil, err
	}
	t := reflect.TypeOf(known).Elem()
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		delete(all, name)
	}
	if len(all) == 0 {
		return nil, nil
	}
	return all, nil
}

// put writes known with the fields kept beside it.
func (k kept) put(known any) ([]byte, error) {
	b, err := json.Marshal(known)
	if err != nil || len(k) == 0 {
		return b, err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(b, &all); err != nil {
		return nil, err
	}
	for name, raw := range k {
		if _, ok := all[name]; !ok {
			all[name] = raw
		}
	}
	return json.Marshal(all)
}

func (d *Document) UnmarshalJSON(b []byte) (err error) {
	type known Document
	d.rest, err = keep(b, (*known)(d))
	return err
}

func (d Document) MarshalJSON() ([]byte, error) {
	type known Document
	return d.rest.put(known(d))
}

func (w *DocWorkspace) UnmarshalJSON(b []byte) (err error) {
	type known DocWorkspace
	w.rest, err = keep(b, (*known)(w))
	return err
}

func (w DocWorkspace) MarshalJSON() ([]byte, error) {
	type known DocWorkspace
	return w.rest.put(known(w))
}

func (t *DocTab) UnmarshalJSON(b []byte) (err error) {
	type known DocTab
	t.rest, err = keep(b, (*known)(t))
	return err
}

func (t DocTab) MarshalJSON() ([]byte, error) {
	type known DocTab
	return t.rest.put(known(t))
}
