package gitsync

import (
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/shell"
)

// FreshFor is how long a machine's read stands for another page asking; an action on the machine drops it.
var FreshFor = 10 * time.Second

// PerMachine is how many network actions run at once on one machine: each is a session of its ssh master, whose
// server allows ten by default.
const PerMachine = 4

// Machine is a machine of the fleet as the place lists it.
type Machine struct {
	Host shell.Host
	Away string // why it cannot be read now, "" while connected
}

// Service is the Repositories place: every machine's repositories read side by side, and the actions on one.
type Service struct {
	Fleet   func() []Machine
	Host    func(machineID string) (shell.Host, error)                   // a connected machine, or why not
	Folders func(machineID string) []string                              // the machine's workspace folders, each its repository's main folder once known
	Busy    func(machineID, checkout string) (agent, doing string)       // the agent at work, asking or ending a turn in the checkout (AgentIn), "" for none
	Check   func(host shell.Host, dir string) (review.SecretScan, error) // the secret check before a push
	Scans   *Scans

	mu      sync.Mutex
	reads   map[string]Listed
	locks   map[string]*sync.Mutex
	running map[string]bool
	slots   map[string]chan struct{}
}

// Listed is one machine's part of the place.
type Listed struct {
	ID          string    `json:"id"`
	Label       string    `json:"label"`
	Away        string    `json:"away,omitempty"`  // not connected: why
	Error       string    `json:"error,omitempty"` // the read failed: why
	Scan        *Scan     `json:"scan,omitempty"`
	ScanWhere   string    `json:"scan_where,omitempty"`   // the scan's folder with the home said as ~
	ScanMissing bool      `json:"scan_missing,omitempty"` // the folder to scan is not there
	More        bool      `json:"more,omitempty"`         // the scan found more than MaxScan
	Repos       []Repo    `json:"repos"`
	At          time.Time `json:"at,omitzero"`
}

// List is every machine of the fleet with its repositories, the connected ones read side by side; fresh reads each
// at once, else a read FreshFor old stands.
func (s *Service) List(fresh bool) []Listed {
	fleet := s.Fleet()
	out := make([]Listed, len(fleet))
	var wg sync.WaitGroup
	for i, m := range fleet {
		out[i] = Listed{ID: m.Host.ID(), Label: m.Host.Label(), Repos: []Repo{}}
		if scan, ok := s.Scans.Of(m.Host.ID()); ok {
			out[i].Scan, out[i].ScanWhere = &scan, homeWords(rpath.For(m.Host.Home()), m.Host.Home(), scan.Folder)
		}
		if m.Away != "" {
			out[i].Away = m.Away
			continue
		}
		wg.Add(1)
		go func(listed *Listed, host shell.Host) {
			defer wg.Done()
			read := s.read(host, fresh)
			read.Label = listed.Label
			*listed = read
		}(&out[i], m.Host)
	}
	wg.Wait()
	return out
}

// read is the machine's last read while it is FreshFor old, else a new one; one read of a machine at a time.
func (s *Service) read(host shell.Host, fresh bool) Listed {
	lock := s.lockOf(host.ID())
	lock.Lock()
	defer lock.Unlock()
	s.mu.Lock()
	last, ok := s.reads[host.ID()]
	s.mu.Unlock()
	if ok && !fresh && time.Since(last.At) < FreshFor {
		return last
	}
	listed := Listed{ID: host.ID(), Label: host.Label(), Repos: []Repo{}, At: time.Now()}
	style := rpath.For(host.Home())
	var scan *Scan
	if found, ok := s.Scans.Of(host.ID()); ok {
		scan = &found
		listed.Scan, listed.ScanWhere = &found, homeWords(style, host.Home(), found.Folder)
	}
	var folders []string
	if s.Folders != nil {
		folders = s.Folders(host.ID())
	}
	output, err := shell.Text(host, ReadScript(style, folders, scan), Timeout)
	if err != nil {
		listed.Error = err.Error()
	} else {
		found := ParseRead(style, host.Home(), output)
		listed.Repos, listed.More, listed.ScanMissing = found.Repos, found.More, found.ScanMissing
		if listed.Repos == nil {
			listed.Repos = []Repo{}
		}
	}
	s.mu.Lock()
	s.reads[host.ID()] = listed
	s.mu.Unlock()
	return listed
}

func (s *Service) lockOf(machineID string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locks == nil {
		s.locks, s.reads, s.running, s.slots = map[string]*sync.Mutex{}, map[string]Listed{}, map[string]bool{}, map[string]chan struct{}{}
	}
	lock := s.locks[machineID]
	if lock == nil {
		lock = &sync.Mutex{}
		s.locks[machineID] = lock
	}
	return lock
}

// Forget drops a machine's read, so the next list reads it afresh; a removed machine's scan goes with it.
func (s *Service) Forget(machineID string, removed bool) {
	s.mu.Lock()
	delete(s.reads, machineID)
	s.mu.Unlock()
	if removed {
		s.Scans.Forget(machineID)
	}
}

// SetScan keeps the machine's folder to scan, checked against its home; an empty folder stops the scan.
func (s *Service) SetScan(host shell.Host, scan Scan) (*Scan, error) {
	if scan.Folder == "" {
		s.Forget(host.ID(), false)
		return nil, s.Scans.Set(host.ID(), Scan{})
	}
	checked, err := CheckScan(host.Home(), scan)
	if err != nil {
		return nil, err
	}
	s.Forget(host.ID(), false)
	return &checked, s.Scans.Set(host.ID(), checked)
}
