package restart

// The agents whose panes went away — a tab or a workspace closed, through remuda or anywhere else — kept a while per
// machine in <state>/closed.json, so one can be brought back on its own session: a tab of the workspace on its folder,
// or a new workspace when that is gone.

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/state"
)

const closedFile = "closed.json"

// ClosedKeep is how many closed agents a machine keeps, the newest.
const ClosedKeep = 20

// ErrNoSuchClosed is a reopen of an agent the list does not hold (any more).
var ErrNoSuchClosed = errors.New("no such closed agent: it was reopened already, or it is too old to be kept")

// Closed is an agent pane that went away, with enough to start it again on its session and to name it in a list.
type Closed struct {
	ID        string    `json:"id"`
	Machine   string    `json:"machine"`
	Pane      string    `json:"pane"`
	Agent     string    `json:"agent"`
	Session   string    `json:"session"`
	Dir       string    `json:"dir"`
	Title     string    `json:"title"`
	Workspace string    `json:"workspace,omitempty"`
	Tab       string    `json:"tab,omitempty"` // the tab's own name, when the person gave it one
	At        time.Time `json:"at"`
}

// paneFace is how a remembered pane looked the last time it was seen: the names a closed one is listed by.
type paneFace struct{ title, workspace, tab string }

// faceOf reads a pane's names off its snapshot.
func faceOf(snap *herdr.Snapshot, p *herdr.PaneInfo) paneFace {
	face := paneFace{title: p.DisplayTitle(), workspace: snap.WorkspaceLabel(p.WorkspaceID)}
	for i := range snap.Tabs {
		if t := &snap.Tabs[i]; t.TabID == p.TabID && t.Label != strconv.Itoa(t.Number) {
			face.tab = t.Label
		}
	}
	return face
}

// noteClosedLocked lists a remembered chat whose pane went away, newest first, once per session.
func (m *Memory) noteClosedLocked(machineID, key, paneID string, c Chat, at time.Time) {
	face := m.faces[key]
	entry := Closed{
		ID: strconv.FormatInt(at.UnixNano(), 36) + "-" + paneID, Machine: machineID, Pane: paneID,
		Agent: c.Kind, Session: c.Session, Dir: c.Dir,
		Title: firstNonEmpty(face.title, c.Title), Workspace: face.workspace, Tab: face.tab, At: at,
	}
	list := []Closed{entry}
	for _, old := range m.closed[machineID] {
		if old.Session != c.Session && len(list) < ClosedKeep {
			list = append(list, old)
		}
	}
	m.closed[machineID] = list
}

func (m *Memory) saveClosedLocked() {
	if m.dir == "" {
		return
	}
	if err := state.SaveJSON(m.dir, closedFile, m.closed); err != nil {
		log.Printf("%s: %v", closedFile, err)
	}
}

// Closed is every machine's closed agents, newest first.
func (m *Memory) Closed() []Closed {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []Closed
	for _, list := range m.closed {
		all = append(all, list...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].At.After(all[j].At) })
	return all
}

// closedEntry finds one closed agent of a machine.
func (m *Memory) closedEntry(machineID, id string) (Closed, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.closed[machineID] {
		if c.ID == id {
			return c, true
		}
	}
	return Closed{}, false
}

// dropClosed takes one closed agent off its machine's list.
func (m *Memory) dropClosed(machineID, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var kept []Closed
	for _, c := range m.closed[machineID] {
		if c.ID != id {
			kept = append(kept, c)
		}
	}
	if len(kept) == 0 {
		delete(m.closed, machineID)
	} else {
		m.closed[machineID] = kept
	}
	m.saveClosedLocked()
}

// Closed lists every machine's closed agents, newest first; none without a memory.
func (r *Restarter) Closed() []Closed {
	if r.Memory == nil {
		return nil
	}
	return r.Memory.Closed()
}

// placer is what reopening needs of herdr's client: a place made for the agent, and the snapshot to find one in.
type placer interface {
	sessions.Placer
	SessionSnapshot() (*herdr.Snapshot, error)
}

// Reopen starts a closed agent again on its session, in a tab of the workspace on its folder or a new workspace on
// it. The folder must still be there, a Claude Code session's file too, and no pane may run that session already;
// the entry leaves the list once the agent is back, or when it turns out to be open already.
func (r *Restarter) Reopen(mc *machines.Machine, id string) (sessions.Place, error) {
	if r.Memory == nil {
		return sessions.Place{}, ErrNoSuchClosed
	}
	c, ok := r.Memory.closedEntry(mc.ID(), id)
	if !ok {
		return sessions.Place{}, ErrNoSuchClosed
	}
	if info, err := mc.ResolvePath("", c.Dir); err != nil || info.Kind != "dir" {
		return sessions.Place{}, fmt.Errorf("its folder %s is gone — nothing to reopen into", c.Dir)
	}
	if c.Agent == "claude" && !sessionFileExists(mc, Chat{Kind: c.Agent, Session: c.Session, Dir: c.Dir}) {
		return sessions.Place{}, errors.New("its session file is gone — nothing to resume")
	}
	client, _, err := mc.Ready()
	if err != nil {
		return sessions.Place{}, err
	}
	return r.reopenWith(client, mc.ID(), c)
}

// ErrReopening is a reopen asked while the same one is under way: a double click, or two devices at once.
var ErrReopening = errors.New("this agent is being reopened already")

// claimReopen takes the one reopen of a closed agent at a time; false while another holds it.
func (m *Memory) claimReopen(machineID, id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.reopening == nil {
		m.reopening = map[string]bool{}
	}
	key := machineID + "|" + id
	if m.reopening[key] {
		return false
	}
	m.reopening[key] = true
	return true
}

func (m *Memory) releaseReopen(machineID, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.reopening, machineID+"|"+id)
}

// reopenWith is Reopen past the checks of the machine's files, one at a time per entry: the entry still listed (a
// reopen that came first may have taken it), the session not open already, then the start.
func (r *Restarter) reopenWith(client placer, machineID string, c Closed) (sessions.Place, error) {
	if !r.Memory.claimReopen(machineID, c.ID) {
		return sessions.Place{}, ErrReopening
	}
	defer r.Memory.releaseReopen(machineID, c.ID)
	if _, ok := r.Memory.closedEntry(machineID, c.ID); !ok {
		return sessions.Place{}, ErrNoSuchClosed
	}
	snap, err := client.SessionSnapshot()
	if err != nil {
		return sessions.Place{}, err
	}
	for i := range snap.Panes {
		if p := &snap.Panes[i]; sessionOf(p) == c.Session {
			r.Memory.dropClosed(machineID, c.ID)
			return sessions.Place{Pane: p.PaneID, Workspace: p.WorkspaceID}, &OpenAlready{Pane: p.PaneID}
		}
	}
	place, err := sessions.StartIn(client, snap, c.Dir, c.Tab, c.Agent, sessions.Start{Resume: c.Session})
	if err != nil {
		return place, err
	}
	r.Memory.dropClosed(machineID, c.ID)
	return place, nil
}

// OpenAlready is a reopen of a session some pane runs already: that pane is the one to open.
type OpenAlready struct{ Pane string }

func (e *OpenAlready) Error() string { return "this conversation is open already, in " + e.Pane }
