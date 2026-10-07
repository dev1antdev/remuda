// Package radar keeps the lifecycle of every agent pane the way a person
// would read it, not the way herdr reports it from moment to moment:
//
//   - done stays done until someone has looked at the pane (herdr focused it,
//     or a herdr-ui client opened it), because `done` collapses into `idle`
//     within seconds and the tick is the whole point;
//   - a question (blocked) stays a question until the agent works again;
//   - a flicker through idle right after working is still working (grace);
//   - the time an agent last worked is remembered, so the UI can cool an
//     idle row with age — and panes that were open before herdr-ui started
//     get that time recovered from their session file.
//
// The display states are working | blocked | done | idle | unknown; the idle
// tiers (fresh / normal / stale) are the UI's to draw from LastActive, since
// they are a matter of thresholds and of the clock, not of events. Stamps
// live in <state>/activity.json so a restart keeps the order of the list.
package radar

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/herdr-ui/internal/herdr"
	"github.com/dev1antdev/herdr-ui/internal/state"
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
	Alert      string `json:"alert,omitempty"`       // the watchdog's note: quiet for 20 min, waiting 15 min, looping
}

// Episode is one pane's current state and when it began, for the watchdog.
type Episode struct {
	Machine, Pane string
	State         string
	Since         time.Time
}

// Recover finds a last-activity time for a pane the tracker has no stamp
// for — from its session record. Zero when there is none.
type Recover func(machineID string, p *herdr.PaneInfo) time.Time

type pane struct {
	status    string // herdr's, after the grace rule
	lastWork  time.Time
	doneHeld  bool // the tick is up until the pane is seen
	doneSeen  bool // this finish was looked at; herdr may go on saying done for hours
	blocked   bool // the question mark is up until the agent works again
	recovered bool
	since     time.Time // when the shown state began
	shown     string    // the state shown last, to notice a change
	alert     string    // the watchdog's note, cleared when the state moves on
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

func New(dir string, recover Recover) *Tracker {
	t := &Tracker{dir: dir, recover: recover, panes: map[string]*pane{}}
	if b, err := os.ReadFile(filepath.Join(dir, activityFile)); err == nil {
		var stamps map[string]int64
		if json.Unmarshal(b, &stamps) == nil {
			for k, ms := range stamps {
				if ms > 0 {
					t.panes[k] = &pane{lastWork: time.UnixMilli(ms), recovered: true}
				}
			}
		}
	}
	return t
}

// Observe folds one snapshot of a machine into the tracker. Panes of that
// machine missing from the snapshot are forgotten.
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

	// a session record is a stat, over ssh for a remote machine: off the
	// poller, one pane after another, and only where nothing arrived since
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
	// a blink through idle right after an observed working tick is still
	// working (a stamp loaded or recovered is not one)
	if (status == herdr.StatusIdle || status == herdr.StatusDone) && st.status == herdr.StatusWorking && now.Sub(st.lastWork) < idleGrace {
		status = herdr.StatusWorking
	}
	// finished: herdr says so, or it was working a poll ago and is not now.
	// herdr keeps saying done until the pane is focused in herdr itself —
	// for hours — so a finish that was looked at here stays looked at until
	// the agent works again
	if status == herdr.StatusWorking {
		st.doneSeen, st.doneHeld = false, false
	} else if !st.doneSeen && (p.AgentStatus == herdr.StatusDone || (st.status == herdr.StatusWorking && status == herdr.StatusIdle)) {
		st.doneHeld = true
	}
	st.status = status

	switch {
	case status == herdr.StatusBlocked:
		st.blocked = true
	case status == herdr.StatusWorking || st.lastWork.IsZero():
		// it is working again — or it never had worked (as far as anyone
		// knows, recovery included), so leaving blocked means the dialog is
		// simply gone
		st.blocked = false
	}
	if p.Focused {
		st.doneHeld, st.doneSeen = false, true // herdr's own focus counts as having looked
	}
	if shown := shownState(st); shown != st.shown {
		st.shown, st.since, st.alert = shown, now, ""
	}
}

// Forget drops a machine's panes: it was removed, nothing of it is to be
// watched or reminded about.
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
	if st := t.panes[key(machineID, paneID)]; st != nil {
		st.doneHeld, st.doneSeen = false, true
	}
}

// Display reads the pane as a person would. A pane the tracker has not
// observed yet is read from herdr's status alone.
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
		// while it works the stamp moves every poll; the roster's ETag
		// should not — the UI reads a working row as the liveliest anyway
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

// plain maps herdr's status to a display state without any holding: a
// `done` herdr already let go of is just idle.
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
	stamps := map[string]int64{}
	for k, st := range t.panes {
		if !st.lastWork.IsZero() {
			stamps[k] = st.lastWork.UnixMilli()
		}
	}
	b, _ := json.Marshal(stamps)
	if state.WriteFile(filepath.Join(t.dir, activityFile), b) == nil {
		t.lastSave, t.dirty = now, false
	}
}
