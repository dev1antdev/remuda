package core

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/state"
)

// docTries is how often a change of the document is made again when another writer moved it in between.
const docTries = 5

// killWithin bounds the end of one terminal: hangup, then terminate, then kill.
const killWithin = 15 * time.Second

func leaf(address string) *herdr.LayoutNode { return &herdr.LayoutNode{Type: "pane", PaneID: address} }

// editDoc reads the document, changes it and writes it at the version read; a write another remuda made in between
// has the change made again on what it wrote. The mirror takes the written document at once, so the snapshot asked for
// next shows it.
func (a *Adapter) editDoc(ctx context.Context, change func(*Document) error) error {
	a.docMu.Lock()
	defer a.docMu.Unlock()
	for try := 1; ; try++ {
		raw, err := a.client.DocGet(ctx, DocName)
		if err != nil {
			return a.refusal(err, "workspace_not_found")
		}
		doc, err := parseDocument(raw.Value)
		if err != nil {
			return fmt.Errorf("the %q document on remuda-core is not one of remuda's, so it is left as it is: %w", DocName, err)
		}
		if err := change(&doc); err != nil {
			return err
		}
		_, err = a.client.DocSet(ctx, DocName, doc, raw.Version)
		if IsKind(err, "conflict") && try < docTries {
			continue
		}
		if err != nil {
			return a.refusal(err, "workspace_not_found")
		}
		a.setDoc(doc)
		return nil
	}
}

// runShell starts the machine's shell in a new terminal of a workspace's tab.
func (a *Adapter) runShell(ctx context.Context, space, tab, cwd string, env map[string]string) (Terminal, error) {
	return a.runIn(ctx, space, tab, RunParams{Cwd: cwd, Env: env})
}

// runIn starts what p says (the machine's shell when it names nothing) in a new terminal of a workspace's tab, its
// labels beside the tab's, and waits for the core's own news of it, so the pane it answers is in the snapshot the
// caller reads next. The mirror alone puts terminals in it: a reply to the start never does, which could bring back a
// shell that ended meanwhile. One that ends first fails the start. The terminal is kept: a lost one comes back as the
// machine's shell, never its command run again.
func (a *Adapter) runIn(ctx context.Context, space, tab string, p RunParams) (Terminal, error) {
	p.Labels = maps.Clone(p.Labels)
	if p.Labels == nil {
		p.Labels = map[string]string{}
	}
	p.Labels[LabelSpace], p.Labels[LabelTab], p.Rm, p.Keep = space, tab, true, true
	what, where := "the shell", cmp.Or(p.Cwd, "the core's folder")
	if len(p.Argv) > 0 {
		what = "the program " + p.Argv[0]
	}
	t, err := a.client.Run(ctx, p)
	if err != nil {
		return Terminal{}, a.refusal(err, "workspace_not_found")
	}
	for {
		a.mu.Lock()
		held, gone := a.state.terminals[t.Address] != nil, a.state.gone[t.Address]
		a.mu.Unlock()
		switch {
		case held:
			return t, nil
		case gone || t.State != "live":
			return Terminal{}, fmt.Errorf("%s in %s ended as soon as it started", what, where)
		}
		select {
		case <-ctx.Done():
			a.kill(t.Address)
			return Terminal{}, fmt.Errorf("remuda-core never told of %s it started in %s: %w", what, where, ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// holds says whether a live terminal the mirror holds now, other than the ones ending, is in a place the snapshot
// shows: read when the document is written, not before the terminals were ended, so a terminal another request put
// there meanwhile keeps its place.
func (a *Adapter) holds(pick func(*herdr.PaneInfo) bool, ending []string) bool {
	a.mu.Lock()
	snap := a.state.snapshot(nil)
	a.mu.Unlock()
	return len(panesWhere(snap, func(p *herdr.PaneInfo) bool { return pick(p) && !slices.Contains(ending, p.PaneID) })) > 0
}

// kill ends terminals side by side; one the core no longer has is gone already.
func (a *Adapter) kill(addresses ...string) error {
	errs := make([]error, len(addresses))
	var wg sync.WaitGroup
	for i, address := range addresses {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), killWithin)
			defer cancel()
			if _, err := a.client.Kill(ctx, address); err != nil && !IsKind(err, "not_found") {
				errs[i] = a.refusal(err, "pane_not_found")
			}
		})
	}
	wg.Wait()
	return cmp.Or(errs...)
}

// adopt puts a workspace the snapshot shows in the document when the document lacks it — one made of terminals placed
// by their labels alone, or of none — so it can be renamed or given a tab; false for a workspace the machine lacks.
func adopt(doc *Document, snap *herdr.Snapshot, space string) (*DocWorkspace, bool) {
	if i := slices.IndexFunc(doc.Workspaces, func(ws DocWorkspace) bool { return ws.ID == space }); i >= 0 {
		return &doc.Workspaces[i], true
	}
	ws := snap.Workspace(space)
	if ws == nil {
		return nil, false
	}
	doc.Workspaces = append(doc.Workspaces, DocWorkspace{ID: space, Label: ws.Label, Folder: snap.WorkspaceDir(ws), ActiveTab: ws.ActiveTabID})
	return &doc.Workspaces[len(doc.Workspaces)-1], true
}

