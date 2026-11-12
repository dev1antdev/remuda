package timelapse

import (
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// The recorder's own clock. The watcher snapshots a machine when herdr's event stream says something moved, and only
// every 15–20 s behind it otherwise: an agent at work whose title stays still (no spinner there) brings no snapshot
// for that long, and its turn would be read that far apart. Between snapshots the last one is observed again, so a
// working pane's screen is read at Interval and a turn at rest is written Grace after it stopped, whatever the
// snapshots' pace.
var (
	TickEvery = 2 * time.Second // how often the last snapshots are observed again
	TickFor   = time.Minute     // a snapshot older than this is a machine not heard from: nothing more is read on it
)

// seenSnap is a machine's last snapshot and when it was taken.
type seenSnap struct {
	snap *herdr.Snapshot
	at   time.Time
}

// Tick observes every machine's last snapshot again as of now, while it is no older than TickFor, and writes the turns
// of a machine not heard from for Away (awayLocked).
func (r *Recorder) Tick(now time.Time) {
	var jobs []job
	r.mu.Lock()
	for machineID, last := range r.seen {
		switch {
		case now.Sub(last.at) >= Away:
			jobs = append(jobs, r.awayLocked(machineID)...)
		case now.After(last.at) && now.Sub(last.at) <= TickFor:
			jobs = append(jobs, r.observeLocked(machineID, last, now)...)
		}
	}
	r.mu.Unlock()
	r.dispatch(jobs)
}

// Loop ticks every TickEvery until done closes.
func (r *Recorder) Loop(done <-chan struct{}) {
	ticker := time.NewTicker(TickEvery)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			r.Tick(r.now())
		}
	}
}
