package resources

import (
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

var (
	// Fresh is how long a reading answers every asker as it is: two pages polling one machine read it once.
	Fresh = 4 * time.Second
	// ReadTimeout bounds one process table's read.
	ReadTimeout = 15 * time.Second
	// BaselineFor is how old a Linux machine's last ticks may be to tell the CPU against; older, the read samples twice.
	BaselineFor = 2 * time.Minute
	// FirstGap is the wait between the two samples of a read with no baseline.
	FirstGap = time.Second
	// AskAgainAfter is how long a pane whose shell herdr named but the table lacks waits before herdr is asked again.
	AskAgainAfter = 30 * time.Second
)

// Pane is a herdr pane of the machine as its last snapshot shows it.
type Pane struct{ ID, Workspace string }

// Inputs are the service's ways into the machines: their panes as the watcher last saw them, and herdr's word on a
// pane's shell.
type Inputs struct {
	Panes    func(machineID string) []Pane
	ShellPID func(machineID, paneID string) (int, error)
}

// Service reads the machines' loads, one read at a time per machine.
type Service struct {
	in  Inputs
	now func() time.Time

	mu       sync.Mutex
	machines map[string]*machine
}

// machine is one machine's last reading, its last ticks on Linux, and its panes' shells as herdr named them
type machine struct {
	mu      sync.Mutex // held for a whole read: a second asker waits for its answer
	last    *Usage
	ticks   map[int]float64
	ticksAt time.Time
	shells  map[string]shellOf
}

// shellOf is a pane's shell as herdr named it, and when it was asked
type shellOf struct {
	pid   int
	asked time.Time
}

func New(in Inputs) *Service {
	return &Service{in: in, now: time.Now, machines: map[string]*machine{}}
}

// Read is the machine's load now: the last reading while it is Fresh, else a new one. A second read of a machine while
// one runs waits for it.
func (s *Service) Read(machineID string, host shell.Shell) (Usage, error) {
	m := s.machineOf(machineID)
	m.mu.Lock()
	defer m.mu.Unlock()
	now := s.now()
	if m.last != nil && now.Sub(m.last.At) < Fresh {
		return *m.last, nil
	}
	table, err := readTable(host)
	if err != nil {
		return Usage{}, err
	}
	at := s.now()
	if table.Linux && (m.ticks == nil || at.Sub(m.ticksAt) > BaselineFor) {
		m.keepTicks(table, at)
		time.Sleep(FirstGap)
		if table, err = readTable(host); err != nil {
			return Usage{}, err
		}
		at = s.now()
	}
	cpu, known := m.cpuOf(table, at)
	usage := Measure(table, cpu, m.roots(s, machineID, table, at), known, at)
	m.last = &usage
	return usage, nil
}

// Forget drops a removed machine's readings.
func (s *Service) Forget(machineID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.machines, machineID)
}

func (s *Service) machineOf(id string) *machine {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.machines[id]
	if m == nil {
		m = &machine{shells: map[string]shellOf{}}
		s.machines[id] = m
	}
	return m
}

// readTable runs the script and reads its answer
func readTable(host shell.Shell) (Table, error) {
	out, err := shell.Text(host, Script(), ReadTimeout)
	if err != nil && strings.TrimSpace(out) == "" {
		return Table{}, err
	}
	return Parse(out)
}

// keepTicks notes a Linux table's ticks as the baseline of the next read
func (m *machine) keepTicks(table Table, at time.Time) {
	m.ticks = make(map[int]float64, len(table.Procs))
	for pid, p := range table.Procs {
		m.ticks[pid] = p.CPU
	}
	m.ticksAt = at
}

// cpuOf is how a process's CPU reads as percent of one core: a Mac's ps says it, a Linux process's ticks are told
// against the last reading's — a process born since spent all of its own in between — and kept for the next
func (m *machine) cpuOf(table Table, at time.Time) (func(Proc) float64, bool) {
	if !table.Linux {
		return func(p Proc) float64 { return p.CPU }, true
	}
	before, since := m.ticks, at.Sub(m.ticksAt).Seconds()
	m.keepTicks(table, at)
	if before == nil || since <= 0 {
		return func(Proc) float64 { return 0 }, false
	}
	perSecond := float64(table.Ticks) * since
	return func(p Proc) float64 {
		spent := p.CPU - before[p.PID]
		if spent < 0 {
			return 0
		}
		return spent / perSecond * 100
	}, true
}

// roots is every pane of the machine with its shell: the one herdr named before while the table still holds it, else
// herdr asked again — a pane whose shell the table lacks at most every AskAgainAfter; panes gone are forgotten
func (m *machine) roots(s *Service, machineID string, table Table, at time.Time) []Root {
	panes := s.in.Panes(machineID)
	live := map[string]bool{}
	roots := make([]Root, 0, len(panes))
	for _, pane := range panes {
		live[pane.ID] = true
		known, ok := m.shells[pane.ID]
		_, running := table.Procs[known.pid]
		if !ok || (!running && at.Sub(known.asked) >= AskAgainAfter) {
			if pid, err := s.in.ShellPID(machineID, pane.ID); err == nil {
				known = shellOf{pid: pid, asked: at}
			} else {
				known.asked = at
			}
			m.shells[pane.ID] = known
		}
		roots = append(roots, Root{Pane: pane.ID, Workspace: pane.Workspace, PID: known.pid})
	}
	for id := range m.shells {
		if !live[id] {
			delete(m.shells, id)
		}
	}
	return roots
}
