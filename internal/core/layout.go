package core

import (
	"cmp"
	"context"
	"log"
	"maps"
	"slices"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/state"
)

// LayoutExport is a tab's pane tree as herdr's layout.export gives it: the document's splits and their shares, each
// pane's folder as it stands now, and the label and command the layout that made it gave it. A pane the tree lacks
// (one placed by its labels alone) goes to the right of the last; one whose terminal ended is left out.
func (a *Adapter) LayoutExport(tabID string) (*herdr.LayoutDescription, error) {
	snap, err := a.SessionSnapshot()
	if err != nil {
		return nil, err
	}
	tab := snap.Tab(tabID)
	if tab == nil {
		return nil, tabMissing(tabID)
	}
	panes := panesWhere(snap, func(p *herdr.PaneInfo) bool { return p.TabID == tabID })
	tree := a.docTree(tabID)
	for _, node := range tree.Panes() {
		if !slices.Contains(panes, node.PaneID) {
			tree = withoutPane(tree, node.PaneID)
		}
	}
	for _, pane := range panes {
		leaves := tree.Panes()
		switch {
		case tree == nil:
			tree = leaf(pane)
		case !slices.ContainsFunc(leaves, func(n *herdr.LayoutNode) bool { return n.PaneID == pane }):
			tree = splitBeside(tree, leaves[len(leaves)-1].PaneID, pane, "right")
		}
	}
	for _, node := range tree.Panes() {
		node.CWD = snap.Pane(node.PaneID).CWD
	}
	return &herdr.LayoutDescription{WorkspaceID: tab.WorkspaceID, TabID: tabID, FocusedPaneID: tree.Panes()[0].PaneID, Root: *tree}, nil
}

// LayoutApply lays a tree out as a tab, as herdr's layout.apply does: a new tab of the workspace, or, with a tab id, in
// place of that tab, whose panes and what ran in them go once the new ones stand. Each pane runs its command, else the
// machine's shell, in its folder (the workspace's when it names none); the document keeps the tree with each pane's
// terminal, label and command. The tab in front stays in front. The tree is taken as scenes checked it.
func (a *Adapter) LayoutApply(workspaceID, tabID, tabLabel string, root herdr.LayoutNode) (*herdr.LayoutDescription, error) {
	if len(root.Panes()) == 0 {
		return nil, &herdr.Error{Code: "invalid_request", Message: "a layout holds at least one pane"}
	}
	snap, err := a.SessionSnapshot()
	if err != nil {
		return nil, err
	}
	space, inPlace := workspaceID, tabID != ""
	var ending []string
	if inPlace {
		space = cmp.Or(spaceOfTab(snap, tabID), a.docSpaceOf(tabID))
		if space == "" {
			return nil, tabMissing(tabID)
		}
		ending = panesWhere(snap, func(p *herdr.PaneInfo) bool { return p.TabID == tabID })
	} else {
		if snap.Workspace(space) == nil && !a.inDoc(space) {
			return nil, spaceMissing(space)
		}
		tabID = "tab-" + state.NewID()
	}
	folder := ""
	if ws := snap.Workspace(space); ws != nil {
		folder = snap.WorkspaceDir(ws)
	}

	tree := clone(&root)
	var started []string
	for _, node := range tree.Panes() {
		ctx, cancel := context.WithTimeout(context.Background(), callWithin)
		t, err := a.runIn(ctx, space, tabID, RunParams{Argv: node.Command, Cwd: cmp.Or(node.CWD, folder), Env: node.Env})
		cancel()
		if err != nil {
			a.kill(started...)
			return nil, err
		}
		started = append(started, t.Address)
		node.PaneID, node.CWD, node.Env = t.Address, cmp.Or(node.CWD, t.Cwd), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	err = a.editDoc(ctx, func(doc *Document) error {
		ws, ok := adopt(doc, snap, space)
		if !ok {
			return spaceMissing(space)
		}
		if !inPlace {
			ws.Tabs = append(ws.Tabs, DocTab{ID: tabID, Label: tabLabel, Layout: clone(tree)})
			return nil
		}
		laid := adoptTab(ws, snap, tabID)
		laid.Layout = clone(tree)
		if tabLabel != "" {
			laid.Label = tabLabel
		}
		return nil
	})
	if err != nil {
		a.kill(started...)
		return nil, err
	}
	if err := a.kill(ending...); err != nil {
		log.Printf("remuda-core: tab %s is laid out anew, but not all its old panes ended: %v", tabID, err)
	}
	return &herdr.LayoutDescription{WorkspaceID: space, TabID: tabID, FocusedPaneID: started[0], Root: *tree}, nil
}

// docTree is a copy of the tab's tree as the mirror's document holds it, nil for a tab it lacks.
func (a *Adapter) docTree(tab string) *herdr.LayoutNode {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, ws := range a.state.doc.Workspaces {
		if i := slices.IndexFunc(ws.Tabs, func(t DocTab) bool { return t.ID == tab }); i >= 0 {
			return clone(ws.Tabs[i].Layout)
		}
	}
	return nil
}

func clone(node *herdr.LayoutNode) *herdr.LayoutNode {
	if node == nil {
		return nil
	}
	c := *node
	c.Command, c.Env = slices.Clone(node.Command), maps.Clone(node.Env)
	c.First, c.Second = clone(node.First), clone(node.Second)
	return &c
}
