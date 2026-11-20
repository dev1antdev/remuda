package machines

import (
	"errors"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/core/coretest"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/transcript"
)

// sessionReads is a machine's SessionsOf on paper: the answers it holds now, and every list it was asked for.
type sessionReads struct {
	mu      sync.Mutex
	answers map[int]string
	fail    error
	asked   [][]transcript.Running
}

func (r *sessionReads) read(procs []transcript.Running) (map[int]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.asked = append(r.asked, procs)
	found := map[int]string{}
	for _, p := range procs {
		if id, ok := r.answers[p.PID]; ok {
			found[p.PID] = id
		}
	}
	return found, r.fail
}

func (r *sessionReads) reads() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.asked)
}

// ask is the lookup as a snapshot makes it: the agent's process in its terminal, which its agent took at the start.
func ask(s *processSessions, agent string, pid int) *herdr.AgentSession {
	return s.of(core.AgentProcess{Agent: agent, PID: pid, Cwd: "/work/api", Terminal: "t" + strconv.Itoa(pid), Since: agentsTook})
}

var agentsTook = time.Now()

// sessionAfter asks until the lookup answers want ("" for none), the reads it books running meanwhile.
func sessionAfter(t *testing.T, s *processSessions, agent string, pid int, want string) {
	t.Helper()
	for start := time.Now(); time.Since(start) < 5*time.Second; time.Sleep(5 * time.Millisecond) {
		got := ask(s, agent, pid)
		if want == "" && got == nil || got != nil && got.Value == want {
			return
		}
	}
	t.Fatalf("%s %d never read as %q", agent, pid, want)
}

func TestAnAgentsSessionIsReadOffItsProcessWhenNewAndAgainOnceDue(t *testing.T) {
	reads := &sessionReads{answers: map[int]string{100: "sid-a"}}
	s := newProcessSessions("gog", reads.read)
	s.fresh = time.Hour

	if got := ask(s, "claude", 100); got != nil {
		t.Fatalf("a process never read answered %+v at once", got)
	}
	sessionAfter(t, s, "claude", 100, "sid-a")
	reads.mu.Lock()
	reads.answers[200] = "sid-b"
	reads.mu.Unlock()
	sessionAfter(t, s, "codex", 200, "sid-b") // a new process is read at once, the answer it has not changing
	n := reads.reads()
	ask(s, "claude", 100)
	time.Sleep(20 * time.Millisecond)
	if reads.reads() != n {
		t.Fatal("an answer still fresh was read again")
	}

	s.mu.Lock()
	s.fresh = 0 // every answer due: a /clear started another conversation in the same process
	s.mu.Unlock()
	reads.mu.Lock()
	reads.answers[100], reads.fail = "sid-c", nil
	reads.mu.Unlock()
	sessionAfter(t, s, "claude", 100, "sid-c")

	reads.mu.Lock()
	reads.fail = errors.New("ssh: connection lost")
	reads.mu.Unlock()
	time.Sleep(20 * time.Millisecond)
	if got := ask(s, "claude", 100); got == nil || got.Value != "sid-c" {
		t.Fatalf("a failed read lost what was known: %+v", got)
	}
	if got := ask(s, "codex", 100); got != nil {
		t.Fatalf("another agent under the same pid took the old one's session: %+v", got)
	}
}

// A sleep or a restart about to stop an agent reads its session afresh, though the last answer is not due: a /resume
// moved the agent to another conversation in the same process; a read that fails says so and keeps what was known.
func TestAFreshReadAnswersTheConversationTheAgentRunsNow(t *testing.T) {
	reads := &sessionReads{answers: map[int]string{100: "sid-a"}}
	s := newProcessSessions("gog", reads.read)
	s.fresh = time.Hour
	sessionAfter(t, s, "claude", 100, "sid-a")
	reads.mu.Lock()
	reads.answers[100] = "sid-b"
	reads.mu.Unlock()
	if got := ask(s, "claude", 100); got == nil || got.Value != "sid-a" {
		t.Fatalf("the answer was read again before it was due: %+v", got)
	}
	if err := s.readNow(); err != nil {
		t.Fatal(err)
	}
	if got := ask(s, "claude", 100); got == nil || got.Value != "sid-b" {
		t.Fatalf("after a fresh read: %+v", got)
	}
	reads.mu.Lock()
	reads.answers[100], reads.fail = "sid-c", errors.New("ssh: connection lost")
	reads.mu.Unlock()
	if err := s.readNow(); err == nil {
		t.Fatal("a fresh read that failed answered nothing")
	}
	if got := ask(s, "claude", 100); got == nil || got.Value != "sid-b" {
		t.Fatalf("a failed fresh read lost what was known: %+v", got)
	}

	// a fresh read asked for before that failed one began, queued behind it: the failure is no read for it either
	queued := time.Now()
	time.Sleep(time.Millisecond)
	if err := s.readNow(); err == nil {
		t.Fatal("the read in front did not fail")
	}
	reads.mu.Lock()
	reads.fail = nil
	reads.mu.Unlock()
	if err := s.readDue(queued, true); err != nil {
		t.Fatal(err)
	}
	if got := ask(s, "claude", 100); got == nil || got.Value != "sid-c" {
		t.Fatalf("the queued fresh read took the failed one for its own: %+v", got)
	}
}

