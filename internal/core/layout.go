package core

import (
	"cmp"
	"context"
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

// sweepFor is how long a layout that failed keeps asking a core it cannot reach for its terminals, to end them.
const sweepFor = 2 * time.Minute

// layoutBeforeWrite runs between a layout's last terminal and its document write: a test closes the workspace there.
var layoutBeforeWrite = func() {}

// LayoutApply lays a tree out as a tab, as herdr's layout.apply does: a new tab of the workspace, or, with a tab id, in
// place of that tab, whose panes and what ran in them go once the new ones stand. Each pane runs its command, else the
// machine's shell, in its folder (the workspace's when it names none); the document keeps the tree with each pane's
// terminal, label and command. The tab in front stays in front. The tree is taken as scenes checked it. A workspace or
// tab closed while the panes started is not brought back: the layout fails and ends what it started.
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
	tree := clone(&root)
	if err := a.refuseTemplates(tree); err != nil {
		return nil, err
	}

	mark := state.NewID()
	var started []string
	fail := func(err error) (*herdr.LayoutDescription, error) {
		a.sweep(mark)
		return nil, err
	}
	for _, node := range tree.Panes() {
		ctx, cancel := context.WithTimeout(context.Background(), callWithin)
		t, err := a.runIn(ctx, space, tabID, RunParams{Argv: node.Command, Cwd: cmp.Or(node.CWD, folder), Env: node.Env, Labels: map[string]string{LabelLayout: mark}})
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
	if err != nil && !(IsTransport(err) && a.holdsTree(tabID, started[0])) {
		return fail(err)
	}
	if err := a.kill(ending...); err != nil {
		log.Printf("remuda-core: tab %s is laid out anew, but not all its old panes ended: %v", tabID, err)
	}
	return &herdr.LayoutDescription{WorkspaceID: space, TabID: tabID, FocusedPaneID: started[0], Root: *tree}, nil
}

// refuseTemplates refuses a pane whose program the core would read as one of its templates: it would run the
// template's command, with the rest of the pane's command as its task, not the program the layout names.
func (a *Adapter) refuseTemplates(tree *herdr.LayoutNode) error {
	var programs []string
	for _, node := range tree.Panes() {
		if len(node.Command) > 0 {
			programs = append(programs, node.Command[0])
		}
	}
	if len(programs) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	var listed struct {
		Templates []struct {
			Name string `json:"name"`
		} `json:"templates"`
	}
	if err := a.client.Call(ctx, "templates.list", map[string]any{}, &listed); err != nil {
		return a.refusal(err, "invalid_request")
	}
	for _, template := range listed.Templates {
		if slices.Contains(programs, template.Name) {
			return &herdr.Error{Code: "invalid_request", Message: fmt.Sprintf("a pane runs %q, which remuda-core on this machine reads as its template of that name: name the program by its path", template.Name)}
		}
	}
	return nil
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

// holdsTree says whether the document the core holds now lays the tab out with the pane: a write whose answer was lost
// may have been made.
func (a *Adapter) holdsTree(tab, pane string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	raw, err := a.client.DocGet(ctx, DocName)
	if err != nil {
		return false
	}
	doc, err := parseDocument(raw.Value)
	if err != nil {
		return false
	}
	for _, ws := range doc.Workspaces {
		for _, t := range ws.Tabs {
			if t.ID == tab && slices.ContainsFunc(t.Layout.Panes(), func(n *herdr.LayoutNode) bool { return n.PaneID == pane }) {
				return true
			}
		}
	}
	return false
}

// sweep ends what a layout that failed started: every terminal carrying its mark, asked of the core again for sweepFor
// while it cannot be reached, since a start whose answer was lost left a terminal running that nothing else names.
func (a *Adapter) sweep(mark string) {
	if !a.endMarked(mark) {
		go func() {
			for until := time.Now().Add(sweepFor); time.Now().Before(until); {
				select {
				case <-a.done:
					return
				case <-time.After(time.Second):
				}
				if a.endMarked(mark) {
					return
				}
			}
			log.Printf("remuda-core: a layout that failed may have left terminals marked %s=%s running", LabelLayout, mark)
		}()
	}
}

// endMarked ends every terminal carrying the layout's mark; false when the core could not be asked.
func (a *Adapter) endMarked(mark string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	left, err := a.client.List(ctx, false, map[string]string{LabelLayout: mark})
	if IsTransport(err) {
		return false
	}
	var addresses []string
	for _, t := range left {
		addresses = append(addresses, t.Address)
	}
	a.kill(addresses...)
	return true
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
