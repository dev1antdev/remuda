package timelapse

import (
	"log"
	"time"
)

// Each machine has a reader of its own: one goroutine at a time per machine doing its jobs in order — the reads of
// its panes' screens, then the writes of what they recorded — so a slow machine never holds up the snapshots or
// another machine, and a turn's last screen lands before the turn is written.

// dispatch hands each job to its machine's reader, starting the reader when none runs: one per machine, in order.
func (r *Recorder) dispatch(jobs []job) {
	if len(jobs) == 0 {
		return
	}
	var start []string
	r.mu.Lock()
	for _, jb := range jobs {
		machineID := jb.take.meta.Machine
		r.queue[machineID] = append(r.queue[machineID], jb)
		if !r.running[machineID] {
			r.running[machineID] = true
			start = append(start, machineID)
		}
	}
	r.mu.Unlock()
	for _, machineID := range start {
		r.run(func() { r.drain(machineID) })
	}
}

func (r *Recorder) drain(machineID string) {
	for {
		r.mu.Lock()
		delete(r.busy, machineID)
		waiting := r.queue[machineID]
		if len(waiting) == 0 {
			r.running[machineID] = false
			delete(r.queue, machineID)
			r.mu.Unlock()
			return
		}
		next := waiting[0]
		r.queue[machineID] = waiting[1:]
		r.busy[machineID] = next
		r.mu.Unlock()
		switch next.kind {
		case sampleJob:
			r.sample(next.take)
		case checkpointJob:
			r.save(next.take, EndedCut, false)
		case finishJob:
			r.save(next.take, next.reason, true)
		}
	}
}

// sample reads the pane's screen and keeps it as a frame when it changed; a failed read is logged once a minute per
// machine, never for a pane that closed, or whose turn ended, while it was read
func (r *Recorder) sample(tk *take) {
	machineID := tk.meta.Machine
	text, err := r.in.Screen(machineID, tk.meta.Pane)
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	tk.queued = false
	if tk.written {
		return
	}
	if err != nil {
		if r.live[key(machineID, tk.meta.Pane)] == tk && now.Sub(r.failed[machineID]) >= time.Minute {
			r.failed[machineID] = now
			log.Printf("timelapse: reading %s on %s: %v", tk.meta.Pane, machineID, err)
		}
		return
	}
	if len(text) > MaxScreen {
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
	if end.IsZero() && final {
		end = tk.seenAt // gone, away or stopped: the last moment a snapshot showed it at work
	}
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
