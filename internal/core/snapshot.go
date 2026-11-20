package core

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/rpath"
)

// mirror is what the adapter knows of a machine: its live terminals by address and the `remuda` document.
type mirror struct {
	terminals map[string]*Terminal
	doc       Document
	version   string
	// gone are the terminals the core said went since the mirror was filled, so a reply to their start that comes
	// after the news puts none of them back.
	gone map[string]bool
}

// placedTab and placedSpace are a tab and a workspace as the snapshot shows them, their terminals found.
type placedTab struct {
	id, label string
	terminals []*Terminal
}

type placedSpace struct {
	id, label, folder, active string
	worktree                  *herdr.WorktreeInfo
	tabs                      []placedTab
}

// snapshot lays the mirror out as herdr's model. The document's workspaces and tabs come first in its order, each
// tab's terminals from its tree, then the ones labelled for it; a terminal labelled for a tab or workspace the
// document lacks gets one of its own, an unlabelled terminal a workspace of its own; a tab with no live terminal and
// a workspace with no tab are left out.
func (m *mirror) snapshot(sessionOf SessionLookup) *herdr.Snapshot {
	live := m.ordered()
	placed := make(map[string]bool, len(live))
	claim := func(t *Terminal) bool {
		if t == nil || placed[t.Address] {
			return false
		}
		placed[t.Address] = true
		return true
	}
	var spaces []placedSpace
	for _, ws := range m.doc.Workspaces {
		space := placedSpace{id: ws.ID, label: ws.Label, folder: ws.Folder, active: ws.ActiveTab, worktree: ws.Worktree}
		for _, tab := range ws.Tabs {
			shown := placedTab{id: tab.ID, label: tab.Label}
			for _, leaf := range tab.Layout.Panes() {
				if t := m.terminals[leaf.PaneID]; claim(t) {
					shown.terminals = append(shown.terminals, t)
				}
			}
			for _, t := range live {
				if t.Labels[LabelTab] == tab.ID && t.Labels[LabelSpace] == ws.ID && claim(t) {
					shown.terminals = append(shown.terminals, t)
				}
			}
			space.tabs = append(space.tabs, shown)
		}
		for _, t := range live {
			if t.Labels[LabelSpace] == ws.ID && claim(t) {
				space.tabs = addToTab(space.tabs, t.Labels[LabelTab], ws.ID, t)
			}
		}
		spaces = append(spaces, space)
	}
	for _, t := range live {
		if !claim(t) {
			continue
		}
		id := t.Labels[LabelSpace]
		if id == "" {
			id = "core-" + t.Address
		}
		i := slices.IndexFunc(spaces, func(s placedSpace) bool { return s.id == id })
		if i < 0 {
			spaces = append(spaces, placedSpace{id: id, label: folderName(t)})
			i = len(spaces) - 1
		}
		spaces[i].tabs = addToTab(spaces[i].tabs, t.Labels[LabelTab], id, t)
	}

	snap := &herdr.Snapshot{Version: "remuda-core " + m.version}
	for _, space := range spaces {
		var tabs []herdr.TabInfo
		for _, tab := range space.tabs {
			if len(tab.terminals) == 0 {
				continue
			}
			info := herdr.TabInfo{TabID: tab.id, WorkspaceID: space.id, Number: len(tabs) + 1, Label: tab.label}
			layout := herdr.Layout{}
			for _, t := range tab.terminals {
				pane := paneOf(t, tab.id, space.id, sessionOf)
				info.AgentStatus = graver(info.AgentStatus, pane.AgentStatus)
				snap.Panes = append(snap.Panes, pane)
				layout.Panes = append(layout.Panes, herdr.LayoutPane{PaneID: t.Address, Rect: herdr.Rect{Width: int(t.Cols), Height: int(t.Rows)}})
			}
			tabs = append(tabs, info)
			snap.Layouts = append(snap.Layouts, layout)
		}
		if len(tabs) == 0 {
			continue
		}
		ws := herdr.WorkspaceInfo{WorkspaceID: space.id, Label: space.label, Folder: space.folder, ActiveTabID: tabs[0].TabID, Worktree: space.worktree}
		for _, tab := range tabs {
			ws.AgentStatus = graver(ws.AgentStatus, tab.AgentStatus)
			if tab.TabID == space.active {
				ws.ActiveTabID = tab.TabID
			}
		}
		snap.Workspaces = append(snap.Workspaces, ws)
		snap.Tabs = append(snap.Tabs, tabs...)
	}
	return snap
}

