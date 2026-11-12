// Package subagents keeps how many of every Claude Code agent's subagents work, sit quiet or are done in its live
// conversation, for the roster and so the Wall's zoo: read off the request path, one pane at a time per machine, and
// only while a page keeps reading the roster.
package subagents

import (
	"log"
	"sort"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/transcript"
)

var (
	// WorkingEvery is the least time between two reads of a pane herdr reports working, or one whose last count had a
	// subagent working: subagents run on while the parent rests.
	WorkingEvery = 10 * time.Second
	// RestingEvery is the least time between two reads of any other agent pane.
	RestingEvery = time.Minute
	// WatchedFor is how long after the roster was last asked for the reads go on.
	WatchedFor = time.Minute
	// ComplainEvery spaces the log lines about a machine whose subagents cannot be read.
	ComplainEvery = time.Hour
)

// Count is how many of a conversation's subagents stand in each state.
type Count struct {
	Working int `json:"working,omitempty"`
	Quiet   int `json:"quiet,omitempty"`
	Done    int `json:"done,omitempty"`
}

// CountOf sums a conversation's subagents up by their status.
func CountOf(list []transcript.Subagent) Count {
	var count Count
	for _, agent := range list {
		switch agent.Status {
		case transcript.SubagentWorking:
			count.Working++
		case transcript.SubagentQuiet:
			count.Quiet++
		case transcript.SubagentDone:
			count.Done++
		}
	}
	return count
}

// Read lists the subagents of a pane's live conversation as they stand at now; none when the pane has no conversation file.
type Read func(machineID, paneID string, now time.Time) ([]transcript.Subagent, error)

// Ready says whether a machine is connected, so its files can be read.
type Ready func(machineID string) bool

// pane is one Claude Code pane as the last snapshot showed it, and its count as last read.
type pane struct {
	working bool
	readAt  time.Time
	count   Count
}

// due says whether the pane's last read is old enough for another: WorkingEvery while it works or its last count had a
// subagent working, else RestingEvery — a quiet one may never finish, so it does not hold the quicker pace.
func (p *pane) due(now time.Time) bool {
	every := RestingEvery
	if p.working || p.count.Working > 0 {
		every = WorkingEvery
	}
	return p.readAt.IsZero() || now.Sub(p.readAt) >= every
}

// dueRead is a pane a sweep reads, the entry it read it for: a pane forgotten meanwhile is not written back.
type dueRead struct {
	ref  events.PaneRef
	pane *pane
}

// Tracker holds one count per Claude Code pane and reads one machine's panes at a time.
type Tracker struct {
	read  Read
	ready Ready
	now   func() time.Time
	// OnChange hears that a read moved a pane's count, so the page's roster moves; the app sets it.
	OnChange func()

	mu         sync.Mutex
	panes      map[events.PaneRef]*pane
	sweeping   map[string]bool
	touched    time.Time
	complained map[string]time.Time
}

func New(read Read, ready Ready) *Tracker {
	return &Tracker{read: read, ready: ready, now: time.Now, panes: map[events.PaneRef]*pane{}, sweeping: map[string]bool{}, complained: map[string]time.Time{}}
}

// Count is the pane's subagents by state as last read; nil when it has none or nothing was read yet.
func (t *Tracker) Count(machineID, paneID string) *Count {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	p := t.panes[events.PaneRef{Machine: machineID, Pane: paneID}]
	if p == nil || p.count == (Count{}) {
		return nil
	}
	count := p.count
	return &count
}

// Observe takes a machine's snapshot: its Claude Code panes and whether each works are noted, the panes gone or no
// longer Claude Code's forgotten, and while a page looks the panes due are read in the background.
func (t *Tracker) Observe(machineID string, snap *herdr.Snapshot) {
	if t == nil || snap == nil {
		return
	}
	claude := map[string]bool{}
	t.mu.Lock()
	for i := range snap.Panes {
		info := &snap.Panes[i]
		if transcript.FormatOfAgent(info.Agent) != transcript.Claude {
			continue
		}
		claude[info.PaneID] = true
		ref := events.PaneRef{Machine: machineID, Pane: info.PaneID}
		p := t.panes[ref]
		if p == nil {
			p = &pane{}
			t.panes[ref] = p
		}
		p.working = info.AgentStatus == herdr.StatusWorking
	}
	for ref := range t.panes {
		if ref.Machine == machineID && !claude[ref.Pane] {
			delete(t.panes, ref)
		}
	}
	watched := t.watched()
	t.mu.Unlock()
	if watched {
		t.sweep(machineID)
	}
}

// Touch says the roster was just read: the reads go on for WatchedFor, and every machine's panes that are due are
// read now, in the background. Nothing is read for a page nobody looks at.
func (t *Tracker) Touch() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.touched = t.now()
	machines := map[string]bool{}
	for ref := range t.panes {
		machines[ref.Machine] = true
	}
	t.mu.Unlock()
	for machineID := range machines {
		t.sweep(machineID)
	}
}

// OnStatus forgets a closed pane.
func (t *Tracker) OnStatus(ev events.PaneStatus) {
	if ev.To != events.Gone {
		return
	}
	t.mu.Lock()
	delete(t.panes, ev.Pane)
	t.mu.Unlock()
}

// Forget drops a removed machine's panes.
func (t *Tracker) Forget(machineID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for ref := range t.panes {
		if ref.Machine == machineID {
			delete(t.panes, ref)
		}
	}
	delete(t.complained, machineID)
}

// watched says whether a page asked for the roster within WatchedFor; the lock is held.
func (t *Tracker) watched() bool {
	return !t.touched.IsZero() && t.now().Sub(t.touched) < WatchedFor
}

// sweep reads the machine's due panes in a goroutine of its own, unless one already runs for it.
func (t *Tracker) sweep(machineID string) {
	now := t.now()
	t.mu.Lock()
	if t.sweeping[machineID] {
		t.mu.Unlock()
		return
	}
	var due []dueRead
	for ref, p := range t.panes {
		if ref.Machine == machineID && p.due(now) {
			due = append(due, dueRead{ref: ref, pane: p})
		}
	}
	if len(due) == 0 {
		t.mu.Unlock()
		return
	}
	t.sweeping[machineID] = true
	t.mu.Unlock()
	sort.Slice(due, func(i, j int) bool { return due[i].ref.Pane < due[j].ref.Pane })
	go t.run(machineID, due)
}

// run reads each due pane of a connected machine; a count that moved is kept and told to OnChange, a failed read
// leaves the last good count and is logged at most once per ComplainEvery for the machine.
func (t *Tracker) run(machineID string, due []dueRead) {
	defer func() {
		t.mu.Lock()
		delete(t.sweeping, machineID)
		t.mu.Unlock()
	}()
	if !t.ready(machineID) {
		return
	}
	for _, d := range due {
		now := t.now()
		list, err := t.read(machineID, d.ref.Pane, now)
		t.mu.Lock()
		if t.panes[d.ref] != d.pane {
			t.mu.Unlock()
			continue
		}
		d.pane.readAt = now
		changed, complain := false, false
		if err != nil {
			complain = now.Sub(t.complained[machineID]) >= ComplainEvery
			if complain {
				t.complained[machineID] = now
			}
		} else if count := CountOf(list); count != d.pane.count {
			d.pane.count = count
			changed = true
		}
		t.mu.Unlock()
		if complain {
			log.Printf("subagents: %s: the subagents of %s cannot be read: %v", machineID, d.ref.Pane, err)
		}
		if changed && t.OnChange != nil {
			t.OnChange()
		}
	}
}
