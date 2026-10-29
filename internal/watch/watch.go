// Package watch polls every machine's session snapshot, browser or not,
// and turns what it sees into events. It is the only poller: handlers read
// its cache and never ask herdr inline.
package watch

import (
	"log"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
)

const (
	localEvery     = 2 * time.Second
	remoteEvery    = 4 * time.Second
	discoverEvery  = 5 * time.Second
	syncHerdrEvery = 30 * time.Second
)

type paneState struct {
	status string
	since  time.Time
}

// localDownAfter is how many failed polls in a row mean the local herdr is really gone.
const localDownAfter = 4

type machineState struct {
	snap    *herdr.Snapshot
	at      time.Time
	err     error
	fails   int
	panes   map[string]paneState
	started bool
	pollMu  sync.Mutex // one poll of this machine at a time: a snapshot never lands after a newer one
}

type Watcher struct {
	machines *machines.Manager
	bus      *events.Bus

	mu     sync.Mutex
	states map[string]*machineState
}

func New(m *machines.Manager, bus *events.Bus) *Watcher {
	return &Watcher{machines: m, bus: bus, states: map[string]*machineState{}}
}

// Start polls forever; new machines are picked up on each discovery round.
func (w *Watcher) Start() {
	go func() {
		lastSync := time.Time{}
		for {
			if time.Since(lastSync) >= syncHerdrEvery {
				w.machines.SyncHerdr()
				lastSync = time.Now()
			}
			for _, mc := range w.machines.List() {
				w.mu.Lock()
				st := w.stateFor(mc.ID())
				started := st.started
				st.started = true
				w.mu.Unlock()
				if !started {
					go w.loop(mc)
				}
			}
			time.Sleep(discoverEvery)
		}
	}()
}

// stateFor must be called with w.mu held.
func (w *Watcher) stateFor(id string) *machineState {
	st := w.states[id]
	if st == nil {
		st = &machineState{panes: map[string]paneState{}}
		w.states[id] = st
	}
	return st
}

func (w *Watcher) loop(mc *machines.Machine) {
	every := remoteEvery
	if mc.Local() {
		every = localEvery
	}
	for !mc.Removed() {
		w.poll(mc)
		time.Sleep(every)
	}
	// a removed machine: forget its state so re-adding the host starts clean
	w.mu.Lock()
	delete(w.states, mc.ID())
	w.mu.Unlock()
	events.Publish(w.bus, events.MachineRemoved{Machine: mc.ID()})
}

// poll fetches one snapshot and applies it; transport failures reach the machine through its client.
func (w *Watcher) poll(mc *machines.Machine) (*herdr.Snapshot, error) {
	w.mu.Lock()
	st := w.stateFor(mc.ID())
	w.mu.Unlock()
	st.pollMu.Lock() // the loop and Refresh may overlap; the RPC and its apply stay in order
	defer st.pollMu.Unlock()
	client, _, err := mc.Ready()
	if err == nil {
		var snap *herdr.Snapshot
		if snap, err = client.SessionSnapshot(); err == nil {
			w.mu.Lock()
			st.fails = 0
			w.mu.Unlock()
			w.apply(mc.ID(), snap)
			return snap, nil
		}
	}
	w.mu.Lock()
	st.err = err
	st.fails++
	fails := st.fails
	w.mu.Unlock()
	// herdr's own restart or update handoff unbinds the socket for a moment: a few misses first
	if mc.Local() && fails >= localDownAfter {
		go events.Publish(w.bus, events.LocalHerdrDown{Err: err})
	}
	return nil, err
}

// apply stores the snapshot, publishes it, and emits one event per pane whose status changed;
// a machine's first snapshot only seeds state, so a reconnect replays no alerts.
func (w *Watcher) apply(machineID string, snap *herdr.Snapshot) {
	now := time.Now()
	w.mu.Lock()
	st := w.stateFor(machineID)
	first := st.snap == nil || st.err != nil
	w.mu.Unlock()
	events.Publish(w.bus, events.Snapshot{Machine: machineID, Snap: snap, At: now, First: first})

	var changes []events.PaneStatus
	w.mu.Lock()
	seed := st.snap == nil
	back := st.err != nil // the first good poll after an outage: the gap is not a turn
	st.snap, st.at, st.err = snap, now, nil
	seen := map[string]bool{}
	for i := range snap.Panes {
		pane := &snap.Panes[i]
		seen[pane.PaneID] = true
		prev, known := st.panes[pane.PaneID]
		if !known {
			st.panes[pane.PaneID] = paneState{status: pane.AgentStatus, since: now}
			continue
		}
		if prev.status == pane.AgentStatus {
			if back {
				st.panes[pane.PaneID] = paneState{status: prev.status, since: now}
			}
			continue
		}
		st.panes[pane.PaneID] = paneState{status: pane.AgentStatus, since: now}
		if seed {
			continue
		}
		worked := time.Duration(0)
		if prev.status == herdr.StatusWorking {
			worked = now.Sub(prev.since)
		}
		changes = append(changes, events.PaneStatus{
			Pane: events.PaneRef{Machine: machineID, Pane: pane.PaneID}, Workspace: snap.WorkspaceLabel(pane.WorkspaceID),
			PaneTitle: pane.DisplayTitle(), Agent: pane.AgentName(), Kind: pane.Agent,
			From: prev.status, To: pane.AgentStatus, Worked: worked,
		})
	}
	for id := range st.panes {
		if !seen[id] {
			delete(st.panes, id)
			// a closed pane cancels whatever was pending for it
			changes = append(changes, events.PaneStatus{Pane: events.PaneRef{Machine: machineID, Pane: id}, To: events.Gone})
		}
	}
	w.mu.Unlock()

	for _, change := range changes {
		if change.To != events.Gone {
			log.Printf("status %s (%s): %s → %s", change.Pane, change.Workspace, change.From, change.To)
		}
		events.Publish(w.bus, change)
	}
}

// Snapshot returns the machine's latest snapshot, or the error its last poll hit.
func (w *Watcher) Snapshot(machineID string) (*herdr.Snapshot, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	st := w.states[machineID]
	if st == nil {
		return nil, errNotWatched
	}
	if st.err != nil {
		return st.snap, st.err
	}
	if st.snap == nil {
		return nil, errNotWatched
	}
	return st.snap, nil
}

type watchErr string

func (e watchErr) Error() string { return string(e) }

const errNotWatched = watchErr("no snapshot yet")

// Refresh polls one machine now, so the next roster read sees what the UI just changed.
func (w *Watcher) Refresh(mc *machines.Machine) (*herdr.Snapshot, error) {
	return w.poll(mc)
}
