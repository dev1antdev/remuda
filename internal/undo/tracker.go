package undo

import (
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/shell"
)

// A turn's pace: a rest of Settle gets the end's checkpoint, a rest of Grace ends the turn (the pane at work again
// sooner is the same turn: herdr's blink through idle), and a turn going for MaxOpen is let go.
var (
	Settle    = 2 * time.Second
	Grace     = 5 * time.Second
	MaxOpen   = 12 * time.Hour
	TickEvery = 2 * time.Second
)

// Inputs are what the tracker reads through.
type Inputs struct {
	Host func(machineID string) (shell.Host, error) // a connected machine's shell
}

// going is one turn being watched.
type going struct {
	open      Open
	gen       int  // the pane's starts of work after a rest: an end read for an earlier rest is stale
	started   bool // the start's checkpoint is on the disk
	dead      bool // no checkpoint can be taken here (outside git, the machine away): watched until it rests, never kept
	restingAt time.Time
	endAsked  bool
	end       *Read
	endGen    int
	gone      bool // the pane closed: the turn ends on the end's checkpoint
	done      bool // finishing or finished
}

// Tracker takes a checkpoint of a folder when an agent pane there starts working and another when it comes to rest,
// and keeps the turn's delta in the Store. Nothing waits on a machine on the poll's path: every read runs in the
// machine's own reader goroutine, one script at a time, in order.
type Tracker struct {
	store *Store
	in    Inputs
	now   func() time.Time

	mu        sync.Mutex
	live      map[string]*going // by machine|pane
	pending   map[string]Open   // the turns the disk held at start, until their machine's first snapshot
	unwatched map[string]bool   // panes at work when their machine was first seen, until they rest: no turn begins in them
	queue     map[string][]job
	running   map[string]bool
	off       bool
	closed    bool
}

// NewTracker watches the turns into store; the turns going when remuda last stopped are picked up again when their
// pane is still at work.
func NewTracker(store *Store, in Inputs) *Tracker {
	t := &Tracker{store: store, in: in, now: time.Now, live: map[string]*going{}, pending: map[string]Open{}, unwatched: map[string]bool{}, queue: map[string][]job{}, running: map[string]bool{}}
	for _, open := range store.LoadOpen() {
		t.pending[key(open.Machine, open.Pane)] = open
	}
	return t
}

func key(machineID, paneID string) string { return machineID + "|" + paneID }

// Scratch turns the checkpoints off: a second remuda beside the real one reads nobody's work into a folder of its
// own; it still lists and undoes what its folder holds.
func (t *Tracker) Scratch() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.off = true
}

// Recording says whether this remuda takes the checkpoints, for the page to say why no turn is listed.
func (t *Tracker) Recording() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return !t.off
}

// Close stops taking checkpoints; the turns going stay on the disk for the next start.
func (t *Tracker) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
}

// Going says whether a turn is going, or still ending, in the folder: an undo then would land in it.
func (t *Tracker) Going(machineID, dir string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, turn := range t.live {
		if turn.open.Machine == machineID && turn.open.Dir == dir && !turn.dead {
			return true
		}
	}
	return false
}

