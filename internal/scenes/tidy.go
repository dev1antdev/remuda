package scenes

import (
	"sort"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/radar"
	"github.com/dev1antdev/remuda/internal/text"
	"github.com/dev1antdev/remuda/internal/worktrees"
)

// The things a tidy does, in the order it does them: a /compact while the pane is still there, then the closes, then
// the worktrees.
const (
	ActCompact    = "compact"
	ActCloseAgent = "close-agent"
	ActCloseShell = "close-shell"
	ActWorktree   = "worktree"
)

var actOrder = map[string]int{ActCompact: 0, ActCloseAgent: 1, ActCloseShell: 2, ActWorktree: 3}

// CompactText is what a tidy types into an idle Claude Code past its context share, because the person pressed it.
const CompactText = "/compact"

// Action is one thing a tidy would do, with what the person needs to judge it.
type Action struct {
	ID        string `json:"id"` // the kind, the machine and the pane or folder: what the run is asked for
	Kind      string `json:"kind"`
	Machine   string `json:"machine"`
	Pane      string `json:"pane,omitempty"`
	Workspace string `json:"workspace,omitempty"` // the workspace's name as the sidebar says it
	Tab       string `json:"tab,omitempty"`       // the tab's own name
	Agent     string `json:"agent,omitempty"`     // the agent's kind
	Title     string `json:"title,omitempty"`     // the pane's title
	Since     int64  `json:"since,omitempty"`     // unix ms the agent last worked, for a close
	Context   int    `json:"context,omitempty"`   // percent of the window its last answer read, for a /compact
	Closes    string `json:"closes,omitempty"`    // tab | workspace: what goes with the pane, its last
	Path      string `json:"path,omitempty"`      // a leftover worktree's folder
	Branch    string `json:"branch,omitempty"`
	KB        int64  `json:"kb,omitempty"`
}

// Unread is a machine a tidy could not look at, and why.
type Unread struct {
	Machine string `json:"machine"`
	Label   string `json:"label"`
	Why     string `json:"why"`
}

// Plan is what a tidy would do now: its actions in the order it would do them, and the machines it could not look at.
type Plan struct {
	Actions []Action `json:"actions"`
	Unread  []Unread `json:"unread"`
}

// Readings are what the page reads about a pane that a tidy goes by.
type Readings struct {
	State   func(machine string, pane *herdr.PaneInfo) radar.Display // the radar's reading: working, blocked, done, idle
	Context func(machine, pane string) (percent int, ok bool)        // the meter's share of the window its last answer read
	Scratch func(machine, workspace string) bool                     // the floating terminal's own shell, never tidied
}

// planPanes is what the tidy does with one machine's panes: a shell at its prompt closed, an agent at rest for long
// enough with a session to come back to closed, an idle Claude Code past the share compacted — never an agent that
// works, asks, has a finish nobody looked at, or that herdr cannot say; the scratch shell left alone.
func planPanes(t Tidy, machine string, snap *herdr.Snapshot, atPrompt func(pane string) bool, read Readings, now time.Time) []Action {
	var out []Action
	for i := range snap.Panes {
		pane := &snap.Panes[i]
		if read.Scratch != nil && read.Scratch(machine, pane.WorkspaceID) {
			continue
		}
		action := Action{Machine: machine, Pane: pane.PaneID, Workspace: snap.WorkspaceLabel(pane.WorkspaceID), Tab: tabLabel(snap, pane.TabID), Agent: pane.Agent, Title: pane.DisplayTitle(), Closes: closes(snap, pane)}
		if pane.Agent == "" {
			if t.Shells && atPrompt(pane.PaneID) {
				action.Kind = ActCloseShell
				out = append(out, action)
			}
			continue
		}
		reading := read.State(machine, pane)
		if reading.State != herdr.StatusIdle || (pane.AgentStatus != herdr.StatusIdle && pane.AgentStatus != herdr.StatusDone) {
			continue
		}
		last := reading.LastActive
		if last == 0 {
			last = reading.Since
		}
		if t.Idle && resumable(pane) && last > 0 && now.Sub(time.UnixMilli(last)) >= time.Duration(t.IdleHours)*time.Hour {
			action.Kind, action.Since = ActCloseAgent, last
			out = append(out, action)
			continue
		}
		if t.Compact && pane.Agent == "claude" && read.Context != nil {
			if share, ok := read.Context(machine, pane.PaneID); ok && share >= t.CompactAt {
				action.Kind, action.Context, action.Closes = ActCompact, share, ""
				out = append(out, action)
			}
		}
	}
	for i := range out {
		out[i].ID = out[i].Kind + "|" + machine + "|" + out[i].Pane
	}
	return out
}

