// Package awake keeps a machine from sleeping while its agents work: a hold
// per machine — caffeinate on macOS, systemd-inhibit on Linux — taken when an
// agent pane there is working and let go a while after the last one settled,
// when the switch goes off and when remuda stops. The switch per machine lives
// in awake.json in the state dir, off by default.
package awake

import (
	"log"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/state"
)

const file = "awake.json"

// Grace is how long the hold stands after the last agent settled, so a pause between two prompts keeps it.
var Grace = 2 * time.Minute

// Reassert is how often a hold that stands is taken again while agents still work, so an inhibitor that died
// (a reboot) comes back, and how long a failed hold waits before it is tried again.
var Reassert = 5 * time.Minute

// Status is a machine's hold as the roster carries it.
type Status struct {
	On      bool   `json:"on"`
	Held    bool   `json:"held"`
	Working int    `json:"working"`
	Error   string `json:"error,omitempty"`
}

// Holder takes and lets go of the hold on a machine.
type Holder interface {
	Hold(host shell.Host) error
	Release(host shell.Host) error
}

type machine struct {
	on      bool
	held    bool
	working int
	err     string
	busy    bool        // a hold or a release is running
	timer   *time.Timer // the grace after the last agent settled
	heldAt  time.Time
	triedAt time.Time
}

// Service holds the switches and the holds of every machine.
type Service struct {
	mu       sync.Mutex
	dir      string
	hosts    func(id string) (shell.Host, bool)
	holder   Holder
	machines map[string]*machine
	wg       sync.WaitGroup
}

// New reads the switches from the state dir; hosts finds a machine's shell, holder takes the hold on it.
func New(dir string, hosts func(id string) (shell.Host, bool), holder Holder) *Service {
	s := &Service{dir: dir, hosts: hosts, holder: holder, machines: map[string]*machine{}}
	on := map[string]bool{}
	if err := state.LoadJSON(dir, file, &on); err != nil {
		log.Printf("awake: %s unreadable, every switch off: %v", file, err)
	}
	for id, v := range on {
		if v {
			s.machines[id] = &machine{on: true}
		}
	}
	return s
}

// Working counts the agent panes of a snapshot that work right now.
func Working(snap *herdr.Snapshot) int {
	if snap == nil {
		return 0
	}
	n := 0
	for i := range snap.Panes {
		if p := &snap.Panes[i]; p.Agent != "" && p.AgentStatus == herdr.StatusWorking {
			n++
		}
	}
	return n
}

// Observe takes a machine's count of working agents, from every snapshot of it.
func (s *Service) Observe(id string, working int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.machines[id]
	if m == nil {
		if working == 0 {
			return
		}
		m = &machine{}
		s.machines[id] = m
	}
	m.working = working
	if working > 0 && m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	if m.on && m.held && working > 0 && !m.busy && time.Since(m.heldAt) >= Reassert {
		s.take(id, m)
		return
	}
	s.settle(id, m)
}

// Set flips the machine's switch and keeps it.
func (s *Service) Set(id string, on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.machines[id]
	if m == nil {
		m = &machine{}
		s.machines[id] = m
	}
	m.on = on
	m.err = ""
	m.triedAt = time.Time{}
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	switches := map[string]bool{}
	for k, v := range s.machines {
		if v.on {
			switches[k] = true
		}
	}
	if err := state.SaveJSON(s.dir, file, switches); err != nil {
		return err
	}
	s.settle(id, m)
	return nil
}

// Status is the machine's switch and hold.
func (s *Service) Status(id string) Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.machines[id]
	if m == nil {
		return Status{}
	}
	return Status{On: m.on, Held: m.held, Working: m.working, Error: m.err}
}

// Forget lets a removed machine's hold go and drops it.
func (s *Service) Forget(id string) {
	s.mu.Lock()
	m := s.machines[id]
	if m == nil {
		s.mu.Unlock()
		return
	}
	m.on = false
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	s.settle(id, m)
	s.mu.Unlock()
	s.wg.Wait()
	s.mu.Lock()
	delete(s.machines, id)
	s.mu.Unlock()
}

// Close lets every hold go: remuda is stopping.
func (s *Service) Close() {
	s.mu.Lock()
	for id, m := range s.machines {
		m.on = false
		if m.timer != nil {
			m.timer.Stop()
			m.timer = nil
		}
		s.settle(id, m)
	}
	s.mu.Unlock()
	s.wg.Wait()
}

// Wait waits for the holds and releases in flight, for tests.
func (s *Service) Wait() { s.wg.Wait() }

// settle decides the machine's next move from its state; called with the lock held.
func (s *Service) settle(id string, m *machine) {
	switch {
	case m.busy:
	case !m.on && m.held:
		s.drop(id, m)
	case m.on && m.working > 0 && !m.held:
		if time.Since(m.triedAt) >= Reassert {
			s.take(id, m)
		}
	case m.on && m.working == 0 && m.held && m.timer == nil:
		m.timer = time.AfterFunc(Grace, func() { s.lapse(id) })
	}
}

// lapse is the grace's end: nothing worked since, so the hold goes.
func (s *Service) lapse(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.machines[id]
	if m == nil {
		return
	}
	m.timer = nil
	if m.on && m.working == 0 && m.held && !m.busy {
		s.drop(id, m)
	}
}

func (s *Service) take(id string, m *machine) {
	m.busy = true
	m.triedAt = time.Now()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		err := s.on(id, s.holder.Hold)
		s.mu.Lock()
		defer s.mu.Unlock()
		m.busy = false
		if err != nil {
			if m.err != err.Error() {
				log.Printf("awake: %s: %v", id, err)
			}
			m.err = err.Error()
			s.settle(id, m)
			return
		}
		m.err = ""
		m.held = true
		m.heldAt = time.Now()
		m.triedAt = time.Time{}
		s.settle(id, m)
	}()
}

func (s *Service) drop(id string, m *machine) {
	m.busy = true
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		err := s.on(id, s.holder.Release)
		s.mu.Lock()
		defer s.mu.Unlock()
		m.busy = false
		m.held = false
		if err != nil {
			log.Printf("awake: %s: the hold could not be let go: %v", id, err)
		}
		s.settle(id, m)
	}()
}

// on runs fn on the machine's shell.
func (s *Service) on(id string, fn func(shell.Host) error) error {
	host, ok := s.hosts(id)
	if !ok {
		return &noHost{id}
	}
	return fn(host)
}

type noHost struct{ id string }

func (e *noHost) Error() string { return "machine " + e.id + " is not known" }
