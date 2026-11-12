package drift

import (
	"sort"
	"time"

	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/updates"
)

// Machine is one column's inputs: the machine, its vitals, what the script read there, and what it offers to run.
type Machine struct {
	ID      string
	Label   string
	Away    bool   // not connected: nothing is read, the column says so
	Error   string // the script failed there: the vitals' rows still stand
	Report  *health.Report
	Facts   *Facts
	Offered []updates.Update
}

// Table is the fleet's setup, machines as columns and facts as rows.
type Table struct {
	At       time.Time `json:"at"`
	Machines []Column  `json:"machines"`
	Sections []Section `json:"sections"`
	Single   []Lone    `json:"single,omitempty"` // repositories checked out on one machine only: nothing to compare
}

// Column is a machine as the table names it.
type Column struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Away  bool   `json:"away,omitempty"`
	Error string `json:"error,omitempty"`
}

// Section is a group of rows: the agents, herdr, remuda, Claude Code's settings, the instructions, or one repository.
type Section struct {
	ID      string            `json:"id"`                // agents | herdr | remuda | claude | instructions | repo
	Name    string            `json:"name,omitempty"`    // a repository's: owner/repo, or its folder's name
	Folders map[string]string `json:"folders,omitempty"` // a repository's main checkout on each machine, by machine id
	Rows    []Row             `json:"rows"`
}

// Row is one fact across the machines: a cell per machine it applies to, the norm most of them share, and how it stands.
type Row struct {
	ID    string          `json:"id"`
	Arg   string          `json:"arg,omitempty"` // the agent, the plugin or the file the row is about
	Kind  string          `json:"kind"`          // version | via | flag | hook | plugin | model | hash | text | behind
	State string          `json:"state"`         // same | odd (one value most share, the rest marked) | split (no value most share) | single
	Norm  string          `json:"norm,omitempty"`
	Cells map[string]Cell `json:"cells"`          // by machine id; a machine with no cell is one the row does not apply to
	Hand  []string        `json:"hand,omitempty"` // the machines off the row's best with no update to bring them in line: theirs to do by hand
}

// Cell is a machine's value in a row; Mark is odd (differs from the norm) or split (the row has no norm).
type Cell struct {
	Value  string  `json:"value"`
	Mark   string  `json:"mark,omitempty"`
	At     int64   `json:"at,omitempty"`   // a file's last change, a checkout's last fetch
	Note   string  `json:"note,omitempty"` // the upstream a checkout is compared with
	Action *Action `json:"action,omitempty"`
}

// Action is the update remuda already has that brings the cell toward the row's best value, as the Updates dialog
// shows it on that machine: the command it runs, seen before Run.
type Action struct {
	What    string `json:"what"`
	Title   string `json:"title"`
	Command string `json:"command"`
}

// Lone is a repository checked out on one machine only.
type Lone struct {
	Name    string `json:"name"`
	Machine string `json:"machine"`
	Folder  string `json:"folder"`
}

// Build lays the machines out as the table, rows that hold nothing anywhere left out.
func Build(machines []Machine, at time.Time) Table {
	table := Table{At: at, Machines: []Column{}, Sections: []Section{}}
	for _, m := range machines {
		table.Machines = append(table.Machines, Column{ID: m.ID, Label: m.Label, Away: m.Away, Error: m.Error})
	}
	live := machines[:0:0]
	for _, m := range machines {
		if !m.Away {
			live = append(live, m)
		}
	}
	for _, section := range []Section{agentsSection(live), herdrSection(live), remudaSection(live), claudeSection(live), instructionsSection(live)} {
		if len(section.Rows) > 0 {
			table.Sections = append(table.Sections, section)
		}
	}
	repos, single := repoSections(live)
	table.Sections = append(table.Sections, repos...)
	table.Single = single
	return table
}

// spec is how a row is read: its cell on a machine (false where it does not apply), which of two values is better
// (nil: neither, the norm is what most share and a tie is a split), and the update that moves a cell toward the best.
type spec struct {
	id, arg, kind string
	cell          func(m Machine) (Cell, bool)
	better        func(a, b string) bool
	action        func(m Machine, value string) string // the update's what, "" for none
}

