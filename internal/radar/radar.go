// Package radar keeps the lifecycle of every agent pane as a person reads it, not as
// herdr reports it from moment to moment: done is held until seen, a question until the
// agent works again, and the last working moment is kept in <state>/activity.json.
package radar

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/state"
)

const (
	idleGrace    = 2500 * time.Millisecond // idle must persist this long after working to count
	saveInterval = 5 * time.Second
	activityFile = "activity.json"
)

// Display is what the UI gets for one agent pane.
type Display struct {
	State      string `json:"state"`                 // working | blocked | done | idle | unknown
	LastActive int64  `json:"last_active,omitempty"` // unix ms of the last moment the agent worked; 0 = never seen working
	Since      int64  `json:"since,omitempty"`       // unix ms when the shown state began
	Alert      string `json:"alert,omitempty"`       // the watchdog's note
}

// Episode is one pane's current state and when it began, for the watchdog.
type Episode struct {
	Machine, Pane string
	State         string
	Since         time.Time
}

// Recover finds a last-activity time for a pane the tracker has no stamp for; zero when there is none.
type Recover func(machineID string, p *herdr.PaneInfo) time.Time

type pane struct {
	status    string // herdr's, after the grace rule
	lastWork  time.Time
	doneHeld  bool // the tick is up until the pane is seen
	doneSeen  bool // this finish was looked at; herdr may go on saying done for hours
	blocked   bool // the question mark is up until the agent works again
	recovered bool
	since     time.Time
	shown     string
	alert     string
	known     bool
}

type Tracker struct {
	dir     string
	recover Recover

	mu       sync.Mutex
	panes    map[string]*pane // "machine|pane"
	lastSave time.Time
	dirty    bool
}

func key(machineID, paneID string) string { return machineID + "|" + paneID }

// stamp is what activity.json keeps per pane: when the agent last worked and whether its finish still waits to be seen.
type stamp struct {
	LastActive int64 `json:"last_active,omitempty"`
	Held       bool  `json:"held,omitempty"`
}

func New(dir string, recover Recover) *Tracker {
	t := &Tracker{dir: dir, recover: recover, panes: map[string]*pane{}}
	if b, err := os.ReadFile(filepath.Join(dir, activityFile)); err == nil {
		var stamps map[string]stamp
		if json.Unmarshal(b, &stamps) != nil {
			// the older file: a bare unix-ms stamp per pane
			var old map[string]int64
			if json.Unmarshal(b, &old) == nil {
				for k, ms := range old {
					stamps[k] = stamp{LastActive: ms}
				}
			}
		}
		for k, st := range stamps {
			if st.LastActive > 0 || st.Held {
				// a finish the earlier run had not raised was looked at
				t.panes[k] = &pane{lastWork: time.UnixMilli(st.LastActive), recovered: true, doneHeld: st.Held, doneSeen: !st.Held, known: true}
			}
		}
	}
	return t
}

// Observe folds one snapshot of a machine into the tracker; its panes missing from the snapshot are forgotten.
func (t *Tracker) Observe(machineID string, snap *herdr.Snapshot, now time.Time) {
	var recover []herdr.PaneInfo
	t.mu.Lock()
	live := map[string]bool{}
	for i := range snap.Panes {
		p := &snap.Panes[i]
		if p.Agent == "" {
			continue
		}
		k := key(machineID, p.PaneID)
		live[k] = true
		st := t.panes[k]
		if st == nil {
			st = &pane{}
			t.panes[k] = st
		}
		if st.lastWork.IsZero() && !st.recovered {
			st.recovered = true
			recover = append(recover, *p)
		}
		t.observe(p, st, now)
	}
	for k := range t.panes {
		if len(k) > len(machineID) && k[:len(machineID)+1] == machineID+"|" && !live[k] {
			delete(t.panes, k)
			t.dirty = true
		}
	}
	t.saveLocked(now, false)
	t.mu.Unlock()

	// a stat, over ssh for a remote: off the poller, and only where nothing arrived since
	if t.recover != nil && len(recover) > 0 {
		go func() {
			for i := range recover {
				p := &recover[i]
				at := t.recover(machineID, p)
				if at.IsZero() {
					continue
				}
				t.mu.Lock()
				if st := t.panes[key(machineID, p.PaneID)]; st != nil && st.lastWork.IsZero() {
					st.lastWork, t.dirty = at, true
				}
				t.mu.Unlock()
			}
		}()
	}
}

