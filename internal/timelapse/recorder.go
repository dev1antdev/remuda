package timelapse

import (
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// take is one turn being recorded: its frames in memory until the turn ends.
type take struct {
	meta      Meta
	frames    []frame
	held      int           // bytes of the frames' rows, a row shared between frames counted once
	every     time.Duration // the pace, Interval doubled at every thinning
	askedAt   time.Time     // when the last read was asked for
	queued    bool          // a read of the screen waits in the machine's queue
	restingAt time.Time     // when the pane stopped working; zero while it works
	restedAs  string        // the status it rests in
	seenAt    time.Time     // when the last snapshot that showed the pane was taken
	savedAt   time.Time     // the last checkpoint, else the start
	written   bool          // the recording is on the disk, or dropped: nothing more lands in it
	writing   sync.Mutex    // one write of the take at a time, taken before the recorder's own lock
}

type jobKind int

const (
	sampleJob jobKind = iota
	checkpointJob
	finishJob
)

// job is one thing a machine's reader does for a take: read its screen, write what it has so far, or write it whole.
type job struct {
	kind   jobKind
	take   *take
	reason string // a finish's: why the turn ended
}

func key(machineID, paneID string) string { return machineID + "|" + paneID }

// Observe hears every snapshot: a working agent pane is recorded — a read of its screen queued when one is due — and
// a recorded one that stopped is read once more and, unless it works again within Grace, written; a pane gone or no
// longer an agent's is written at once, and so is one last seen Away before this snapshot (its machine was out of
// reach), the pane at work again starting a turn of its own. Nothing here waits on a machine: the reads run in the
// machine's own reader. The snapshot is kept for Tick, which goes on reading between snapshots (tick.go).
func (r *Recorder) Observe(machineID string, snap *herdr.Snapshot, at time.Time) {
	if snap == nil {
		return
	}
	r.mu.Lock()
	if r.seen == nil {
		r.seen = map[string]seenSnap{}
	}
	last := seenSnap{snap: snap, at: at}
	r.seen[machineID] = last
	jobs := r.observeLocked(machineID, last, at)
	r.mu.Unlock()
	r.dispatch(jobs)
}

// observeLocked is the work on a machine's last snapshot as it stands at at, answering the jobs for its reader; r.mu
// held.
func (r *Recorder) observeLocked(machineID string, last seenSnap, at time.Time) []job {
	if r.off || r.scratch || r.closed {
		return nil
	}
	snap := last.snap
	var jobs []job
	seen := map[string]bool{}
	for i := range snap.Panes {
		pane := &snap.Panes[i]
		if pane.Agent == "" {
			continue
		}
		k := key(machineID, pane.PaneID)
		seen[k] = true
		tk := r.live[k]
		if tk != nil && last.at.Sub(tk.seenAt) >= Away {
			delete(r.live, k)
			jobs = append(jobs, job{kind: finishJob, take: tk, reason: EndedAway})
			tk = nil
		}
		if pane.AgentStatus == herdr.StatusWorking {
			if tk == nil {
				tk = r.begin(machineID, snap, pane, at)
				r.live[k] = tk
			}
			tk.seenAt = last.at
			tk.restingAt = time.Time{}
			tk.grow(snap.PaneSize(pane.PaneID))
			if !tk.queued && at.Sub(tk.askedAt) >= tk.every-time.Second {
				tk.queued, tk.askedAt = true, at
				jobs = append(jobs, job{kind: sampleJob, take: tk})
			}
			if at.Sub(tk.savedAt) >= CheckpointEvery {
				tk.savedAt = at
				jobs = append(jobs, job{kind: checkpointJob, take: tk})
			}
			continue
		}
		if tk == nil {
			continue
		}
		tk.seenAt = last.at
		tk.restedAs = pane.AgentStatus
		switch {
		case tk.restingAt.IsZero():
			tk.restingAt = at
			jobs = append(jobs, job{kind: sampleJob, take: tk}) // the screen it stopped on
		case at.Sub(tk.restingAt) >= Grace:
			delete(r.live, k)
			jobs = append(jobs, job{kind: finishJob, take: tk, reason: tk.restedAs})
		}
	}
	for k, tk := range r.live {
		if tk.meta.Machine == machineID && !seen[k] {
			delete(r.live, k)
			jobs = append(jobs, job{kind: finishJob, take: tk, reason: EndedGone})
		}
	}
	return jobs
}

// awayLocked writes the turns of a machine not heard from for Away, as they were when it was last seen: a recording
// never spans the machine's absence, nor waits in memory for its return; r.mu held.
func (r *Recorder) awayLocked(machineID string) []job {
	if r.off || r.scratch || r.closed {
		return nil
	}
	var jobs []job
	for k, tk := range r.live {
		if tk.meta.Machine == machineID {
			delete(r.live, k)
			reason := EndedAway
			if !tk.restingAt.IsZero() {
				reason = tk.restedAs
			}
			jobs = append(jobs, job{kind: finishJob, take: tk, reason: reason})
		}
	}
	return jobs
}

// begin is a new take of a pane seen working, named after the moment.
func (r *Recorder) begin(machineID string, snap *herdr.Snapshot, pane *herdr.PaneInfo, at time.Time) *take {
	meta := Meta{ID: newID(at), Machine: machineID, Pane: pane.PaneID, Agent: pane.Agent, Title: clip(pane.DisplayTitle()), Start: at}
	if ws := snap.Workspace(pane.WorkspaceID); ws != nil {
		meta.Workspace = clip(ws.Label)
		meta.Dir = clip(snap.WorkspaceDir(ws))
		meta.Folder = meta.Dir
		if r.in.Folder != nil {
			meta.Folder = clip(r.in.Folder(machineID, meta.Dir, ws))
		}
	}
	if meta.Dir == "" {
		meta.Dir = clip(pane.Dir())
		meta.Folder = meta.Dir
	}
	return &take{meta: meta, every: Interval, savedAt: at, seenAt: at}
}

// grow keeps the widest grid the pane had, so no frame is drawn narrower than it was
func (tk *take) grow(cols, rows int) {
	tk.meta.Cols = max(tk.meta.Cols, cols)
	tk.meta.Rows = max(tk.meta.Rows, rows)
}

// Forget writes the turns a removed machine's panes were in: the machine is gone, what they did stays.
func (r *Recorder) Forget(machineID string) {
	var jobs []job
	r.mu.Lock()
	delete(r.seen, machineID)
	for k, tk := range r.live {
		if tk.meta.Machine == machineID {
			delete(r.live, k)
			jobs = append(jobs, job{kind: finishJob, take: tk, reason: EndedGone})
		}
	}
	r.mu.Unlock()
	r.dispatch(jobs)
}

// Close writes every turn still going as stopped, and every one that ended and waits in a reader's queue — or is being
// written that moment — as it ended, since a reader's goroutine does not outlive the process; then it records nothing
// more, and leaves the readers nothing more to read: remuda is stopping.
func (r *Recorder) Close() {
	r.mu.Lock()
	r.closed = true
	var finals []job
	for k, tk := range r.live {
		finals = append(finals, job{kind: finishJob, take: tk, reason: EndedStopped})
		delete(r.live, k)
	}
	for machineID, waiting := range r.queue {
		for _, jb := range waiting {
			if jb.kind == finishJob {
				finals = append(finals, jb)
			}
		}
		delete(r.queue, machineID)
	}
	for _, jb := range r.busy {
		if jb.kind == finishJob {
			finals = append(finals, jb) // save waits for the write under way, then finds the take written
		}
	}
	r.mu.Unlock()
	for _, jb := range finals {
		r.save(jb.take, jb.reason, true)
	}
}

// dropLocked takes every turn off the recorder — those going, those waiting in a reader's queue and the one each reader
// is busy with — so nothing more of them lands on the disk, and answers each once; r.mu is held
func (r *Recorder) dropLocked() []*take {
	var dropped []*take
	once := map[*take]bool{}
	drop := func(tk *take) {
		if !once[tk] {
			once[tk] = true
			tk.written = true
			dropped = append(dropped, tk)
		}
	}
	for k, tk := range r.live {
		drop(tk)
		delete(r.live, k)
	}
	for machineID, waiting := range r.queue {
		for _, jb := range waiting {
			drop(jb.take)
		}
		delete(r.queue, machineID)
	}
	for _, jb := range r.busy {
		drop(jb.take)
	}
	return dropped
}

// isLive says whether a recording is a turn still going, whose checkpoint no listing shows; r.mu is held
func (r *Recorder) isLive(id string) bool {
	for _, tk := range r.live {
		if tk.meta.ID == id {
			return true
		}
	}
	return false
}
