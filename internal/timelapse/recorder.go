package timelapse

import (
	"log"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// take is one turn being recorded: its frames in memory until the turn ends.
type take struct {
	meta      Meta
	frames    []frame
	every     time.Duration // the pace, Interval doubled at every thinning
	askedAt   time.Time     // the snapshot the last read was asked at: the pace goes by the snapshots' clock
	queued    bool          // a read of the screen waits in the machine's queue
	restingAt time.Time     // when the pane stopped working; zero while it works
	restedAs  string        // the status it rests in
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
	pane   string
	take   *take
	reason string // a finish's: why the turn ended
}

func key(machineID, paneID string) string { return machineID + "|" + paneID }

// Observe hears every snapshot: a working agent pane is recorded — a read of its screen queued when one is due — and
// a recorded one that stopped is read once more and, unless it works again within Grace, written; a pane gone or no
// longer an agent's is written at once. Nothing here waits on a machine: the reads run in the machine's own reader.
func (r *Recorder) Observe(machineID string, snap *herdr.Snapshot, at time.Time) {
	if snap == nil {
		return
	}
	var jobs []job
	r.mu.Lock()
	if r.off || r.scratch || r.closed {
		r.mu.Unlock()
		return
	}
	seen := map[string]bool{}
	for i := range snap.Panes {
		pane := &snap.Panes[i]
		if pane.Agent == "" {
			continue
		}
		k := key(machineID, pane.PaneID)
		seen[k] = true
		tk := r.live[k]
		if pane.AgentStatus == herdr.StatusWorking {
			if tk == nil {
				tk = r.begin(machineID, snap, pane, at)
				r.live[k] = tk
			}
			tk.restingAt = time.Time{}
			tk.grow(snap.PaneSize(pane.PaneID))
			if !tk.queued && at.Sub(tk.askedAt) >= tk.every-time.Second {
				tk.queued, tk.askedAt = true, at
				jobs = append(jobs, job{kind: sampleJob, pane: pane.PaneID, take: tk})
			}
			if at.Sub(tk.savedAt) >= CheckpointEvery {
				tk.savedAt = at
				jobs = append(jobs, job{kind: checkpointJob, pane: pane.PaneID, take: tk})
			}
			continue
		}
		if tk == nil {
			continue
		}
		tk.restedAs = pane.AgentStatus
		switch {
		case tk.restingAt.IsZero():
			tk.restingAt = at
			jobs = append(jobs, job{kind: sampleJob, pane: pane.PaneID, take: tk}) // the screen it stopped on
		case at.Sub(tk.restingAt) >= Grace:
			delete(r.live, k)
			jobs = append(jobs, job{kind: finishJob, pane: pane.PaneID, take: tk, reason: tk.restedAs})
		}
	}
	for k, tk := range r.live {
		if tk.meta.Machine == machineID && !seen[k] {
			delete(r.live, k)
			jobs = append(jobs, job{kind: finishJob, pane: tk.meta.Pane, take: tk, reason: EndedGone})
		}
	}
	r.mu.Unlock()
	r.dispatch(machineID, jobs)
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
	return &take{meta: meta, every: Interval, savedAt: at}
}

// grow keeps the widest grid the pane had, so no frame is drawn narrower than it was
func (tk *take) grow(cols, rows int) {
	tk.meta.Cols = max(tk.meta.Cols, cols)
	tk.meta.Rows = max(tk.meta.Rows, rows)
}

// dispatch hands the jobs to the machine's reader, starting it when none runs: one reader per machine, in order.
func (r *Recorder) dispatch(machineID string, jobs []job) {
	if len(jobs) == 0 {
		return
	}
	r.mu.Lock()
	r.queue[machineID] = append(r.queue[machineID], jobs...)
	start := !r.running[machineID]
	r.running[machineID] = true
	r.mu.Unlock()
	if start {
		r.run(func() { r.drain(machineID) })
	}
}

func (r *Recorder) drain(machineID string) {
	for {
		r.mu.Lock()
		waiting := r.queue[machineID]
		if len(waiting) == 0 {
			r.running[machineID] = false
			r.mu.Unlock()
			return
		}
		next := waiting[0]
		r.queue[machineID] = waiting[1:]
		r.mu.Unlock()
		switch next.kind {
		case sampleJob:
			r.sample(machineID, next)
		case checkpointJob:
			r.save(next.take, EndedCut, false)
		case finishJob:
			r.save(next.take, next.reason, true)
		}
	}
}

// sample reads the pane's screen and keeps it as a frame when it changed; a failed read is logged once a minute per machine
func (r *Recorder) sample(machineID string, jb job) {
	text, err := r.in.Screen(machineID, jb.pane)
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	tk := jb.take
	tk.queued = false
	if err != nil {
		if now.Sub(r.failed[machineID]) >= time.Minute {
			r.failed[machineID] = now
			log.Printf("timelapse: reading %s on %s: %v", jb.pane, machineID, err)
		}
		return
	}
	if tk.written || len(text) > MaxScreen {
		return
	}
	tk.add(now, text)
}

// save writes the take: a checkpoint of what it has so far, or the turn whole once it ended — dropped, with any
// checkpoint, when it was shorter than MinTurn or holds a single screen
func (r *Recorder) save(tk *take, reason string, final bool) {
	tk.writing.Lock()
	defer tk.writing.Unlock()
	r.mu.Lock()
	if tk.written {
		r.mu.Unlock()
		return
	}
	end := tk.restingAt
	if end.IsZero() || !final {
		end = r.now()
	}
	frames := tk.frames
	if n := len(frames); n > 0 && frames[n-1].at.After(end) {
		end = frames[n-1].at
	}
	meta := tk.meta
	if final {
		tk.written = true
	}
	r.mu.Unlock()
	meta.End, meta.Ended, meta.Count = end, reason, len(frames)
	if len(frames) < 2 || end.Sub(meta.Start) < MinTurn {
		if final {
			r.remove(meta.ID)
		}
		return
	}
	if err := r.write(Recording{Meta: meta, Frames: encode(frames, meta.Start)}); err != nil {
		log.Printf("timelapse: writing %s: %v", meta.ID, err)
	}
}

// Forget writes the turns a removed machine's panes were in: the machine is gone, what they did stays.
func (r *Recorder) Forget(machineID string) {
	var jobs []job
	r.mu.Lock()
	for k, tk := range r.live {
		if tk.meta.Machine == machineID {
			delete(r.live, k)
			jobs = append(jobs, job{kind: finishJob, pane: tk.meta.Pane, take: tk, reason: EndedGone})
		}
	}
	r.mu.Unlock()
	r.dispatch(machineID, jobs)
}

// Close writes every turn still going as stopped, and records nothing more: remuda is stopping.
func (r *Recorder) Close() {
	r.mu.Lock()
	r.closed = true
	takes := make([]*take, 0, len(r.live))
	for k, tk := range r.live {
		takes = append(takes, tk)
		delete(r.live, k)
	}
	r.mu.Unlock()
	for _, tk := range takes {
		r.save(tk, EndedStopped, true)
	}
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
