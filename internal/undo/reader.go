package undo

import "log"

// A machine's reader: the checkpoints of its turns read one at a time, in order, off the poll's path.

type jobKind int

const (
	startJob jobKind = iota
	endJob
	finishJob
)

type job struct {
	kind jobKind
	turn *going
	gen  int
}

// dispatch queues the jobs on their machines' readers and starts a reader where none runs.
func (t *Tracker) dispatch(jobs []job) {
	if len(jobs) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, j := range jobs {
		machineID := j.turn.open.Machine
		t.queue[machineID] = append(t.queue[machineID], j)
		if !t.running[machineID] {
			t.running[machineID] = true
			go t.drain(machineID)
		}
	}
}

func (t *Tracker) drain(machineID string) {
	for {
		t.mu.Lock()
		waiting := t.queue[machineID]
		if len(waiting) == 0 || t.closed {
			t.running[machineID] = false
			t.queue[machineID] = nil
			t.mu.Unlock()
			return
		}
		next := waiting[0]
		t.queue[machineID] = waiting[1:]
		t.mu.Unlock()
		t.run(next)
	}
}

func (t *Tracker) run(j job) {
	switch j.kind {
	case startJob:
		t.takeStart(j.turn)
	case endJob:
		t.takeEnd(j.turn, j.gen)
	case finishJob:
		t.finish(j.turn)
	}
}

// takeStart reads the folder as the turn begins and keeps it on the disk; a folder it cannot read makes the turn dead.
func (t *Tracker) takeStart(turn *going) {
	host, err := t.in.Host(turn.open.Machine)
	var start Read
	if err == nil {
		start, err = read(host, turn.open.Dir, readOptions{patch: forwardPatch})
	}
	if err == nil {
		patch := start.Patch
		start.Patch = ""
		t.mu.Lock()
		turn.open.Start = start
		open := turn.open
		t.mu.Unlock()
		err = t.store.SaveOpen(open, patch)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if err != nil {
		if err != ErrNotRepo {
			log.Printf("undo: no checkpoint of %s on %s: %v", turn.open.Dir, turn.open.Machine, err)
		}
		turn.dead = true
		return
	}
	turn.started = true
}

// takeEnd reads the folder as the turn rests, against the start's HEAD; a pane at work again meanwhile makes it stale.
func (t *Tracker) takeEnd(turn *going, gen int) {
	t.mu.Lock()
	if turn.dead || !turn.started || turn.gen != gen {
		t.mu.Unlock()
		return
	}
	base := turn.open.Start.Head
	t.mu.Unlock()
	host, err := t.in.Host(turn.open.Machine)
	var end Read
	if err == nil {
		end, err = read(host, turn.open.Dir, readOptions{base: base, patch: reversePatch})
	}
	t.mu.Lock()
	if err != nil {
		if err != ErrNotRepo {
			log.Printf("undo: no end checkpoint of %s on %s: %v", turn.open.Dir, turn.open.Machine, err)
		}
		turn.dead = true
		t.mu.Unlock()
		return
	}
	if turn.gen == gen {
		turn.end, turn.endGen = &end, gen
	}
	jobs := t.stepLocked(t.now())
	t.mu.Unlock()
	t.dispatch(jobs)
}

// finish keeps the turn's delta, unless it changed nothing; its start leaves the disk either way.
func (t *Tracker) finish(turn *going) {
	defer t.store.DropOpen(turn.open.ID)
	patch, err := t.store.OpenPatch(turn.open.ID)
	if err != nil {
		log.Printf("undo: the start of the turn in %s is gone: %v", turn.open.Dir, err)
		return
	}
	start := turn.open.Start
	start.Patch = patch
	delta, startPatch, endPatch := Delta(start, *turn.end)
	if len(delta.Files) == 0 && len(delta.Commits) == 0 && delta.HeadStart == delta.HeadEnd {
		return
	}
	delta.ID, delta.Machine, delta.Dir, delta.Pane, delta.Agent = turn.open.ID, turn.open.Machine, turn.open.Dir, turn.open.Pane, turn.open.Agent
	delta.Started, delta.Ended, delta.Shared = turn.open.Started, turn.restingAt, turn.open.Shared
	if err := t.store.Add(delta, startPatch, endPatch); err != nil {
		log.Printf("undo: the turn in %s could not be kept: %v", turn.open.Dir, err)
	}
}
