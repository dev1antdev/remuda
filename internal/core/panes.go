package core

import (
	"cmp"
	"context"
	"fmt"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// PaneSplit opens a shell beside the pane, "right" or "down", in the folder given (the pane's when none), in the
// pane's tab.
func (a *Adapter) PaneSplit(paneID, direction, cwd string) (*herdr.PaneInfo, error) {
	if direction != "right" && direction != "down" {
		return nil, &herdr.Error{Code: "invalid_request", Message: fmt.Sprintf("a split is right or down, not %q", direction)}
	}
	snap, err := a.SessionSnapshot()
	if err != nil {
		return nil, err
	}
	pane := snap.Pane(paneID)
	if pane == nil {
		return nil, &herdr.Error{Code: "pane_not_found", Message: "no pane " + paneID}
	}
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	t, err := a.runShell(ctx, pane.WorkspaceID, pane.TabID, cmp.Or(cwd, pane.CWD), nil)
	if err != nil {
		return nil, err
	}
	err = a.editDoc(ctx, func(doc *Document) error {
		ws, ok := adopt(doc, snap, pane.WorkspaceID)
		if !ok {
			return &herdr.Error{Code: "pane_not_found", Message: "no pane " + paneID}
		}
		tab := adoptTab(ws, snap, pane.TabID)
		tab.Layout = splitBeside(tab.Layout, paneID, t.Address, direction)
		return nil
	})
	if err != nil {
		a.kill(t.Address)
		return nil, err
	}
	made := paneOf(&t, pane.TabID, pane.WorkspaceID, nil)
	return &made, nil
}

// splitBeside puts a new pane beside one of a tree's, half the room each; a tree that lacks it takes the new pane beside
// the whole of it.
func splitBeside(tree *herdr.LayoutNode, beside, added, direction string) *herdr.LayoutNode {
	if tree == nil {
		tree = leaf(beside)
	}
	for _, node := range tree.Panes() {
		if node.PaneID == beside {
			old := *node
			*node = herdr.LayoutNode{Type: "split", Direction: direction, Ratio: 0.5, First: &old, Second: leaf(added)}
			return tree
		}
	}
	return &herdr.LayoutNode{Type: "split", Direction: direction, Ratio: 0.5, First: tree, Second: leaf(added)}
}

// dropPane takes a pane out of its tab's tree in the document, saying whether the tree still places another pane.
func dropPane(doc *Document, tab, pane string) (placesOthers bool) {
	for i := range doc.Workspaces {
		for j := range doc.Workspaces[i].Tabs {
			if t := &doc.Workspaces[i].Tabs[j]; t.ID == tab {
				t.Layout = withoutPane(t.Layout, pane)
				placesOthers = t.Layout != nil
			}
		}
	}
	return placesOthers
}

// withoutPane is a tree with a pane taken out, its split given whole to the other side; nil once no pane is left.
func withoutPane(tree *herdr.LayoutNode, pane string) *herdr.LayoutNode {
	switch {
	case tree == nil:
		return nil
	case tree.Type != "split":
		if tree.PaneID == pane {
			return nil
		}
		return tree
	}
	first, second := withoutPane(tree.First, pane), withoutPane(tree.Second, pane)
	switch {
	case first == nil:
		return second
	case second == nil:
		return first
	}
	tree.First, tree.Second = first, second
	return tree
}

// PaneClose ends the pane's terminal and takes it out of its tab's tree; the tab goes with its last pane and the
// workspace with its last tab, as herdr's do. A tab the document still places another pane in — a split another
// request wrote meanwhile — stays.
func (a *Adapter) PaneClose(paneID string) error {
	snap, err := a.SessionSnapshot()
	if err != nil {
		return err
	}
	pane := snap.Pane(paneID)
	if pane == nil {
		return &herdr.Error{Code: "pane_not_found", Message: "no pane " + paneID}
	}
	if err := a.kill(paneID); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	ending := []string{paneID}
	return a.editDoc(ctx, func(doc *Document) error {
		if !dropPane(doc, pane.TabID, paneID) && !a.holds(func(p *herdr.PaneInfo) bool { return p.TabID == pane.TabID }, ending) {
			a.dropTab(doc, pane.TabID, ending)
		}
		return nil
	})
}
