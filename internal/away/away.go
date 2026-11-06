// Package away turns a machine's connection transitions into the two notices a person wants: that a machine has
// been unreachable for a while, and that it answers again. A blip under the threshold says nothing, remuda's own
// start says nothing (the first state of a machine only seeds), and a "back" is only ever the end of an away that
// was reported.
package away

import (
	"sync"
	"time"
)

// AwayAfter is how long a machine stays unreachable before it is reported away.
const AwayAfter = 2 * time.Minute

// Notice is what the tracker says about a machine.
type Notice struct {
	Machine string
	Label   string
	Away    bool          // true: unreachable for AwayAfter; false: back after a reported away
	Reason  string        // why it could not be reached, as the connection said
	Since   time.Time     // when it was last seen reachable
	For     time.Duration // how long it was away, on a back
}

// Tracker keeps one reading per machine; Observe takes the transitions, Sweep reports the aways that stand.
type Tracker struct {
	AwayAfter time.Duration
	Notify    func(Notice)
	Now       func() time.Time

	mu       sync.Mutex
	machines map[string]*reading
}

type reading struct {
	label    string
	up       bool
	seenUp   bool      // reachable at least once in this tracker's life: a machine first seen down is not away, it was never here
	since    time.Time // down since
	reason   string
	reported bool
}

// New makes a tracker that reports an away after awayAfter.
func New(awayAfter time.Duration, notify func(Notice)) *Tracker {
	return &Tracker{AwayAfter: awayAfter, Notify: notify, Now: time.Now, machines: map[string]*reading{}}
}

// Observe takes a machine's new state: "connected", or anything else for unreachable with the reason.
func (t *Tracker) Observe(machine, label, state, reason string) {
	up := state == "connected"
	now := t.Now()
	t.mu.Lock()
	r, seen := t.machines[machine]
	if !seen {
		t.machines[machine] = &reading{label: label, up: up, seenUp: up, since: now, reason: reason}
		t.mu.Unlock()
		return
	}
	r.label = label
	var notice *Notice
	switch {
	case up && !r.up:
		if r.reported {
			notice = &Notice{Machine: machine, Label: label, Away: false, Since: r.since, For: now.Sub(r.since)}
		}
		r.up, r.seenUp, r.reported, r.reason = true, true, false, ""
	case !up && r.up:
		r.up, r.since, r.reason = false, now, reason
	case !up:
		r.reason = reason
	}
	t.mu.Unlock()
	if notice != nil && t.Notify != nil {
		t.Notify(*notice)
	}
}

// Sweep reports every machine that has been unreachable for AwayAfter and was not reported yet.
func (t *Tracker) Sweep(now time.Time) {
	var notices []Notice
	t.mu.Lock()
	for id, r := range t.machines {
		if r.up || !r.seenUp || r.reported || now.Sub(r.since) < t.AwayAfter {
			continue
		}
		r.reported = true
		notices = append(notices, Notice{Machine: id, Label: r.label, Away: true, Reason: r.reason, Since: r.since})
	}
	t.mu.Unlock()
	if t.Notify != nil {
		for _, n := range notices {
			t.Notify(n)
		}
	}
}

// Forget drops a machine that left the registry.
func (t *Tracker) Forget(machine string) {
	t.mu.Lock()
	delete(t.machines, machine)
	t.mu.Unlock()
}

// Words is the notice as a push reads it: the machine's name and the reason, or how long it was gone.
func Words(n Notice) (title, body string) {
	if n.Away {
		reason := n.Reason
		if len([]rune(reason)) > 120 {
			reason = string([]rune(reason)[:117]) + "…"
		}
		return n.Label + " is away", "unreachable since " + n.Since.Local().Format("15:04") + ": " + reason
	}
	return n.Label + " is back", "away " + n.For.Round(time.Minute).String()
}

// Run sweeps every so often until done is closed.
func (t *Tracker) Run(every time.Duration, done <-chan struct{}) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case now := <-ticker.C:
			t.Sweep(now)
		}
	}
}