// adoptTab is adopt for a tab of the workspace.
func adoptTab(ws *DocWorkspace, snap *herdr.Snapshot, tab string) *DocTab {
	if i := slices.IndexFunc(ws.Tabs, func(t DocTab) bool { return t.ID == tab }); i >= 0 {
		return &ws.Tabs[i]
	}
	label := ""
	if shown := snap.Tab(tab); shown != nil {
		label = shown.Label
	}
	ws.Tabs = append(ws.Tabs, DocTab{ID: tab, Label: label})
	return &ws.Tabs[len(ws.Tabs)-1]
}

func spaceMissing(space string) error {
	return &herdr.Error{Code: "workspace_not_found", Message: "no workspace " + space}
}

func tabMissing(tab string) error {
	return &herdr.Error{Code: "tab_not_found", Message: "no tab " + tab}
}

// WorkspaceCreate opens a workspace on a folder (the core's own when none): a shell in a tab of its own, and the git
// checkout the folder is, read once.
func (a *Adapter) WorkspaceCreate(cwd, label string) (*herdr.WorkspaceCreateResult, error) {
	return a.workspaceOn(cwd, label, nil)
}

// workspaceOn is WorkspaceCreate with the checkout known already (a worktree just made); nil reads it.
func (a *Adapter) workspaceOn(cwd, label string, tree *herdr.WorktreeInfo) (*herdr.WorkspaceCreateResult, error) {
	if _, err := a.SessionSnapshot(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	space, tab := "ws-"+state.NewID(), "tab-"+state.NewID()
	t, err := a.runShell(ctx, space, tab, cwd, nil)
	if err != nil {
		return nil, err
	}
	folder := cmp.Or(cwd, t.Cwd)
	label = cmp.Or(label, folderName(&Terminal{Cwd: folder, Name: t.Name}))
	if tree == nil {
		tree = a.checkoutOf(folder)
	}
	err = a.editDoc(ctx, func(doc *Document) error {
		doc.Workspaces = append(doc.Workspaces, DocWorkspace{ID: space, Label: label, Folder: folder, ActiveTab: tab, Tabs: []DocTab{{ID: tab, Layout: leaf(t.Address)}}, Worktree: tree})
		return nil
	})
	if err != nil {
		a.kill(t.Address)
		return nil, err
	}
	return &herdr.WorkspaceCreateResult{
		Workspace: herdr.WorkspaceInfo{WorkspaceID: space, Label: label, Folder: folder, ActiveTabID: tab, Worktree: tree},
		RootPane:  paneOf(&t, tab, space, nil),
	}, nil
}

// TabCreate opens another tab in the workspace's folder.
func (a *Adapter) TabCreate(workspaceID, label string) (*herdr.TabCreateResult, error) {
	return a.TabCreateIn(workspaceID, label, "", nil)
}

// TabCreateIn opens another tab of the workspace, its shell in the folder given (the workspace's when none) with env.
func (a *Adapter) TabCreateIn(workspaceID, label, cwd string, env map[string]string) (*herdr.TabCreateResult, error) {
	snap, err := a.SessionSnapshot()
	if err != nil {
		return nil, err
	}
	ws := snap.Workspace(workspaceID)
	if ws == nil && !a.inDoc(workspaceID) {
		return nil, spaceMissing(workspaceID)
	}
	if cwd == "" && ws != nil {
		cwd = snap.WorkspaceDir(ws)
	}
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	tab := "tab-" + state.NewID()
	t, err := a.runShell(ctx, workspaceID, tab, cwd, env)
	if err != nil {
		return nil, err
	}
	number := 0
	err = a.editDoc(ctx, func(doc *Document) error {
		space, ok := adopt(doc, snap, workspaceID)
		if !ok {
			return spaceMissing(workspaceID)
		}
		space.Tabs = append(space.Tabs, DocTab{ID: tab, Label: label, Layout: leaf(t.Address)})
		number = len(space.Tabs)
		return nil
	})
	if err != nil {
		a.kill(t.Address)
		return nil, err
	}
	return &herdr.TabCreateResult{
		Tab:      herdr.TabInfo{TabID: tab, WorkspaceID: workspaceID, Number: number, Label: label},
		RootPane: paneOf(&t, tab, workspaceID, nil),
	}, nil
}

// inDoc says whether the mirror's document names the workspace (one whose tabs all ended shows in no snapshot).
func (a *Adapter) inDoc(space string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.ContainsFunc(a.state.doc.Workspaces, func(ws DocWorkspace) bool { return ws.ID == space })
}

// WorkspaceRename names a workspace.
func (a *Adapter) WorkspaceRename(workspaceID, label string) error {
	snap, err := a.SessionSnapshot()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	return a.editDoc(ctx, func(doc *Document) error {
		space, ok := adopt(doc, snap, workspaceID)
		if !ok {
			return spaceMissing(workspaceID)
		}
		space.Label = label
		return nil
	})
}

// TabRename names a tab; an empty label leaves it unnamed.
func (a *Adapter) TabRename(tabID, label string) error {
	snap, err := a.SessionSnapshot()
	if err != nil {
		return err
	}
	space := spaceOfTab(snap, tabID)
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	return a.editDoc(ctx, func(doc *Document) error {
		if space == "" {
			space = docSpaceOfTab(doc, tabID)
		}
		ws, ok := adopt(doc, snap, space)
		if !ok {
			return tabMissing(tabID)
		}
		adoptTab(ws, snap, tabID).Label = label
		return nil
	})
}

func spaceOfTab(snap *herdr.Snapshot, tab string) string {
	if shown := snap.Tab(tab); shown != nil {
		return shown.WorkspaceID
	}
	return ""
}

func docSpaceOfTab(doc *Document, tab string) string {
	for _, ws := range doc.Workspaces {
		if slices.ContainsFunc(ws.Tabs, func(t DocTab) bool { return t.ID == tab }) {
			return ws.ID
		}
	}
	return ""
}

// panesWhere are the addresses of the snapshot's panes a test picks.
func panesWhere(snap *herdr.Snapshot, pick func(*herdr.PaneInfo) bool) []string {
	var out []string
	for i := range snap.Panes {
		if pick(&snap.Panes[i]) {
			out = append(out, snap.Panes[i].PaneID)
		}
	}
	return out
}

// closeBeforeWrite runs between a close's ending of the terminals it saw and its document write: a test lays a layout
// out there.
var closeBeforeWrite = func() {}

// WorkspaceClose ends every terminal of the workspace and takes it out of the document, then ends whatever terminal
// still carries its label: one a layout or a tab started while it closed.
func (a *Adapter) WorkspaceClose(workspaceID string) error {
	snap, err := a.SessionSnapshot()
	if err != nil {
		return err
	}
	if snap.Workspace(workspaceID) == nil && !a.inDoc(workspaceID) {
		return spaceMissing(workspaceID)
	}
	if err := a.kill(panesWhere(snap, func(p *herdr.PaneInfo) bool { return p.WorkspaceID == workspaceID })...); err != nil {
		return err
	}
	closeBeforeWrite()
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	err = a.editDoc(ctx, func(doc *Document) error {
		doc.Workspaces = slices.DeleteFunc(doc.Workspaces, func(ws DocWorkspace) bool { return ws.ID == workspaceID })
		return nil
	})
	if err != nil {
		return err
	}
	return a.endLabelled(map[string]string{LabelSpace: workspaceID})
}

// endLabelled ends every live terminal carrying the labels.
func (a *Adapter) endLabelled(labels map[string]string) error {
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	left, err := a.client.List(ctx, false, labels)
	if err != nil {
		return a.refusal(err, "pane_not_found")
	}
	addresses := make([]string, 0, len(left))
	for _, t := range left {
		addresses = append(addresses, t.Address)
	}
	return a.kill(addresses...)
}

// TabClose ends every terminal of the tab and takes it out of the document; the workspace goes with its last tab, as
// herdr's does.
func (a *Adapter) TabClose(tabID string) error {
	snap, err := a.SessionSnapshot()
	if err != nil {
		return err
	}
	panes := panesWhere(snap, func(p *herdr.PaneInfo) bool { return p.TabID == tabID })
	if cmp.Or(spaceOfTab(snap, tabID), a.docSpaceOf(tabID)) == "" {
		return tabMissing(tabID)
	}
	if err := a.kill(panes...); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	err = a.editDoc(ctx, func(doc *Document) error {
		a.dropTab(doc, tabID, panes)
		return nil
	})
	if err != nil {
		return err
	}
	return a.endLabelled(map[string]string{LabelTab: tabID})
}

// docSpaceOf is the workspace the mirror's document puts a tab in ("" when none does).
func (a *Adapter) docSpaceOf(tab string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return docSpaceOfTab(&a.state.doc, tab)
}

// dropTab takes a tab out of the document as it reads now, and its workspace when the document keeps no other tab of
// it and no terminal but the ending ones is in it.
func (a *Adapter) dropTab(doc *Document, tab string, ending []string) {
	for i := range doc.Workspaces {
		ws := &doc.Workspaces[i]
		if !slices.ContainsFunc(ws.Tabs, func(t DocTab) bool { return t.ID == tab }) {
			continue
		}
		ws.Tabs = slices.DeleteFunc(ws.Tabs, func(t DocTab) bool { return t.ID == tab })
		if ws.ActiveTab == tab {
			ws.ActiveTab = ""
			if len(ws.Tabs) > 0 {
				ws.ActiveTab = ws.Tabs[0].ID
			}
		}
		space := ws.ID
		if len(ws.Tabs) == 0 && !a.holds(func(p *herdr.PaneInfo) bool { return p.WorkspaceID == space }, ending) {
			doc.Workspaces = slices.Delete(doc.Workspaces, i, i+1)
		}
		return
	}
}