// A pid given out again names another process: one in another terminal, or in the same terminal once another agent
// took it, never answers the session read for the process before it, and is read at once.
func TestAPidGivenOutAgainIsNotTheProcessBeforeIt(t *testing.T) {
	reads := &sessionReads{answers: map[int]string{100: "sid-a"}}
	s := newProcessSessions("gog", reads.read)
	s.fresh = time.Hour
	sessionAfter(t, s, "claude", 100, "sid-a")
	reads.mu.Lock()
	reads.answers[100] = "sid-b"
	reads.mu.Unlock()

	later := []core.AgentProcess{
		{Agent: "claude", PID: 100, Cwd: "/work/api", Terminal: "t7", Since: agentsTook},
		{Agent: "claude", PID: 100, Cwd: "/work/api", Terminal: "t100", Since: agentsTook.Add(time.Minute)},
	}
	for _, process := range later {
		if got := s.of(process); got != nil {
			t.Fatalf("%+v answered the session of the process before it: %+v", process, got)
		}
		for start := time.Now(); ; time.Sleep(5 * time.Millisecond) {
			if got := s.of(process); got != nil && got.Value == "sid-b" {
				break
			}
			if time.Since(start) > 5*time.Second {
				t.Fatalf("%+v was never read", process)
			}
		}
	}
	if got := ask(s, "claude", 100); got == nil || got.Value != "sid-a" {
		t.Fatalf("the process read first lost its answer: %+v", got)
	}
}

// Once the process that took a pid later is known, the one before it is never read again, though a snapshot still
// names it: a look at the pid would see the later one's session.
func TestTheProcessBeforeAPidsLaterOneIsNeverReadAgain(t *testing.T) {
	reads := &sessionReads{answers: map[int]string{100: "sid-a"}}
	s := newProcessSessions("gog", reads.read)
	s.fresh = time.Hour
	before := core.AgentProcess{Agent: "claude", PID: 100, Cwd: "/work/api", Terminal: "t1", Since: agentsTook}
	after := core.AgentProcess{Agent: "claude", PID: 100, Cwd: "/work/api", Terminal: "t2", Since: agentsTook.Add(time.Minute)}
	for start := time.Now(); ; time.Sleep(5 * time.Millisecond) {
		if got := s.of(before); got != nil && got.Value == "sid-a" {
			break
		}
		if time.Since(start) > 5*time.Second {
			t.Fatal("the first process was never read")
		}
	}
	reads.mu.Lock()
	reads.answers[100] = "sid-b"
	reads.mu.Unlock()
	for start := time.Now(); ; time.Sleep(5 * time.Millisecond) {
		if got := s.of(after); got != nil && got.Value == "sid-b" {
			break
		}
		if time.Since(start) > 5*time.Second {
			t.Fatal("the later process was never read")
		}
	}

	// the one before comes due while the later one is fresh, and a snapshot still naming it asks about it last
	beforeKey := processKey{terminal: "t1", since: agentsTook.UnixMilli(), pid: 100}
	s.mu.Lock()
	s.procs[beforeKey].triedAt = time.Now().Add(-2 * time.Hour)
	s.procs[beforeKey].asked = time.Now().Add(time.Second)
	s.mu.Unlock()
	reads.mu.Lock()
	reads.answers[100] = "sid-c"
	reads.mu.Unlock()
	if err := s.readDue(time.Now().Add(-time.Hour), false); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	got := s.procs[beforeKey].session
	s.mu.Unlock()
	if got != "sid-a" {
		t.Fatalf("the process before took a look at its pid's later holder: %q", got)
	}
}

