package core

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// docTries is how often a change of the document is made again when another writer moved it in between.
const docTries = 5

// killWithin bounds the end of one terminal: hangup, then terminate, then kill.
const killWithin = 15 * time.Second

// newID is a workspace's or a tab's id in the document: its prefix and eight hex digits.
func newID(prefix string) string {
	var b [4]byte
	rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:])
}

func leaf(address string) *herdr.LayoutNode { return &herdr.LayoutNode{Type: "pane", PaneID: address} }

// shown is the machine as the mirror holds it now, laid out as the snapshot shows it.
func (a *Adapter) shown() (*herdr.Snapshot, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.held {
		return nil, a.whyNotHeld()
	}
	return a.state.snapshot(nil), nil
}

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
		a.mu.Lock()
		a.state.doc = doc
		a.tellLocked(herdr.Event{Kind: "layout.updated"})
		a.mu.Unlock()
		return nil
	}
}

// runShell starts the machine's shell in a new terminal of a workspace's tab and puts it in the mirror at once, so the
// pane it answers is there for the caller's next request; the core's own news of it finds it there.
func (a *Adapter) runShell(ctx context.Context, space, tab, cwd string, env map[string]string) (Terminal, error) {
	t, err := a.client.Run(ctx, RunParams{Cwd: cwd, Env: env, Labels: map[string]string{LabelSpace: space, LabelTab: tab}, Rm: true})
	if err != nil {
		return Terminal{}, a.refusal(err, "workspace_not_found")
	}
	a.mu.Lock()
	if a.held && local(t.Address) && a.state.terminals[t.Address] == nil {
		record := t
		followShell(&record)
		a.state.terminals[t.Address] = &record
		a.tellLocked(herdr.Event{Kind: "pane.created", PaneID: t.Address})
	}
	a.mu.Unlock()
	return t, nil
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
	for _, t := range snap.Tabs {
		if t.TabID == tab {
			label = t.Label
		}
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

// WorkspaceCreate opens a workspace on a folder (the core's own when none): a shell in a tab of its own.
func (a *Adapter) WorkspaceCreate(cwd, label string) (*herdr.WorkspaceCreateResult, error) {
	if _, err := a.shown(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	space, tab := newID("ws-"), newID("tab-")
	t, err := a.runShell(ctx, space, tab, cwd, nil)
	if err != nil {
		return nil, err
	}
	folder := cmp.Or(cwd, t.Cwd)
	label = cmp.Or(label, folderName(&Terminal{Cwd: folder, Name: t.Name}))
	err = a.editDoc(ctx, func(doc *Document) error {
		doc.Workspaces = append(doc.Workspaces, DocWorkspace{ID: space, Label: label, Folder: folder, ActiveTab: tab, Tabs: []DocTab{{ID: tab, Layout: leaf(t.Address)}}})
		return nil
	})
	if err != nil {
		a.kill(t.Address)
		return nil, err
	}
	return &herdr.WorkspaceCreateResult{
		Workspace: herdr.WorkspaceInfo{WorkspaceID: space, Label: label, Folder: folder, ActiveTabID: tab},
		RootPane:  paneOf(&t, tab, space, nil),
	}, nil
}

// TabCreate opens another tab in the workspace's folder.
func (a *Adapter) TabCreate(workspaceID, label string) (*herdr.TabCreateResult, error) {
	return a.TabCreateIn(workspaceID, label, "", nil)
}

// TabCreateIn opens another tab of the workspace, its shell in the folder given (the workspace's when none) with env.
func (a *Adapter) TabCreateIn(workspaceID, label, cwd string, env map[string]string) (*herdr.TabCreateResult, error) {
	snap, err := a.shown()
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
	tab := newID("tab-")
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
	snap, err := a.shown()
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
	snap, err := a.shown()
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
	for _, t := range snap.Tabs {
		if t.TabID == tab {
			return t.WorkspaceID
		}
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

// WorkspaceClose ends every terminal of the workspace and takes it out of the document.
func (a *Adapter) WorkspaceClose(workspaceID string) error {
	snap, err := a.shown()
	if err != nil {
		return err
	}
	if snap.Workspace(workspaceID) == nil && !a.inDoc(workspaceID) {
		return spaceMissing(workspaceID)
	}
	if err := a.kill(panesWhere(snap, func(p *herdr.PaneInfo) bool { return p.WorkspaceID == workspaceID })...); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	return a.editDoc(ctx, func(doc *Document) error {
		doc.Workspaces = slices.DeleteFunc(doc.Workspaces, func(ws DocWorkspace) bool { return ws.ID == workspaceID })
		return nil
	})
}

// TabClose ends every terminal of the tab and takes it out of the document; the workspace goes with its last tab, as
// herdr's does.
func (a *Adapter) TabClose(tabID string) error {
	snap, err := a.shown()
	if err != nil {
		return err
	}
	space := spaceOfTab(snap, tabID)
	panes := panesWhere(snap, func(p *herdr.PaneInfo) bool { return p.TabID == tabID })
	if space == "" && !a.tabInDoc(tabID) {
		return tabMissing(tabID)
	}
	if err := a.kill(panes...); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), callWithin)
	defer cancel()
	return a.editDoc(ctx, func(doc *Document) error {
		dropTab(doc, snap, tabID, panes)
		return nil
	})
}

func (a *Adapter) tabInDoc(tab string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return docSpaceOfTab(&a.state.doc, tab) != ""
}

// dropTab takes a tab out of the document, and its workspace when no other tab of it holds a terminal that stays.
func dropTab(doc *Document, snap *herdr.Snapshot, tab string, ending []string) {
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
		stays := panesWhere(snap, func(p *herdr.PaneInfo) bool { return p.WorkspaceID == ws.ID && !slices.Contains(ending, p.PaneID) })
		if len(stays) == 0 {
			doc.Workspaces = slices.Delete(doc.Workspaces, i, i+1)
		}
		return
	}
}
