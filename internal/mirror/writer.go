package mirror

import (
	"errors"
	"log"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// written is what remuda last wrote on a pane or a workspace, and when; a zero time is due again whatever it holds.
type written struct {
	figures Figures
	at      time.Time
}

// writer keeps one machine's herdr in step: the newest snapshot waits for it, one sync at a time, every write paced.
type writer struct {
	machine string
	svc     *Service
	wake    chan struct{}
	stop    chan struct{}
	once    sync.Once

	mu     sync.Mutex
	snap   *herdr.Snapshot
	snapAt time.Time

	// the loop's own
	panes      map[string]written
	workspaces map[string]written
	next       time.Time // no write before it
	refused    time.Time // herdr refused a write: none until then
	tasks      map[string]int64
	tasksAt    time.Time
	logged     time.Time
}

func newWriter(machine string, svc *Service) *writer {
	return &writer{machine: machine, svc: svc, wake: make(chan struct{}, 1), stop: make(chan struct{}),
		panes: map[string]written{}, workspaces: map[string]written{}}
}

// offer leaves the newest snapshot for the loop; a sync under way takes it next, an older one waiting is dropped.
func (w *writer) offer(snap *herdr.Snapshot, at time.Time) {
	w.mu.Lock()
	w.snap, w.snapAt = snap, at
	w.mu.Unlock()
	w.poke()
}

// poke has the loop sync again on the snapshot it has: the switches moved.
func (w *writer) poke() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *writer) end() { w.once.Do(func() { close(w.stop) }) }

func (w *writer) run() {
	for {
		select {
		case <-w.stop:
			return
		case <-w.wake:
		}
		w.mu.Lock()
		snap, at := w.snap, w.snapAt
		w.mu.Unlock()
		if snap != nil {
			w.sync(snap, at)
		}
	}
}

// sync writes what moved on every pane and workspace of the snapshot, clears what no longer stands, and forgets what
// herdr no longer has. With the figures off everything remuda wrote is cleared, what the snapshot shows of it too.
func (w *writer) sync(snap *herdr.Snapshot, at time.Time) {
	now := w.svc.now()
	if now.Before(w.refused) || w.svc.in.Herdr == nil {
		return
	}
	client, ok := w.svc.in.Herdr(w.machine)
	if !ok {
		return
	}
	var panes, spaces map[string]Figures
	if w.svc.Settings().Figures {
		panes, spaces = w.svc.in.figures(w.machine, snap, w.liveTasks(now))
	}
	seen := map[string]bool{}
	for i := range snap.Panes {
		pane := &snap.Panes[i]
		seen[pane.PaneID] = true
		if !w.keep(w.panes, pane.PaneID, panes[pane.PaneID], pane.Tokens, PaneTokens, at, func(want, have Figures) error {
			return client.PaneReportMetadata(paneReport(pane.PaneID, want, have))
		}) {
			return
		}
	}
	for i := range snap.Workspaces {
		ws := &snap.Workspaces[i]
		seen[ws.WorkspaceID] = true
		if !w.keep(w.workspaces, ws.WorkspaceID, spaces[ws.WorkspaceID], ws.Tokens, WorkspaceTokens, at, func(want, have Figures) error {
			return client.WorkspaceReportMetadata(herdr.WorkspaceMetadata{Workspace: ws.WorkspaceID, Source: Source, Tokens: patch(want.Tokens, have.Tokens), TTL: ttlFor(want)})
		}) {
			return
		}
	}
	for _, entries := range []map[string]written{w.panes, w.workspaces} {
		for id := range entries {
			if !seen[id] {
				delete(entries, id)
			}
		}
	}
}

