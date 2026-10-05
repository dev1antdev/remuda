// Package watch keeps a fresh session snapshot per machine and turns pane
// status changes into events. It runs whether or not a browser is open —
// that is what makes an alert reach a phone with the app closed — and it is
// the only place snapshots come from, so handlers never poll herdr inline.
package watch

import (
	"log"
	"sync"
	"time"

	"github.com/dev1antdev/herdr-ui/internal/herdr"
	"github.com/dev1antdev/herdr-ui/internal/machines"
	"github.com/dev1antdev/herdr-ui/internal/push"
)

const (
	localEvery     = 2 * time.Second
	remoteEvery    = 4 * time.Second
	discoverEvery  = 5 * time.Second // new machines (incl. herdr's list) picked up this often
	syncHerdrEvery = 30 * time.Second
)

type paneState struct {
	status string
	since  time.Time // when this status began
}

type machineState struct {
	snap    *herdr.Snapshot
	at      time.Time
	err     error
	panes   map[string]paneState
	started bool
}

type Watcher struct {
	machines *machines.Manager
	push     *push.Service

	mu     sync.Mutex
	states map[string]*machineState
}

func New(m *machines.Manager, p *push.Service) *Watcher {
	return &Watcher{machines: m, push: p, states: map[string]*machineState{}}
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
				st := w.stateFor(mc.Cfg.ID)
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
	if mc.Local {
		every = localEvery
	}
	for !mc.Removed() {
		w.poll(mc)
		time.Sleep(every)
	}
	// a removed machine: forget its state so re-adding the host starts clean
	w.mu.Lock()
	delete(w.states, mc.Cfg.ID)
	w.mu.Unlock()
}

// poll fetches one snapshot and applies it; the machine's client reports
// transport failures to the machine itself.
func (w *Watcher) poll(mc *machines.Machine) (*herdr.Snapshot, error) {
	client, _, err := mc.Ready()
	if err == nil {
		var snap *herdr.Snapshot
		if snap, err = client.SessionSnapshot(); err == nil {
			w.apply(mc.Cfg.ID, snap)
			return snap, nil
		}
	}
	w.mu.Lock()
	w.stateFor(mc.Cfg.ID).err = err
	w.mu.Unlock()
	return nil, err
}

// apply stores the snapshot and emits one event per pane whose status
// changed since the last poll. The first snapshot of a machine only seeds
// state — reconnecting must not replay a day of alerts.
func (w *Watcher) apply(machineID string, snap *herdr.Snapshot) {
	now := time.Now()
	labels := map[string]string{}
	for _, ws := range snap.Workspaces {
		labels[ws.WorkspaceID] = ws.Label
	}

	var events []push.Event
	w.mu.Lock()
	st := w.stateFor(machineID)
	seed := st.snap == nil
	st.snap, st.at, st.err = snap, now, nil
	seen := map[string]bool{}
	for i := range snap.Panes {
		p := &snap.Panes[i]
		seen[p.PaneID] = true
		prev, known := st.panes[p.PaneID]
		if !known {
			st.panes[p.PaneID] = paneState{status: p.AgentStatus, since: now}
			continue
		}
		if prev.status == p.AgentStatus {
			continue
		}
		st.panes[p.PaneID] = paneState{status: p.AgentStatus, since: now}
		if seed {
			continue
		}
		worked := time.Duration(0)
		if prev.status == herdr.StatusWorking {
			worked = now.Sub(prev.since)
		}
		events = append(events, push.Event{
			Pane: push.PaneRef{Machine: machineID, Pane: p.PaneID}, Workspace: labels[p.WorkspaceID],
			PaneTitle: p.DisplayTitle(), Agent: p.AgentName(),
			From: prev.status, To: p.AgentStatus, Worked: worked,
		})
	}
	for id := range st.panes {
		if !seen[id] {
			delete(st.panes, id)
			// a closed pane cancels whatever was pending for it
			events = append(events, push.Event{Pane: push.PaneRef{Machine: machineID, Pane: id}, To: "gone"})
		}
	}
	w.mu.Unlock()

	for _, ev := range events {
		if ev.To != "gone" {
			log.Printf("status %s (%s): %s → %s", ev.Pane, ev.Workspace, ev.From, ev.To)
		}
		w.push.OnStatus(ev)
	}
}

// Snapshot returns the machine's latest snapshot, or the error its last poll
// hit. The watcher is the only poller: a handler never asks herdr itself.
func (w *Watcher) Snapshot(machineID string) (*herdr.Snapshot, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	st := w.states[machineID]
	if st == nil {
		return nil, errNotWatched
	}
	if st.snap != nil && st.err == nil {
		return st.snap, nil
	}
	if st.err != nil {
		return st.snap, st.err
	}
	return nil, errNotWatched
}

type watchErr string

func (e watchErr) Error() string { return string(e) }

const errNotWatched = watchErr("no snapshot yet")

// Refresh polls one machine now (after the UI created or closed something)
// so the next roster read sees it.
func (w *Watcher) Refresh(mc *machines.Machine) (*herdr.Snapshot, error) {
	return w.poll(mc)
}
