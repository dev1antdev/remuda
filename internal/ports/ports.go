// Package ports keeps the live ports of every connected machine's workspaces, for the roster and the status bar: the
// listeners internal/preview finds, matched to the workspace whose folder holds their process, read every so often
// while a page reads the roster and quicker while the status bar shows — a server's folder looked up once, while it
// runs — and a port's process stopped at the person's word.
package ports

import (
	"log"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/preview"
	"github.com/dev1antdev/remuda/internal/shell"
)

var (
	// Every is the pace of the reads while a page reads the roster.
	Every = 15 * time.Second
	// QuickEvery is the pace while the status bar asks.
	QuickEvery = 5 * time.Second
	// WatchedFor is how long after the roster was last asked for the reads go on.
	WatchedFor = time.Minute
	// QuickFor is how long after the status bar last asked the quicker pace holds.
	QuickFor = 20 * time.Second
	// StaleAfter is how old a reading may be for the roster to carry it: once nobody looked for a while it says nothing.
	StaleAfter = 2 * time.Minute
	// TickEvery is how often the loop looks for a machine whose read is due.
	TickEvery = time.Second
	// ComplainEvery spaces the log lines about a machine whose ports cannot be read.
	ComplainEvery = time.Hour
)

// Port is one port a process of the person's listens on, as the roster carries it: its process's name, what it serves
// (vite, next…) when its arguments say, its pid, and where it listens as a person writes it — localhost:5173, *:8080.
type Port struct {
	Port      int    `json:"port"`
	Process   string `json:"process"`
	Label     string `json:"label,omitempty"`
	PID       int    `json:"pid"`
	Address   string `json:"address"`
	Workspace string `json:"workspace,omitempty"` // the workspace whose folder holds it, "" for one outside them
}

// Reading is a machine's ports as last read, and why the last read failed when it did: the ports before it stand.
type Reading struct {
	At    time.Time `json:"at"`
	Ports []Port    `json:"ports"`
	Error string    `json:"error,omitempty"`
}

// Target is a machine whose ports are read: its shell and its workspaces' folders by id.
type Target struct {
	ID      string
	Host    shell.Shell
	Folders map[string]string
}

// Tracker reads each target's ports one read at a time per machine, only while a page looks.
type Tracker struct {
	targets func() []Target
	skip    func() map[int]bool
	now     func() time.Time
	// OnChange hears that a machine's ports moved, so the page's roster moves; the app sets it.
	OnChange func()

	mu         sync.Mutex
	machines   map[string]*machine
	touched    time.Time
	hurried    time.Time
	complained map[string]time.Time
}

// machine is one machine's reading, what its listeners' processes were found to be, and whether a read runs
type machine struct {
	reading Reading
	readAt  time.Time
	busy    bool
	seen    map[int]preview.Seen // the read's own: one read runs at a time
}

// New is a tracker over the targets the app lists, never offering the ports skip names.
func New(targets func() []Target, skip func() map[int]bool) *Tracker {
	return &Tracker{targets: targets, skip: skip, now: time.Now, machines: map[string]*machine{}, complained: map[string]time.Time{}}
}

// Touch says the roster was just read: the reads go on for WatchedFor.
func (t *Tracker) Touch() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.touched = t.now()
	t.mu.Unlock()
}

// Hurry says the status bar shows: the reads go at QuickEvery for QuickFor.
func (t *Tracker) Hurry() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.touched, t.hurried = t.now(), t.now()
	t.mu.Unlock()
}

// For is a workspace's ports as last read, in the order of their numbers.
func (t *Tracker) For(machineID, workspaceID string) []Port {
	if t == nil || workspaceID == "" {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	m := t.machines[machineID]
	if m == nil || t.now().Sub(m.reading.At) > StaleAfter {
		return nil
	}
	var out []Port
	for _, p := range m.reading.Ports {
		if p.Workspace == workspaceID {
			p.Workspace = ""
			out = append(out, p)
		}
	}
	return out
}

// Of is the machine's whole reading, its ports outside every workspace too; a machine never read has none.
func (t *Tracker) Of(machineID string) (Reading, bool) {
	if t == nil {
		return Reading{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	m := t.machines[machineID]
	if m == nil || m.readAt.IsZero() {
		return Reading{Ports: []Port{}}, false
	}
	reading := m.reading
	reading.Ports = append([]Port{}, m.reading.Ports...)
	return reading, true
}

// Forget drops a removed machine.
func (t *Tracker) Forget(machineID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.machines, machineID)
	delete(t.complained, machineID)
}

// Again reads the machine's ports at the next tick, as a stop moved them.
func (t *Tracker) Again(machineID string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if m := t.machines[machineID]; m != nil {
		m.readAt = time.Time{}
	}
}

// Loop reads every target whose read is due, until done.
func (t *Tracker) Loop(done <-chan struct{}) {
	tick := time.NewTicker(TickEvery)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case <-tick.C:
			t.Sweep()
		}
	}
}