// resumable says whether a closed agent can be reopened on its session: herdr names one remuda can follow.
func resumable(pane *herdr.PaneInfo) bool {
	return pane.AgentSession != nil && text.SessionID(pane.AgentSession.Value)
}

// closes says what goes with a pane when it closes: its tab when it is the tab's last, the workspace when that tab is the workspace's last.
func closes(snap *herdr.Snapshot, pane *herdr.PaneInfo) string {
	for i := range snap.Panes {
		if other := &snap.Panes[i]; other.PaneID != pane.PaneID && other.TabID == pane.TabID {
			return ""
		}
	}
	for _, tab := range snap.Tabs {
		if tab.WorkspaceID == pane.WorkspaceID && tab.TabID != pane.TabID {
			return "tab"
		}
	}
	return "workspace"
}

func tabLabel(snap *herdr.Snapshot, tabID string) string {
	for _, tab := range snap.Tabs {
		if tab.TabID == tabID {
			return tab.Label
		}
	}
	return ""
}

// planWorktrees are the leftover worktrees a tidy removes: the clean ones git still knows.
func planWorktrees(machine string, list []worktrees.Leftover) []Action {
	var out []Action
	for _, left := range list {
		if left.Dirty != "" || left.Broken != "" {
			continue
		}
		out = append(out, Action{ID: ActWorktree + "|" + machine + "|" + left.Path, Kind: ActWorktree, Machine: machine, Path: left.Path, Branch: left.Branch, KB: left.KB})
	}
	return out
}

// Plan is what the tidy would do now on the machines it covers, every pane read afresh.
// The machines are looked at side by side (a remote's shells are a call each, its worktrees a du), each machine's part
// kept in the fleet's order.
func (s *Service) Plan(t Tidy, now time.Time) Plan {
	var covered []Machine
	for _, mc := range s.Machines() {
		if t.Covers(mc.ID()) {
			covered = append(covered, mc)
		}
	}
	parts := make([]Plan, len(covered))
	var wg sync.WaitGroup
	for i, mc := range covered {
		wg.Add(1)
		go func() {
			defer wg.Done()
			parts[i] = s.planMachine(t, mc, now)
		}()
	}
	wg.Wait()
	plan := Plan{Actions: []Action{}, Unread: []Unread{}}
	for _, part := range parts {
		plan.Actions = append(plan.Actions, part.Actions...)
		plan.Unread = append(plan.Unread, part.Unread...)
	}
	sort.SliceStable(plan.Actions, func(i, j int) bool { return actOrder[plan.Actions[i].Kind] < actOrder[plan.Actions[j].Kind] })
	return plan
}

// planMachine is the tidy's part on one machine, or why it could not look there.
func (s *Service) planMachine(t Tidy, mc Machine, now time.Time) Plan {
	var plan Plan
	unread := func(why string) Plan {
		plan.Unread = append(plan.Unread, Unread{Machine: mc.ID(), Label: mc.Label(), Why: why})
		return plan
	}
	client, err := mc.Herdr()
	if err != nil {
		return unread(err.Error())
	}
	snap := mc.Snapshot()
	if snap == nil {
		return unread("no snapshot of the machine yet")
	}
	atPrompt := func(pane string) bool {
		at, err := client.PaneAtPrompt(pane)
		return err == nil && at
	}
	plan.Actions = planPanes(t, mc.ID(), snap, atPrompt, s.Readings, now)
	if t.Worktrees {
		list, err := mc.Leftovers(worktrees.OpenFolders(snap))
		if err != nil {
			return unread("its worktrees could not be read: " + err.Error())
		}
		plan.Actions = append(plan.Actions, planWorktrees(mc.ID(), list)...)
	}
	return plan
}

// staleWhy is why an action the person picked is no longer in the plan: its pane gone, or the agent at work or asking now.
func staleWhy(a Action, snap *herdr.Snapshot, read Readings) string {
	if a.Kind == ActWorktree {
		return "no longer a clean leftover worktree"
	}
	if snap == nil {
		return "its machine has not been read again"
	}
	pane := snap.Pane(a.Pane)
	if pane == nil {
		return "gone already"
	}
	if pane.Agent == "" {
		return "something runs in it now"
	}
	if a.Kind == ActCloseShell {
		return "an agent runs in it now"
	}
	switch read.State(a.Machine, pane).State {
	case herdr.StatusWorking:
		return "it works now"
	case herdr.StatusBlocked:
		return "it asks you something now"
	case herdr.StatusDone:
		return "it has a finish you have not looked at"
	}
	return "no longer to tidy"
}
