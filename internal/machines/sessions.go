package machines

import (
	"log"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/transcript"
)

// How long a process's session answer stands before it is read again — a /clear or a /resume starts another
// conversation in the same process — and how long a process no snapshot asked about is kept.
const (
	sessionsFresh  = 30 * time.Second
	sessionsForget = 2 * time.Minute
)

// processSessions names the conversation of each agent on a remuda-core machine, which reports none, by its process
// (transcript.Reader.SessionsOf): a process new to it is read at once, every answer again once it is sessionsFresh
// old, all of them in one script on the machine, off the snapshot's path, which only reads what was last found.
type processSessions struct {
	read          func([]transcript.Running) (map[int]string, error)
	name          string // the machine's, for the log
	fresh, forget time.Duration

	readMu     sync.Mutex // one read on the machine at a time
	mu         sync.Mutex
	procs      map[int]*process
	reading    bool
	complained time.Time
}

// process is an agent's process as last read: its kind and folder, its session ("" for none found) and when.
type process struct {
	running transcript.Running
	session string
	readAt  time.Time
	asked   time.Time
}

func newProcessSessions(name string, read func([]transcript.Running) (map[int]string, error)) *processSessions {
	return &processSessions{name: name, read: read, fresh: sessionsFresh, forget: sessionsForget, procs: map[int]*process{}}
}

// of is the core adapter's SessionLookup: what the last read found of the process, a read booked when it is new or due.
func (s *processSessions) of(agent string, pid int, cwd string) *herdr.AgentSession {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.procs[pid]
	if p == nil || p.running.Agent != agent {
		p = &process{}
		s.procs[pid] = p
	}
	p.running, p.asked = transcript.Running{PID: pid, Agent: agent, CWD: cwd}, now
	if now.Sub(p.readAt) >= s.fresh && !s.reading {
		s.reading = true
		since := now.Add(-s.fresh)
		go s.readDue(since)
	}
	if p.session == "" {
		return nil
	}
	return &herdr.AgentSession{Kind: "id", Value: p.session}
}

// readNow reads every process asked about lately again before it answers, for a step about to stop an agent and
// resume it on its session: a snapshot answers what the last read found, and a /resume may have moved the agent since.
func (s *processSessions) readNow() error {
	return s.readDue(time.Now())
}

// readDue reads every process asked about lately whose answer was read before since, and forgets the ones nobody asks
// about; a failure keeps the answers read before.
func (s *processSessions) readDue(since time.Time) error {
	s.readMu.Lock()
	defer s.readMu.Unlock()
	s.mu.Lock()
	now := time.Now()
	var due []transcript.Running
	for pid, p := range s.procs {
		switch {
		case now.Sub(p.asked) > s.forget:
			delete(s.procs, pid)
		case p.readAt.Before(since) || p.readAt.Equal(since):
			due = append(due, p.running)
		}
	}
	s.mu.Unlock()
	found, err := s.read(due)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reading = false
	if err != nil && time.Since(s.complained) > time.Hour {
		s.complained = time.Now()
		log.Printf("%s: the agents' sessions could not be read off their processes: %v", s.name, err)
	}
	for _, r := range due {
		if p := s.procs[r.PID]; p != nil && p.running.Agent == r.Agent {
			p.readAt = now
			if err == nil {
				p.session = found[r.PID]
			}
		}
	}
	return err
}