// keep writes one pane's or workspace's figures when they moved or are due again, and notes what herdr holds now;
// false stops the sync: herdr is out of reach, or refused.
func (w *writer) keep(entries map[string]written, id string, want Figures, held map[string]string, names []string, snapAt time.Time, write func(want, have Figures) error) bool {
	have := w.known(entries, id, held, names, snapAt)
	if !due(want, have, w.svc.now()) {
		return true
	}
	w.pace()
	err := write(want, have.figures)
	var refusal *herdr.Error
	switch {
	case err == nil:
		if want.empty() {
			delete(entries, id)
		} else {
			entries[id] = written{figures: want, at: w.svc.now()}
		}
		return true
	case errors.As(err, &refusal) && strings.HasSuffix(refusal.Code, "_not_found"):
		delete(entries, id) // closed since the snapshot
		return true
	case errors.As(err, &refusal):
		w.refused = w.svc.now().Add(refusedPause) // an older herdr without the method, or a value it will not take
	}
	w.note(err)
	return false
}

// known is what remuda last wrote there, due again when a snapshot read well after the write lacks a value of it (herdr
// restarted, or the TTL ran out); a place first seen in this run starts from remuda's own tokens herdr still holds.
func (w *writer) known(entries map[string]written, id string, held map[string]string, names []string, snapAt time.Time) written {
	if have, ok := entries[id]; ok {
		if snapAt.After(have.at.Add(staleAfter)) {
			for name := range have.figures.Tokens {
				if _, ok := held[name]; !ok {
					have.at = time.Time{}
					break
				}
			}
		}
		return have
	}
	seeded := map[string]string{}
	for _, name := range names {
		if value, ok := held[name]; ok {
			seeded[name] = value
		}
	}
	return written{figures: Figures{Tokens: seeded}}
}

// due says whether a place needs a write: its figures moved, or the ones it carries are old enough to refresh.
func due(want Figures, have written, now time.Time) bool {
	if !maps.Equal(want.Tokens, have.figures.Tokens) || !maps.Equal(want.Labels, have.figures.Labels) {
		return true
	}
	return !want.empty() && (have.at.IsZero() || now.Sub(have.at) >= Refresh)
}

// paneReport is one pane's write: every token wanted and a clear for each one no longer, the labels replaced whole
// (herdr keeps one set per source) or cleared, and the TTL on all of it.
func paneReport(pane string, want, have Figures) herdr.PaneMetadata {
	report := herdr.PaneMetadata{Pane: pane, Source: Source, Tokens: patch(want.Tokens, have.Tokens), TTL: ttlFor(want)}
	switch {
	case len(want.Labels) > 0:
		report.Labels = want.Labels
	case len(have.Labels) > 0:
		report.ClearLabels = true
	}
	return report
}

// patch sets every value wanted and clears every name written before that is no longer.
func patch(want, have map[string]string) map[string]*string {
	out := map[string]*string{}
	for name, value := range want {
		out[name] = &value
	}
	for name := range have {
		if _, ok := want[name]; !ok {
			out[name] = nil
		}
	}
	return out
}

func ttlFor(want Figures) time.Duration {
	if want.empty() {
		return 0
	}
	return TTL
}

// pace waits for the write's turn: at most WritesPerSecond a second on one machine.
func (w *writer) pace() {
	now := w.svc.now()
	if w.next.After(now) {
		w.svc.sleep(w.next.Sub(now))
		now = w.next
	}
	w.next = now.Add(time.Second / WritesPerSecond)
}

// liveTasks is the machine's tasks by pane, read again at most every tasksEvery.
func (w *writer) liveTasks(now time.Time) map[string]int64 {
	if w.svc.in.Tasks == nil {
		return nil
	}
	if w.tasks == nil || now.Sub(w.tasksAt) >= tasksEvery {
		if tasks, err := w.svc.in.Tasks(w.machine); err == nil {
			w.tasks, w.tasksAt = tasks, now
		}
	}
	return w.tasks
}

// note logs a failed write once an hour per machine.
func (w *writer) note(err error) {
	if now := w.svc.now(); now.Sub(w.logged) >= time.Hour {
		w.logged = now
		log.Printf("mirror: herdr on %s took no figures: %v", w.machine, err)
	}
}
