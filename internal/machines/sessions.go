package machines

import (
	"log"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
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
	procs      map[processKey]*process
	reading    bool
	complained time.Time
}

// processKey is one process however its pid is given out again: the terminal it holds and the count of the moment it
// was seen take it.
type processKey struct {
	terminal string
	held     uint64
	pid      int
}

// process is an agent's process as last read: its kind and folder, its session ("" for none found), when it was last
// read and when a read was last tried, whatever came of it.
type process struct {
	running         transcript.Running
	session         string
	readAt, triedAt time.Time
	asked           time.Time
}

func newProcessSessions(name string, read func([]transcript.Running) (map[int]string, error)) *processSessions {
	return &processSessions{name: name, read: read, fresh: sessionsFresh, forget: sessionsForget, procs: map[processKey]*process{}}
}

// of is the core adapter's SessionLookup: what the last read found of the process, a read booked when it is new or due.
func (s *processSessions) of(asked core.AgentProcess) *herdr.AgentSession {
	now := time.Now()
	running := transcript.Running{PID: asked.PID, Agent: asked.Agent, CWD: asked.Cwd, Seen: asked.Seen}
	key := processKey{terminal: asked.Terminal, held: asked.Held, pid: asked.PID}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.procs[key]
	if p == nil || p.running != running {
		p = &process{}
		s.procs[key] = p
	}
	p.running, p.asked = running, now
	if now.Sub(p.triedAt) >= s.fresh && !s.reading {
		s.reading = true
		since := now.Add(-s.fresh)
		go s.readDue(since, false)
	}
	if p.session == "" {
		return nil
	}
	return &herdr.AgentSession{Kind: "id", Value: p.session}
}

// holders is, pid by pid, the process that holds it now: of two asked about under one pid, the one seen take its
// terminal later (a pid never goes back to an earlier process). The other gave the pid up and is never read again,
// since a look at the pid sees the one holding it.
func (s *processSessions) holders() map[int]*process {
	held := map[int]processKey{}
	for key := range s.procs {
		if other, ok := held[key.pid]; !ok || s.tookLater(key, other) {
			held[key.pid] = key
		}
	}
	holders := make(map[int]*process, len(held))
	for pid, key := range held {
		holders[pid] = s.procs[key]
	}
	return holders
}

// tookLater says whether the process under key took its pid after the one under other.
func (s *processSessions) tookLater(key, other processKey) bool {
	if key.held != other.held {
		return key.held > other.held
	}
	return s.procs[key].asked.After(s.procs[other].asked)
}

// readNow reads every process asked about lately again before it answers, for a step about to stop an agent and
// resume it on its session: a snapshot answers what the last read found, and a /resume may have moved the agent since.
func (s *processSessions) readNow() error {
	return s.readDue(time.Now(), true)
}

// readDue reads every pid's holder whose read was last tried before since — read, when the answer must be one read
// since (another caller's failed try is none) — and forgets the processes nobody asks about; a failure keeps the
// answers read before, and an answer for a pid another process took meanwhile is dropped.
func (s *processSessions) readDue(since time.Time, read bool) error {
	s.readMu.Lock()
	defer s.readMu.Unlock()
	s.mu.Lock()
	now := time.Now()
	for key, p := range s.procs {
		if now.Sub(p.asked) > s.forget {
			delete(s.procs, key)
		}
	}
	var due []*process
	for _, p := range s.holders() {
		if !p.triedAt.After(since) || read && !p.readAt.After(since) {
			due = append(due, p)
		}
	}
	look := make([]transcript.Running, len(due))
	for i, p := range due {
		look[i] = p.running
	}
	s.mu.Unlock()
	found, err := s.read(look)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reading = false
	if err != nil && time.Since(s.complained) > time.Hour {
		s.complained = time.Now()
		log.Printf("%s: the agents' sessions could not be read off their processes: %v", s.name, err)
	}
	holders := s.holders()
	for _, p := range due {
		if holders[p.running.PID] != p {
			continue // another process was asked about under its pid meanwhile: what was read may be that one's
		}
		p.triedAt = now
		if err == nil {
			p.session, p.readAt = found[p.running.PID], now
		}
	}
	return err
}