// Each fill of the mirror finds the agents running anew: one found by the next fill under the same terminal and pid
// may be another process, so it is read at once and never answers the session found before.
func TestAMirrorFilledAgainReadsTheProcessItFinds(t *testing.T) {
	reads := &sessionReads{answers: map[int]string{100: "sid-a"}}
	s := newProcessSessions("gog", reads.read)
	s.fresh = time.Hour
	firstFill := core.AgentProcess{Agent: "claude", PID: 100, Cwd: "/work/api", Terminal: "t100", Since: agentsTook}
	for start := time.Now(); ; time.Sleep(5 * time.Millisecond) {
		if got := s.of(firstFill); got != nil && got.Value == "sid-a" {
			break
		}
		if time.Since(start) > 5*time.Second {
			t.Fatal("the process the first fill found was never read")
		}
	}
	reads.mu.Lock()
	reads.answers[100] = "sid-b"
	reads.mu.Unlock()
	nextFill := firstFill
	nextFill.Since = agentsTook.Add(time.Minute)
	if got := s.of(nextFill); got != nil {
		t.Fatalf("the next fill's process answered the session found before: %+v", got)
	}
	for start := time.Now(); ; time.Sleep(5 * time.Millisecond) {
		if got := s.of(nextFill); got != nil && got.Value == "sid-b" {
			break
		}
		if time.Since(start) > 5*time.Second {
			t.Fatal("the next fill's process was never read")
		}
	}
}

func TestProcessesNobodyAsksAboutAreForgotten(t *testing.T) {
	reads := &sessionReads{answers: map[int]string{100: "sid-a", 200: "sid-b"}}
	s := newProcessSessions("gog", reads.read)
	s.fresh = 0
	sessionAfter(t, s, "claude", 100, "sid-a")
	sessionAfter(t, s, "claude", 200, "sid-b")
	s.mu.Lock()
	s.procs[processKey{terminal: "t200", since: agentsTook.UnixMilli(), pid: 200}].asked = time.Now().Add(-sessionsForget - time.Minute)
	s.mu.Unlock()
	for start := time.Now(); ; time.Sleep(5 * time.Millisecond) {
		s.mu.Lock()
		_, kept := s.procs[processKey{terminal: "t200", since: agentsTook.UnixMilli(), pid: 200}]
		s.mu.Unlock()
		if !kept {
			break
		}
		if time.Since(start) > 5*time.Second {
			t.Fatal("a process nobody asked about is kept")
		}
		ask(s, "claude", 100)
	}
}

// On a throwaway core, a stand-in Claude Code that keeps its session file where Claude Code does is named by it in the
// machine's snapshot, read off its process.
func TestAnAgentOnTheLocalCoreIsNamedByItsSessionFile(t *testing.T) {
	const sid = "9b1c2d3e-4f50-4a6b-8c7d-0e1f2a3b4c5d"
	coretest.StandInClaude(t, sid)
	c := coretest.Server(t)
	c.Use(t)
	m := NewCoreManager(t.TempDir(), "", nil, false)
	defer m.Close()
	mc, err := m.Get(LocalID)
	if err != nil {
		t.Fatal(err)
	}
	client, _, err := mc.Ready()
	if err != nil {
		t.Fatal(err)
	}
	folder, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	made, err := client.WorkspaceCreate(folder, "")
	if err != nil {
		t.Fatal(err)
	}
	pane := made.RootPane.PaneID
	for start := time.Now(); time.Since(start) < 10*time.Second; time.Sleep(200 * time.Millisecond) {
		if err = client.AgentStart(pane, "claude"); !herdr.IsCode(err, "agent_pane_busy") {
			break
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	var p *herdr.PaneInfo
	for start := time.Now(); time.Since(start) < 15*time.Second; time.Sleep(200 * time.Millisecond) {
		if snap, err := client.SessionSnapshot(); err == nil {
			if p = snap.Pane(pane); p != nil && p.AgentSession != nil && p.AgentSession.Value == sid {
				return
			}
		}
	}
	t.Fatalf("the pane never named its session: %+v", p)
}
