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
	// QuickEvery is the pace of the machine the status bar asks about.
	QuickEvery = 5 * time.Second
	// WatchedFor is how long after the roster was last asked for the reads go on.
	WatchedFor = time.Minute
	// QuickFor is how long after the status bar last asked about a machine its quicker pace holds.
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

// Reading is a machine's ports as last read — At is when they were — and why the last read failed when it did: the
// ports before it stand, as old as they are.
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
	hurried    map[string]time.Time // by machine: the status bar shows one machine's ports, the others keep the roster's pace
	complained map[string]time.Time
	reads      sync.WaitGroup // the reads under way, OnChange included
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
	return &Tracker{targets: targets, skip: skip, now: time.Now, machines: map[string]*machine{}, hurried: map[string]time.Time{}, complained: map[string]time.Time{}}
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

// Hurry says the status bar shows the machine's ports: that machine's reads go at QuickEvery for QuickFor, the
// others' at the roster's pace.
func (t *Tracker) Hurry(machineID string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	now := t.now()
	t.touched, t.hurried[machineID] = now, now
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

// Of is the machine's whole reading, its ports outside every workspace too; a machine never read has none, and one
// whose reading is older than StaleAfter counts as not read, as the roster then carries none of its ports.
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
	if t.now().Sub(m.reading.At) > StaleAfter {
		return Reading{Ports: []Port{}, Error: m.reading.Error}, false
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
	delete(t.hurried, machineID)
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
		every := Every
		if at, ok := t.hurried[target.ID]; ok && now.Sub(at) < QuickFor {
			every = QuickEvery
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
	for id, at := range t.hurried {
		if now.Sub(at) >= QuickFor {
			delete(t.hurried, id)
		}
	}
	t.reads.Add(len(due))
	t.mu.Unlock()
	for _, target := range due {
		go t.read(target)
	}
}

// read looks at one machine's listeners — a known process's folder not looked up again — and keeps them, telling
// OnChange when they moved; a failed read keeps the ports before it, and when they were read, and is logged at most
// every ComplainEvery
func (t *Tracker) read(target Target) {
	defer t.reads.Done()
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
		m.reading.Error = err.Error()
		if m.reading.Ports == nil {
			m.reading.Ports = []Port{}
		}
		complain = now.Sub(t.complained[target.ID]) >= ComplainEvery
		if complain {
			t.complained[target.ID] = now
		}
	} else {
		// a stale reading put none of its ports on the roster, so ports that did not move are news there all the same
		changed = m.reading.Error != "" || !slices.Equal(m.reading.Ports, list) || now.Sub(m.reading.At) > StaleAfter
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