// build reads the row on every machine and settles it; false for a row that holds nothing anywhere.
func (s spec) build(machines []Machine) (Row, bool) {
	row := Row{ID: s.id, Arg: s.arg, Kind: s.kind, Cells: map[string]Cell{}}
	held := false
	for _, m := range machines {
		if c, ok := s.cell(m); ok {
			row.Cells[m.ID] = c
			held = held || c.Value != ""
		}
	}
	if !held {
		return Row{}, false
	}
	settle(&row, s.better)
	if row.State != "odd" && row.State != "split" {
		return row, true
	}
	best := bestOf(row, s.better)
	for _, m := range machines {
		c, ok := row.Cells[m.ID]
		if !ok {
			continue
		}
		worse := s.better != nil && s.better(best, c.Value)
		if worse && s.action != nil {
			if action := offered(m, s.action(m, c.Value)); action != nil {
				c.Action = action
				row.Cells[m.ID] = c
				continue
			}
		}
		if worse || (c.Mark != "" && (s.better == nil || c.Value != best)) {
			row.Hand = append(row.Hand, m.ID)
		}
	}
	return row, true
}

// settle finds the norm: the value most cells share; a tie goes to the better value where better can tell, else the
// row is split. Cells off the norm are marked odd, every cell of a split row split.
func settle(row *Row, better func(a, b string) bool) {
	counts := map[string]int{}
	for _, c := range row.Cells {
		counts[c.Value]++
	}
	if len(row.Cells) < 2 {
		row.State = "single"
		for _, c := range row.Cells {
			row.Norm = c.Value
		}
		return
	}
	top := 0
	for _, n := range counts {
		top = max(top, n)
	}
	var leaders []string
	for value, n := range counts {
		if n == top {
			leaders = append(leaders, value)
		}
	}
	sort.Strings(leaders)
	norm, ok := leaders[0], len(leaders) == 1
	if !ok && better != nil {
		for _, l := range leaders {
			if better(l, norm) {
				norm = l
			}
		}
		ok = true
		for _, l := range leaders {
			if l != norm && !better(norm, l) {
				ok = false
			}
		}
	}
	switch {
	case len(counts) == 1:
		row.State, row.Norm = "same", norm
		return
	case !ok:
		row.State = "split"
	default:
		row.State, row.Norm = "odd", norm
	}
	for id, c := range row.Cells {
		if row.State == "split" {
			c.Mark = "split"
		} else if c.Value != norm {
			c.Mark = "odd"
		}
		row.Cells[id] = c
	}
}

// bestOf is the best value of the row as better ranks them.
func bestOf(row Row, better func(a, b string) bool) string {
	var values []string
	for _, c := range row.Cells {
		values = append(values, c.Value)
	}
	sort.Strings(values)
	best := values[0]
	for _, v := range values {
		if better != nil && better(v, best) {
			best = v
		}
	}
	return best
}

// offered is the machine's update called what, when it offers one.
func offered(m Machine, what string) *Action {
	if what == "" {
		return nil
	}
	for _, u := range m.Offered {
		if u.What == what {
			return &Action{What: u.What, Title: u.Title, Command: u.Command}
		}
	}
	return nil
}

// newer ranks versions: an installed one over none, then the newer.
func newer(a, b string) bool {
	return a != "" && (b == "" || health.Newer(b, a))
}

// yes ranks a flag on over off.
func yes(a, b string) bool { return a == "yes" && b == "no" }

// present ranks something there over nothing.
func present(a, b string) bool { return a != "" && b == "" }

// hookRank ranks herdr's hooks: current, then outdated, then missing.
func hookRank(a, b string) bool {
	rank := map[string]int{"current": 3, "outdated": 2, "missing": 1}
	return rank[a] > rank[b]
}

// fewer ranks a checkout fewer commits behind its upstream first.
func fewer(a, b string) bool { return len(a) < len(b) || (len(a) == len(b) && a < b) }
