package core

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"slices"
	"time"

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

// LabelLayout marks the terminals one LayoutApply starts, so a layout that fails ends every one of them, a terminal
// whose start never answered with its address too.
const LabelLayout = "remuda.layout"

// layoutBeforeWrite runs between a layout's last terminal and its document write: a test closes the workspace there.
var layoutBeforeWrite = func() {}

// LayoutApply lays a tree out as a tab, as herdr's layout.apply does: a new tab of the workspace, or, with a tab id, in
// place of that tab, whose panes and what ran in them go once the new ones stand. Each pane is the machine's shell in
// its folder (the workspace's when it names none), its command typed at the prompt as the person's shell reads it; the
// document keeps the tree with each pane's terminal, label and command. The tab in front stays in front. The tree is
// taken as scenes checked it. A workspace or tab closed while the panes started is not brought back: the layout fails
// and ends what it started.
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
	var known bool
	if inPlace {
		space = cmp.Or(spaceOfTab(snap, tabID), a.docSpaceOf(tabID))
		if space == "" {
			return nil, tabMissing(tabID)
		}
		ending = panesWhere(snap, func(p *herdr.PaneInfo) bool { return p.TabID == tabID })
		known = a.docSpaceOf(tabID) != ""
	} else {
		if snap.Workspace(space) == nil && !a.inDoc(space) {
			return nil, spaceMissing(space)
		}
		tabID = "tab-" + state.NewID()
		known = a.inDoc(space)
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
		t, err := a.startPane(space, tabID, mark, cmp.Or(node.CWD, folder), node.Env, node.Command)
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
		if ended := a.endedOf(started); ended != "" {
			return &herdr.Error{Code: "pane_not_found", Message: "pane " + ended + " of the layout ended before it was laid out"}
		}
		switch {
		case inPlace && known && docSpaceOfTab(doc, tabID) == "":
			return tabMissing(tabID)
		case !inPlace && known && !slices.ContainsFunc(doc.Workspaces, func(ws DocWorkspace) bool { return ws.ID == space }):
			return spaceMissing(space)
		}
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

// startPane starts one pane of a layout: the machine's shell, carrying the layout's mark, and the pane's command typed at
// its prompt once the shell holds the terminal, quoted for that shell. Typed, never run as the terminal's program: the
// core would read a program named as one of its templates as that template, and the shell gives the command the
// person's own PATH.
func (a *Adapter) startPane(space, tab, mark, cwd string, env map[string]string, command []string) (Terminal, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	t, err := a.runIn(ctx, space, tab, RunParams{Cwd: cwd, Env: env, Labels: map[string]string{LabelLayout: mark}})
	if err != nil || len(command) == 0 {
		return t, err
	}
	for {
		shell, err := a.terminal(t.Address)
		if err != nil {
			return t, err
		}
		if atPrompt(&shell) {
			if _, err := a.client.Send(ctx, t.Address, commandLine(programName(shell.Foreground.Name), command)); err != nil {
				return t, a.refusal(err, "pane_not_found")
			}
			return t, nil
		}
		select {
		case <-ctx.Done():
			return t, fmt.Errorf("the shell of pane %s never came to its prompt for %q: %w", t.Address, command[0], ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// endedOf is a terminal of the ones given that the mirror no longer holds, "" while all stand.
func (a *Adapter) endedOf(addresses []string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, address := range addresses {
		if a.state.terminals[address] == nil {
			return address
		}
	}
	return ""
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
