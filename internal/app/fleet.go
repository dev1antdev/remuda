package app

import (
	"path"

	"github.com/dev1antdev/remuda/internal/awake"
	"github.com/dev1antdev/remuda/internal/budget"
	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/limits"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/placement"
	"github.com/dev1antdev/remuda/internal/tasks"
	"github.com/dev1antdev/remuda/internal/watch"
)

// fleetPlacer is where a project whose queue may use any machine starts its next card: the connected machines of the
// fleet — never the host of a remuda that only serves the page — holding a workspace on a checkout of the project's
// repository, each with its vitals, its plan window for the agent kind, its agents at work and its budgets.
type fleetPlacer struct {
	fleet   *machines.Manager
	watcher *watch.Watcher
	vitals  *health.Collector
	limits  *limits.Service
	budgets *budget.Service
	remotes *placement.Remotes
}

// Place gathers the candidates and chooses among them.
func (f *fleetPlacer) Place(a tasks.Automation, agent string) placement.Result {
	type found struct {
		mc   *machines.Machine
		snap *herdr.Snapshot
		dirs []workspaceDir
	}
	var connected []found
	for _, mc := range f.fleet.List() {
		if f.fleet.Server() && mc.ID() == machines.LocalID {
			continue // the page's host runs no agents
		}
		if state, _ := mc.State(); state != machines.StateConnected {
			continue
		}
		snap, _ := f.watcher.Snapshot(mc.ID())
		dirs := checkoutDirs(snap)
		folders := make([]string, 0, len(dirs)+1)
		if mc.ID() == a.Machine {
			folders = append(folders, a.Dir) // the repository's own folder, read even with no workspace open on it
		}
		for _, d := range dirs {
			if d.dir != a.Dir || mc.ID() != a.Machine {
				folders = append(folders, d.dir)
			}
		}
		f.remotes.Refresh(mc, folders)
		connected = append(connected, found{mc, snap, dirs})
	}
	repository := f.remotes.Of(a.Machine, a.Dir)
	var candidates []placement.Machine
	for _, c := range connected {
		home := c.mc.ID() == a.Machine
		at := pickDir(c.dirs, home, a.Dir, repository, func(dir string) string { return f.remotes.Of(c.mc.ID(), dir) })
		if at == nil {
			continue
		}
		candidates = append(candidates, f.candidate(c.mc, c.snap, *at, home, agent))
	}
	return placement.Choose(candidates)
}

// pickDir is the folder of a machine's workspaces a card would branch off: the project's own folder on its home
// machine, else the first holding the same repository by its remote; nil when none does.
func pickDir(dirs []workspaceDir, home bool, homeDir, repository string, repositoryOf func(dir string) string) *workspaceDir {
	var first *workspaceDir
	for i := range dirs {
		if home && dirs[i].dir == homeDir {
			return &dirs[i]
		}
		if first == nil && repository != "" && repositoryOf(dirs[i].dir) == repository {
			first = &dirs[i]
		}
	}
	return first
}

// candidate is one machine as the scoring sees it.
func (f *fleetPlacer) candidate(mc *machines.Machine, snap *herdr.Snapshot, at workspaceDir, home bool, agent string) placement.Machine {
	m := placement.Machine{ID: mc.ID(), Label: mc.Label(), Dir: at.dir, Workspace: at.workspace, Home: home, Plan: -1, Working: awake.Working(snap)}
	if r := f.vitals.Report(mc.ID()); r != nil {
		m.PerCore, m.TempC = placement.LoadPerCore(r.Load1, r.Cores), r.TempC
		if r.Battery != nil {
			m.Battery = &placement.Battery{Percent: r.Battery.Percent, Discharging: r.Battery.Discharging}
		}
	}
	for _, w := range f.limits.Windows(mc.ID(), agent) {
		if w.Used > m.Plan {
			m.Plan = w.Used
		}
	}
	if hold, held := f.limits.Holding(mc.ID(), agent); held {
		m.Held = hold.Why
	}
	m.Budget = crossedBudget(f.budgets, mc.ID(), at.dir)
	return m
}

// workspaceDir is a workspace open on a folder, the kind a worktree branches off.
type workspaceDir struct{ dir, workspace string }

// checkoutDirs are the folders of a machine's workspaces that are not linked worktrees, each once, in herdr's order.
func checkoutDirs(snap *herdr.Snapshot) []workspaceDir {
	if snap == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []workspaceDir
	for i := range snap.Workspaces {
		ws := &snap.Workspaces[i]
		if ws.Worktree != nil && ws.Worktree.IsLinkedWorktree {
			continue
		}
		dir := snap.WorkspaceDir(ws)
		if dir == "" {
			continue
		}
		if dir = path.Clean(dir); seen[dir] {
			continue
		}
		seen[dir] = true
		out = append(out, workspaceDir{dir: dir, workspace: ws.WorkspaceID})
	}
	return out
}