// ordered is the live terminals oldest first, so a layout's strays and the workspaces made of them keep their places.
func (m *mirror) ordered() []*Terminal {
	live := make([]*Terminal, 0, len(m.terminals))
	for _, t := range m.terminals {
		live = append(live, t)
	}
	slices.SortFunc(live, func(a, b *Terminal) int {
		return cmp.Or(cmp.Compare(a.CreatedMs, b.CreatedMs), strings.Compare(a.Address, b.Address))
	})
	return live
}

// addToTab puts a terminal in the tab its label names, one made for it when the workspace has none of that id.
func addToTab(tabs []placedTab, id, space string, t *Terminal) []placedTab {
	if id == "" {
		id = space + "-" + t.Address
	}
	if i := slices.IndexFunc(tabs, func(tab placedTab) bool { return tab.id == id }); i >= 0 {
		tabs[i].terminals = append(tabs[i].terminals, t)
		return tabs
	}
	return append(tabs, placedTab{id: id, terminals: []*Terminal{t}})
}

func paneOf(t *Terminal, tab, space string, sessionOf SessionLookup) herdr.PaneInfo {
	pane := herdr.PaneInfo{
		PaneID: t.Address, TabID: tab, WorkspaceID: space, TerminalID: t.Address,
		Agent: agentOf(t), AgentStatus: statusOf(t), CWD: t.Cwd,
		Title: t.Title, TitleStripped: strings.TrimLeftFunc(t.Title, notWordStart), Restored: t.restored,
	}
	if t.Foreground != nil {
		pane.ForegroundCWD = t.Foreground.Cwd
	}
	if pane.Agent != "" && sessionOf != nil {
		pid := t.Pid
		if t.Foreground != nil {
			pid = t.Foreground.Pid
		}
		pane.AgentSession = sessionOf(pane.Agent, pid, cmp.Or(pane.ForegroundCWD, pane.CWD))
	}
	return pane
}

// notWordStart is a title's leading spinner and spaces, which herdr strips from a title too.
func notWordStart(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }

// folderName names a workspace made for a terminal no document places: its folder's name.
func folderName(t *Terminal) string {
	if name := rpath.For(t.Cwd).Base(t.Cwd); name != "" && name != "." && name != "/" && name != `\` {
		return name
	}
	return t.Name
}

var shells = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "fish": true, "dash": true, "ksh": true, "tcsh": true, "csh": true,
	"nu": true, "pwsh": true, "powershell": true, "cmd": true,
}

// agentOf is the agent a terminal runs: the process in its foreground when it is one, nothing when its shell holds
// it, else the app it reported through OSC 7501 (an agent run in a way its process does not say).
func agentOf(t *Terminal) string {
	if fg := t.Foreground; fg != nil {
		if agent := herdr.AgentProgram(append([]string{fg.Name}, fg.Argv...)); agent != "" {
			return agent
		}
		if atPrompt(t) {
			return ""
		}
	}
	if t.Status == nil {
		return ""
	}
	return appAgent(t.Status.App)
}

// appAgent is the agent an OSC 7501 report names as its app, "" for none.
func appAgent(app string) string {
	if agent := herdr.AgentProgram([]string{app}); agent != "" || app == "" {
		return agent
	}
	return strings.ToLower(app)
}

// atPrompt says whether the terminal's own shell holds it: nothing started from it runs in its foreground, and it is
// no scene pane's own command, whose `sh -c '…'` keeps its own process in front all along (LabelCommand).
func atPrompt(t *Terminal) bool {
	fg := t.Foreground
	return fg != nil && fg.Pid == t.Pid && shells[programName(fg.Name)] && t.Labels[LabelCommand] == ""
}

// programName is a process's name without its folder or Windows' .exe.
func programName(name string) string {
	return strings.TrimSuffix(strings.ToLower(rpath.Style{Windows: strings.Contains(name, `\`)}.Base(name)), ".exe")
}

// statusOf is the program's own OSC 7501 word in herdr's: an error ends a turn as done does.
func statusOf(t *Terminal) string {
	if t.Status == nil {
		return herdr.StatusUnknown
	}
	switch t.Status.State {
	case herdr.StatusWorking, herdr.StatusBlocked, herdr.StatusDone, herdr.StatusIdle:
		return t.Status.State
	case "error":
		return herdr.StatusDone
	}
	return herdr.StatusUnknown
}

var pressing = map[string]int{herdr.StatusBlocked: 5, herdr.StatusWorking: 4, herdr.StatusDone: 3, herdr.StatusIdle: 2, herdr.StatusUnknown: 1}

// graver is the status a tab or workspace shows: its most pressing pane's.
func graver(a, b string) string {
	if pressing[b] > pressing[a] {
		return b
	}
	return a
}