func (t *Tracker) observe(p *herdr.PaneInfo, st *pane, now time.Time) {
	status := p.AgentStatus
	if status == herdr.StatusWorking {
		st.lastWork = now
		t.dirty = true
	}
	// a blink through idle right after an observed working tick is still working (a loaded or recovered stamp is not one)
	if (status == herdr.StatusIdle || status == herdr.StatusDone) && st.status == herdr.StatusWorking && now.Sub(st.lastWork) < idleGrace {
		status = herdr.StatusWorking
	}
	// herdr keeps reporting done until the pane is focused in herdr itself, for hours: a finish looked at here stays looked at until the agent works again
	if !st.known {
		// first sight (a restart, or a pane from before remuda): a finish herdr still reports from before is not news
		st.known = true
		if status != herdr.StatusWorking && !st.doneHeld {
			st.doneSeen = true
		}
		t.dirty = true
	}
	if status == herdr.StatusWorking {
		if st.doneSeen || st.doneHeld {
			t.dirty = true
		}
		st.doneSeen, st.doneHeld = false, false
	} else if !st.doneSeen && (p.AgentStatus == herdr.StatusDone || (st.status == herdr.StatusWorking && status == herdr.StatusIdle)) {
		if !st.doneHeld {
			t.dirty = true
		}
		st.doneHeld = true
	}
	st.status = status

	switch {
	case status == herdr.StatusBlocked:
		st.blocked = true
	case status == herdr.StatusWorking || st.lastWork.IsZero():
		// working again, or it never had worked (recovery included): the dialog simply closed
		st.blocked = false
	}
	if p.Focused && st.doneHeld {
		st.doneHeld, st.doneSeen = false, true // herdr's own focus counts as having looked
		t.dirty = true
	}
	if shown := shownState(st); shown != st.shown {
		st.shown, st.since, st.alert = shown, now, ""
	}
}

// Forget drops a removed machine's panes.
func (t *Tracker) Forget(machineID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for k := range t.panes {
		if strings.HasPrefix(k, machineID+"|") {
			delete(t.panes, k)
			t.dirty = true
		}
	}
}

// Seen records that someone looked at the pane: the held tick comes down.
func (t *Tracker) Seen(machineID, paneID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if st := t.panes[key(machineID, paneID)]; st != nil && st.doneHeld {
		st.doneHeld, st.doneSeen = false, true
		t.dirty = true
	}
}

// Display reads the pane as a person would; one not observed yet is read from herdr's status alone.
func (t *Tracker) Display(machineID string, p *herdr.PaneInfo) Display {
	t.mu.Lock()
	defer t.mu.Unlock()
	st := t.panes[key(machineID, p.PaneID)]
	if st == nil {
		return Display{State: plain(p.AgentStatus)}
	}
	d := Display{}
	if !st.lastWork.IsZero() {
		d.LastActive = st.lastWork.UnixMilli()
		// the stamp moves every poll while it works; the roster's ETag must not
		if st.status == herdr.StatusWorking {
			d.LastActive = st.lastWork.Truncate(time.Minute).UnixMilli()
		}
	}
	d.State = shownState(st)
	if !st.since.IsZero() {
		d.Since = st.since.UnixMilli()
	}
	d.Alert = st.alert
	return d
}

func shownState(st *pane) string {
	switch {
	case st.status == herdr.StatusWorking:
		return herdr.StatusWorking
	case st.blocked:
		return herdr.StatusBlocked
	case st.doneHeld:
		return herdr.StatusDone
	default:
		return plain(st.status)
	}
}

// Episodes lists every tracked pane's shown state and when it began.
func (t *Tracker) Episodes() []Episode {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Episode, 0, len(t.panes))
	for k, st := range t.panes {
		m, p, _ := strings.Cut(k, "|")
		out = append(out, Episode{Machine: m, Pane: p, State: shownState(st), Since: st.since})
	}
	return out
}

// SetAlert attaches the watchdog's note to a pane ("" clears it).
func (t *Tracker) SetAlert(machineID, paneID, note string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if st := t.panes[key(machineID, paneID)]; st != nil {
		st.alert = note
	}
}

// plain maps herdr's status to a display state without any holding: a done herdr let go of is idle.
func plain(status string) string {
	switch status {
	case herdr.StatusWorking, herdr.StatusBlocked:
		return status
	case herdr.StatusIdle, herdr.StatusDone:
		return herdr.StatusIdle
	}
	return herdr.StatusUnknown
}

// Flush writes the stamps now (shutdown).
func (t *Tracker) Flush() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.saveLocked(time.Now(), true)
}

func (t *Tracker) saveLocked(now time.Time, force bool) {
	if !t.dirty || (!force && now.Sub(t.lastSave) < saveInterval) {
		return
	}
	stamps := map[string]stamp{}
	for k, st := range t.panes {
		if !st.lastWork.IsZero() || st.doneHeld {
			var ms int64
			if !st.lastWork.IsZero() {
				ms = st.lastWork.UnixMilli()
			}
			stamps[k] = stamp{LastActive: ms, Held: st.doneHeld}
		}
	}
	b, _ := json.Marshal(stamps)
	if state.WriteFile(filepath.Join(t.dir, activityFile), b) == nil {
		t.lastSave, t.dirty = now, false
	}
}
