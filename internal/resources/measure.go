package resources

import (
	"sort"
	"time"
)

// Root is a herdr pane and the shell its terminal runs: the root of every process started in it.
type Root struct {
	Pane, Workspace string
	PID             int
}

// Top is the process that holds the most memory of a tree.
type Top struct {
	Name  string `json:"name"`
	RSSKB int64  `json:"rss_kb"`
}

// Weight is what a tree of processes weighs: CPU as a share of all the machine's cores, resident memory, and its
// heaviest process.
type Weight struct {
	CPU   float64 `json:"cpu"`
	RSSKB int64   `json:"rss_kb"`
	Top   *Top    `json:"top,omitempty"`
}

// add sums another weight in, keeping the heavier top
func (w *Weight) add(o Weight) {
	w.CPU += o.CPU
	w.RSSKB += o.RSSKB
	if o.Top != nil && (w.Top == nil || o.Top.RSSKB > w.Top.RSSKB) {
		top := *o.Top
		w.Top = &top
	}
}

// Terminal is one pane's tree.
type Terminal struct {
	Pane string `json:"pane"`
	Weight
}

// Workspace is a workspace's panes summed, and each of them, the heaviest first.
type Workspace struct {
	ID string `json:"id"`
	Weight
	Terminals []Terminal `json:"terminals"`
}

// Usage is what a machine's herdr panes weigh on it at one moment: every workspace by memory, the herdr server's own
// processes beside them, and their sum against the machine's cores and memory.
type Usage struct {
	At         time.Time   `json:"at"`
	Cores      int         `json:"cores"`
	MemKB      int64       `json:"mem_kb"`
	Total      Weight      `json:"total"`
	CPUKnown   bool        `json:"cpu_known"` // false when a Linux machine's ticks had no reading before to be told against
	Workspaces []Workspace `json:"workspaces"`
	Herdr      Weight      `json:"herdr"`            // the herdr server and its attaches, past the panes
	Unread     int         `json:"unread,omitempty"` // panes whose shell herdr did not name, or is gone
}

// Measure sums every pane's tree of processes into its workspace, each process counted once, then the herdr
// server's own; cpu is a process's CPU as percent of one core.
func Measure(table Table, cpu func(Proc) float64, roots []Root, cpuKnown bool, at time.Time) Usage {
	sort.Slice(roots, func(i, j int) bool {
		if roots[i].Workspace != roots[j].Workspace {
			return roots[i].Workspace < roots[j].Workspace
		}
		return roots[i].Pane < roots[j].Pane
	})
	claimed := map[int]bool{}
	byID := map[string]*Workspace{}
	var order []string
	usage := Usage{At: at, Cores: table.Cores, MemKB: table.MemKB, CPUKnown: cpuKnown, Workspaces: []Workspace{}}
	for _, root := range roots {
		if _, ok := table.Procs[root.PID]; !ok || root.PID <= 1 {
			usage.Unread++
			continue
		}
		weight := sumTree(root.PID, table, claimed, cpu)
		ws := byID[root.Workspace]
		if ws == nil {
			ws = &Workspace{ID: root.Workspace}
			byID[root.Workspace] = ws
			order = append(order, root.Workspace)
		}
		ws.add(weight)
		ws.Terminals = append(ws.Terminals, Terminal{Pane: root.Pane, Weight: weight})
	}
	pids := make([]int, 0, len(table.Procs))
	for pid := range table.Procs {
		pids = append(pids, pid)
	}
	sort.Ints(pids)
	for _, pid := range pids {
		if p := table.Procs[pid]; p.Name == "herdr" || p.Name == "herdr.exe" {
			usage.Herdr.add(sumTree(pid, table, claimed, cpu))
		}
	}
	share := func(w *Weight) { w.CPU /= float64(table.Cores) }
	share(&usage.Herdr)
	for _, id := range order {
		ws := byID[id]
		share(&ws.Weight)
		for i := range ws.Terminals {
			share(&ws.Terminals[i].Weight)
		}
		sort.SliceStable(ws.Terminals, func(i, j int) bool { return ws.Terminals[i].RSSKB > ws.Terminals[j].RSSKB })
		usage.Workspaces = append(usage.Workspaces, *ws)
		usage.Total.add(Weight{CPU: ws.CPU, RSSKB: ws.RSSKB})
	}
	usage.Total.add(Weight{CPU: usage.Herdr.CPU, RSSKB: usage.Herdr.RSSKB})
	sort.SliceStable(usage.Workspaces, func(i, j int) bool { return usage.Workspaces[i].RSSKB > usage.Workspaces[j].RSSKB })
	return usage
}

// sumTree weighs the tree from root, skipping the processes another tree claimed already
func sumTree(root int, table Table, claimed map[int]bool, cpu func(Proc) float64) Weight {
	var w Weight
	stack := []int{root}
	for len(stack) > 0 {
		pid := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		p, ok := table.Procs[pid]
		if !ok || claimed[pid] {
			continue
		}
		claimed[pid] = true
		w.CPU += cpu(p)
		w.RSSKB += p.RSSKB
		if w.Top == nil || p.RSSKB > w.Top.RSSKB {
			w.Top = &Top{Name: p.Name, RSSKB: p.RSSKB}
		}
		stack = append(stack, table.Kids[pid]...)
	}
	return w
}