// Sweep starts a read of every target that is due, in a goroutine each, while a page looks; the machines no longer
// listed are dropped.
func (t *Tracker) Sweep() {
	now := t.now()
	t.mu.Lock()
	watched := !t.touched.IsZero() && now.Sub(t.touched) < WatchedFor
	every := Every
	if !t.hurried.IsZero() && now.Sub(t.hurried) < QuickFor {
		every = QuickEvery
	}
	t.mu.Unlock()
	if !watched {
		return
	}
	targets := t.targets()
	listed := map[string]bool{}
	var due []Target
	t.mu.Lock()
	for _, target := range targets {
		listed[target.ID] = true
		m := t.machines[target.ID]
		if m == nil {
			m = &machine{seen: map[int]preview.Seen{}}
			t.machines[target.ID] = m
		}
		if !m.busy && (m.readAt.IsZero() || now.Sub(m.readAt) >= every) {
			m.busy = true
			due = append(due, target)
		}
	}
	for id := range t.machines {
		if !listed[id] {
			delete(t.machines, id)
		}
	}
	t.mu.Unlock()
	for _, target := range due {
		go t.read(target)
	}
}

// read looks at one machine's listeners — a known process's folder not looked up again — and keeps them, telling
// OnChange when they moved; a failed read keeps the ports before it and is logged at most every ComplainEvery
func (t *Tracker) read(target Target) {
	t.mu.Lock()
	m := t.machines[target.ID]
	t.mu.Unlock()
	if m == nil {
		return
	}
	known := make([]int, 0, len(m.seen))
	for pid := range m.seen {
		known = append(known, pid)
	}
	slices.Sort(known)
	out, err := shell.Text(target.Host, preview.ScriptKnowing(known), preview.FindTimeout)
	failed := err != nil && strings.TrimSpace(out) == ""
	var list []Port
	if !failed {
		list = portsOf(preview.Match(preview.Parse(preview.Recall(out, m.seen), t.skip()), target.Folders))
	}
	now := t.now()
	t.mu.Lock()
	if t.machines[target.ID] != m {
		t.mu.Unlock()
		return
	}
	m.busy, m.readAt = false, now
	changed, complain := false, false
	if failed {
		changed = m.reading.Error == ""
		m.reading.At, m.reading.Error = now, err.Error()
		if m.reading.Ports == nil {
			m.reading.Ports = []Port{}
		}
		complain = now.Sub(t.complained[target.ID]) >= ComplainEvery
		if complain {
			t.complained[target.ID] = now
		}
	} else {
		changed = m.reading.Error != "" || !slices.Equal(m.reading.Ports, list)
		m.reading = Reading{At: now, Ports: list}
	}
	t.mu.Unlock()
	if complain {
		log.Printf("ports: %s: the listening ports cannot be read: %v", target.ID, err)
	}
	if changed && t.OnChange != nil {
		t.OnChange()
	}
}

// portsOf is the listeners as the roster carries them
func portsOf(list []preview.Listener) []Port {
	out := make([]Port, 0, len(list))
	for _, l := range list {
		out = append(out, Port{Port: l.Port, Process: l.Command, Label: l.Label, PID: l.PID, Address: AddressOf(l.Address, l.Port), Workspace: l.Workspace})
	}
	return out
}

// AddressOf is where a listener listens as a person writes it: localhost for a loopback, * for every address
func AddressOf(bound string, port int) string {
	host := bound
	switch bound {
	case "127.0.0.1", "::1", "localhost":
		host = "localhost"
	case "*", "::", "0.0.0.0", "":
		host = "*"
	default:
		if ip := net.ParseIP(bound); ip != nil && ip.To4() == nil {
			return net.JoinHostPort(bound, strconv.Itoa(port))
		}
	}
	return host + ":" + strconv.Itoa(port)
}
