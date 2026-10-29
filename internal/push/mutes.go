package push

import (
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/state"
)

// A workspace folder muted for a while: its panes send no push, no webhook and add no feed row until the time
// passes — the person asked for silence there, so the activity is not news. The voice and the badge on the page
// read the roster and are not touched.

const mutesFile = "mutes.json"

// Mute is one folder on one machine, quiet until Until.
type Mute struct {
	Machine string    `json:"machine"`
	Dir     string    `json:"dir"`
	Until   time.Time `json:"until"`
}

// Mutes is the list, kept in the state dir; an expired one is dropped the next time the list is read.
type Mutes struct {
	mu   sync.Mutex
	dir  string
	list map[string]Mute
	now  func() time.Time
}

func muteKey(machine, dir string) string { return machine + "|" + dir }

func loadMutes(dir string) *Mutes {
	m := &Mutes{dir: dir, list: map[string]Mute{}, now: time.Now}
	var stored []Mute
	if err := state.LoadJSON(dir, mutesFile, &stored); err != nil {
		log.Printf("mutes: %s unreadable, starting empty: %v", mutesFile, err)
	}
	for _, mute := range stored {
		m.list[muteKey(mute.Machine, mute.Dir)] = mute
	}
	return m
}

// Mute keeps the folder quiet until the time given, which must still be ahead.
func (m *Mutes) Mute(machine, dir string, until time.Time) error {
	if machine == "" || dir == "" {
		return fmt.Errorf("a machine and a folder are required")
	}
	if !until.After(m.now()) {
		return fmt.Errorf("the time to unmute at is already past")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.list[muteKey(machine, dir)] = Mute{Machine: machine, Dir: dir, Until: until}
	return m.saveLocked()
}

// Unmute ends a folder's silence now; a folder that was not muted is no error.
func (m *Mutes) Unmute(machine, dir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.list[muteKey(machine, dir)]; !ok {
		return nil
	}
	delete(m.list, muteKey(machine, dir))
	return m.saveLocked()
}

// Until says when a folder's silence ends, false when it is not muted (or no longer).
func (m *Mutes) Until(machine, dir string) (time.Time, bool) {
	if dir == "" {
		return time.Time{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked()
	mute, ok := m.list[muteKey(machine, dir)]
	return mute.Until, ok
}

// All lists the mutes still standing, by machine then folder.
func (m *Mutes) All() []Mute {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked()
	out := make([]Mute, 0, len(m.list))
	for _, mute := range m.list {
		out = append(out, mute)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Machine != out[j].Machine {
			return out[i].Machine < out[j].Machine
		}
		return out[i].Dir < out[j].Dir
	})
	return out
}

// pruneLocked drops the mutes whose time has passed and writes the file when any went.
func (m *Mutes) pruneLocked() {
	now := m.now()
	changed := false
	for key, mute := range m.list {
		if !mute.Until.After(now) {
			delete(m.list, key)
			changed = true
		}
	}
	if changed {
		_ = m.saveLocked()
	}
}

func (m *Mutes) saveLocked() error {
	if m.dir == "" {
		return nil
	}
	list := make([]Mute, 0, len(m.list))
	for _, mute := range m.list {
		list = append(list, mute)
	}
	sort.Slice(list, func(i, j int) bool {
		return muteKey(list[i].Machine, list[i].Dir) < muteKey(list[j].Machine, list[j].Dir)
	})
	return state.SaveJSON(m.dir, mutesFile, list)
}

// silenced says whether an alert about a pane falls in a muted folder: the folder is asked of the watcher at that
// moment, so a mute set after the alert was scheduled still holds.
func (s *Service) silenced(p Payload) bool {
	if s.Folder == nil || s.Mutes == nil || p.Pane == "" {
		return false
	}
	dir := s.Folder(p.Machine, p.Pane)
	if dir == "" {
		return false
	}
	_, muted := s.Mutes.Until(p.Machine, dir)
	return muted
}
