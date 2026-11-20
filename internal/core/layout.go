package core

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"slices"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/state"
)

// LayoutExport is a tab's pane tree as herdr's layout.export gives it: the document's splits and their shares, each
// pane's folder as it stands now and the label the layout that made it gave it. A pane the tree lacks (one placed by
// its labels alone) goes to the right of the last; one whose terminal ended is left out.
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

// LabelLayout marks the terminals one LayoutApply starts, so a layout that fails ends every one of them, a terminal
// whose start never answered with its address too.
const LabelLayout = "remuda.layout"

// layoutBeforeWrite runs between a layout's last terminal and its document write: a test closes the workspace there.
var layoutBeforeWrite = func() {}

// LayoutApply lays a tree out as a tab, as herdr's layout.apply does: a new tab of the workspace, or, with a tab id, in
// place of that tab, whose panes and what ran in them go once the new ones stand. Each pane is the machine's shell in
// its folder (the workspace's when it names none), or its command started as it is (`literal`) by a core that says it
// can (runLiteral), so no template of the program's name takes its place and a missing program fails the layout; an
// older core is refused a layout with a command. The document keeps the tree with each pane's terminal and label. The
// tab in front stays in front. The tree is taken as scenes checked it. Only a workspace or tab the document holds takes
// a layout, and one closed while the panes started is not brought back: the layout fails and ends what it started.
func (a *Adapter) LayoutApply(workspaceID, tabID, tabLabel string, root herdr.LayoutNode) (*herdr.LayoutDescription, error) {
	panes := root.Panes()
	if len(panes) == 0 {
		return nil, &herdr.Error{Code: "invalid_request", Message: "a layout holds at least one pane"}
	}
	if i := slices.IndexFunc(panes, func(p *herdr.LayoutNode) bool { return len(p.Command) > 0 }); i >= 0 {
		ctx, cancel := context.WithTimeout(context.Background(), callWithin)
		hello, err := a.client.Hello(ctx)
		cancel()
		if err != nil {
			return nil, a.refusal(err, "workspace_not_found")
		}
		if !hello.Has(runLiteral) {
			return nil, &herdr.Error{Code: "invalid_request", Message: fmt.Sprintf("pane %d of the layout runs a command of its own, which waits for a remuda-core that starts a program as it is, not as a template of that name: update it (Core 35)", i+1)}
		}
	}
	space, inPlace := workspaceID, tabID != ""
	if inPlace {
		if space = a.docSpaceOf(tabID); space == "" {
			return nil, tabMissing(tabID)
		}
	} else if !a.inDoc(space) {
		return nil, &herdr.Error{Code: "workspace_not_found", Message: "no workspace " + space + " in remuda's document on this core"}
	}
	snap, err := a.SessionSnapshot()
	if err != nil {
		return nil, err
	}
	var ending []string
	if inPlace {
		ending = panesWhere(snap, func(p *herdr.PaneInfo) bool { return p.TabID == tabID })
	} else {
		tabID = "tab-" + state.NewID()
	}
	folder := ""
	if ws := snap.Workspace(space); ws != nil {
		folder = snap.WorkspaceDir(ws)
	}

	tree, mark := clone(&root), state.NewID()
	var started []string
	fail := func(err error) (*herdr.LayoutDescription, error) {
		if swept := a.endLabelled(map[string]string{LabelLayout: mark}); swept != nil {
			return nil, fmt.Errorf("%w; its panes may still run, as a tab of their own: %v", err, swept)
		}
		return nil, err
	}
	for _, node := range tree.Panes() {
		ctx, cancel := context.WithTimeout(context.Background(), callWithin)
		t, err := a.runIn(ctx, space, tabID, RunParams{Argv: node.Command, Literal: len(node.Command) > 0, Cwd: cmp.Or(node.CWD, folder), Env: node.Env, Labels: map[string]string{LabelLayout: mark}})
		cancel()
		if err != nil {
			return fail(err)
		}
		started = append(started, t.Address)
		node.PaneID, node.CWD, node.Env = t.Address, cmp.Or(node.CWD, t.Cwd), nil
	}
	layoutBeforeWrite()

	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	err = a.editDoc(ctx, func(doc *Document) error {
		i := slices.IndexFunc(doc.Workspaces, func(ws DocWorkspace) bool { return ws.ID == space })
		if i < 0 {
			return spaceMissing(space)
		}
		ws := &doc.Workspaces[i]
		if !inPlace {
			ws.Tabs = append(ws.Tabs, DocTab{ID: tabID, Label: tabLabel, Layout: clone(tree)})
			return nil
		}
		j := slices.IndexFunc(ws.Tabs, func(t DocTab) bool { return t.ID == tabID })
		if j < 0 {
			return tabMissing(tabID)
		}
		ws.Tabs[j].Layout = clone(tree)
		if tabLabel != "" {
			ws.Tabs[j].Label = tabLabel
		}
		return nil
	})
	var refused *herdr.Error
	switch {
	case err == nil:
	case errors.As(err, &refused):
		return fail(err)
	default:
		switch a.holdsTree(tabID, started[0]) {
		case treeAbsent:
			return fail(err)
		case treeUnknown:
			return nil, fmt.Errorf("%w; whether the layout was written is not known, so its panes are left as they are", err)
		}
	}
	if err := a.kill(ending...); err != nil {
		log.Printf("remuda-core: tab %s is laid out anew, but not all its old panes ended: %v", tabID, err)
	}
	return &herdr.LayoutDescription{WorkspaceID: space, TabID: tabID, FocusedPaneID: started[0], Root: *tree}, nil
}

// What the core's document says of a layout's write whose answer never came.
const (
	treeAbsent = iota
	treeHeld
	treeUnknown
)

// holdsTree reads the document the core holds now for the tab's tree with the pane: whether a write whose answer was
// lost was made, or treeUnknown when the core cannot say.
func (a *Adapter) holdsTree(tab, pane string) int {
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	raw, err := a.client.DocGet(ctx, DocName)
	if err != nil {
		return treeUnknown
	}
	doc, err := parseDocument(raw.Value)
	if err != nil {
		return treeUnknown
	}
	for _, ws := range doc.Workspaces {
		for _, t := range ws.Tabs {
			if t.ID == tab && slices.ContainsFunc(t.Layout.Panes(), func(n *herdr.LayoutNode) bool { return n.PaneID == pane }) {
				return treeHeld
			}
		}
	}
	return treeAbsent
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