// Observe hears every snapshot: an agent pane at work begins a turn (a question it waits on keeps it going), one at
// rest ends it, a pane gone ends it too. On a machine's first snapshot (first: or the first after its polls failed) a
// pane already at work begins none until it has rested: what it did before was not watched.
func (t *Tracker) Observe(machineID string, snap *herdr.Snapshot, at time.Time, first bool) {
	if snap == nil {
		return
	}
	t.mu.Lock()
	if t.off || t.closed {
		t.mu.Unlock()
		return
	}
	var jobs []job
	seen := map[string]bool{}
	for i := range snap.Panes {
		pane := &snap.Panes[i]
		if pane.Agent == "" {
			continue
		}
		ws := snap.Workspace(pane.WorkspaceID)
		if ws == nil {
			continue
		}
		dir := snap.WorkspaceDir(ws)
		if dir == "" {
			continue
		}
		k := key(machineID, pane.PaneID)
		seen[k] = true
		turn := t.live[k]
		atWork := pane.AgentStatus == herdr.StatusWorking || pane.AgentStatus == herdr.StatusBlocked
		if open, ok := t.pending[k]; ok {
			delete(t.pending, k)
			if turn == nil && atWork && open.Dir == dir && at.Sub(open.Started) < MaxOpen {
				turn = &going{open: open, started: true}
				t.live[k] = turn
			} else {
				t.store.DropOpen(open.ID)
			}
		}
		if turn == nil && atWork && first {
			t.unwatched[k] = true // at work before remuda saw the machine: its start was not watched, so it is no turn to undo
		}
		if !atWork {
			delete(t.unwatched, k)
		}
		switch {
		case pane.AgentStatus == herdr.StatusWorking && turn == nil && !t.unwatched[k]:
			turn = &going{open: Open{ID: NewID(at), Machine: machineID, Pane: pane.PaneID, Dir: dir, Agent: pane.DisplayAgent, Started: at}}
			if turn.open.Agent == "" {
				turn.open.Agent = pane.Agent
			}
			t.markShared(turn)
			t.live[k] = turn
			jobs = append(jobs, job{kind: startJob, turn: turn})
		case turn == nil:
		case atWork:
			if !turn.restingAt.IsZero() || turn.endAsked {
				turn.gen++
				turn.restingAt, turn.endAsked = time.Time{}, false
			}
		case turn.restingAt.IsZero():
			turn.restingAt = at
		}
	}
	for k, turn := range t.live {
		if turn.open.Machine == machineID && !seen[k] && !turn.gone {
			turn.gone = true
			if turn.restingAt.IsZero() {
				turn.restingAt = at
			}
		}
	}
	for k := range t.unwatched {
		if strings.HasPrefix(k, machineID+"|") && !seen[k] {
			delete(t.unwatched, k)
		}
	}
	for k, open := range t.pending {
		if open.Machine == machineID {
			delete(t.pending, k)
			t.store.DropOpen(open.ID)
		}
	}
	jobs = append(jobs, t.stepLocked(at)...)
	t.mu.Unlock()
	t.dispatch(jobs)
}

// markShared notes a turn that begins while another goes in the same folder: each one's changes are in the other too.
func (t *Tracker) markShared(turn *going) {
	for _, other := range t.live {
		if other.open.Machine == turn.open.Machine && other.open.Dir == turn.open.Dir && !other.dead {
			other.open.Shared, turn.open.Shared = true, true
		}
	}
}

// Forget drops a removed machine's turns.
func (t *Tracker) Forget(machineID string) {
	t.mu.Lock()
	for k, turn := range t.live {
		if turn.open.Machine == machineID {
			turn.done = true
			delete(t.live, k)
		}
	}
	t.mu.Unlock()
	t.store.Forget(machineID)
}

// Loop moves the turns on between snapshots — herdr's stream brings one only when something moved — until stop closes.
func (t *Tracker) Loop(stop <-chan struct{}) {
	ticker := time.NewTicker(TickEvery)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			t.mu.Lock()
			jobs := t.stepLocked(t.now())
			t.mu.Unlock()
			t.dispatch(jobs)
		}
	}
}

// stepLocked asks for the end's checkpoint of every turn at rest for Settle, and finishes every turn at rest for
// Grace whose end's checkpoint is in; t.mu held.
func (t *Tracker) stepLocked(at time.Time) []job {
	var jobs []job
	for k, turn := range t.live {
		if turn.done {
			continue
		}
		if at.Sub(turn.open.Started) >= MaxOpen && turn.restingAt.IsZero() {
			t.dropLocked(k, turn)
			continue
		}
		if turn.restingAt.IsZero() {
			continue
		}
		rested := at.Sub(turn.restingAt)
		if turn.dead {
			if turn.gone || rested >= Grace {
				t.dropLocked(k, turn)
			}
			continue
		}
		if !turn.endAsked && (turn.gone || rested >= Settle) {
			turn.endAsked = true
			jobs = append(jobs, job{kind: endJob, turn: turn, gen: turn.gen})
		}
		if turn.end != nil && turn.endGen == turn.gen && (turn.gone || rested >= Grace) {
			turn.done = true
			delete(t.live, k)
			jobs = append(jobs, job{kind: finishJob, turn: turn})
		}
	}
	return jobs
}

func (t *Tracker) dropLocked(k string, turn *going) {
	turn.done = true
	delete(t.live, k)
	t.store.DropOpen(turn.open.ID)
}
