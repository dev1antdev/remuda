package drift

import (
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/updates"
)

// FreshFor is how long a machine's read stands before the table reads it again; Read again reads it at once.
var FreshFor = time.Minute

// ReadTimeout bounds the script on one machine.
var ReadTimeout = 30 * time.Second

// Service reads every machine of the fleet side by side and lays them out as the table.
type Service struct {
	Fleet   *machines.Manager
	Health  *health.Collector
	Updates *updates.Service
	Dirs    func(machineID string) []string // the folders of the machine's workspaces, from the watcher's snapshot

	mu    sync.Mutex
	reads map[string]read
	locks map[string]*sync.Mutex
}

// read is a machine's facts as last read.
type read struct {
	facts Facts
	err   error
	at    time.Time
}

// Read runs the script on the host for its workspaces' folders.
func Read(host shell.Host, dirs []string) (Facts, error) {
	out, err := shell.Text(host, Script(dirs), ReadTimeout)
	if err != nil {
		return Facts{}, err
	}
	return Parse(out), nil
}

// Table is the fleet as it stands: every connected machine read (at once when fresh, its vitals too), the away ones
// named with nothing read. The host of a remuda serving the page only is no machine of the fleet and no column.
func (s *Service) Table(fresh bool) Table {
	list := s.Fleet.List()
	columns := make([]Machine, len(list))
	var wg sync.WaitGroup
	for i, mc := range list {
		columns[i] = Machine{ID: mc.ID(), Label: mc.Label()}
		if state, _ := mc.State(); state != machines.StateConnected {
			columns[i].Away = true
			continue
		}
		wg.Add(1)
		go func(column *Machine, mc *machines.Machine) {
			defer wg.Done()
			if fresh {
				s.Health.Refresh(mc)
			}
			r := s.facts(mc, fresh)
			if r.err != nil {
				column.Error = r.err.Error()
			} else {
				column.Facts = &r.facts
			}
			column.Report = s.Health.Report(mc.ID())
			column.Offered = s.Updates.Available(mc)
		}(&columns[i], mc)
	}
	wg.Wait()
	return Build(columns, time.Now())
}

// facts is the machine's last read while it is FreshFor old, else a new one; one read of a machine at a time.
func (s *Service) facts(mc *machines.Machine, fresh bool) read {
	s.mu.Lock()
	if s.locks == nil {
		s.locks, s.reads = map[string]*sync.Mutex{}, map[string]read{}
	}
	lock := s.locks[mc.ID()]
	if lock == nil {
		lock = &sync.Mutex{}
		s.locks[mc.ID()] = lock
	}
	s.mu.Unlock()
	lock.Lock()
	defer lock.Unlock()
	s.mu.Lock()
	last, ok := s.reads[mc.ID()]
	s.mu.Unlock()
	if ok && !fresh && time.Since(last.at) < FreshFor {
		return last
	}
	var dirs []string
	if s.Dirs != nil {
		dirs = s.Dirs(mc.ID())
	}
	facts, err := Read(mc, dirs)
	r := read{facts: facts, err: err, at: time.Now()}
	s.mu.Lock()
	s.reads[mc.ID()] = r
	s.mu.Unlock()
	return r
}

// Forget drops a removed machine's read.
func (s *Service) Forget(machineID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.reads, machineID)
}
