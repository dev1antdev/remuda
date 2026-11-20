package machines

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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

// sessionAfter asks until the lookup answers want ("" for none), the reads it books running meanwhile.
func sessionAfter(t *testing.T, s *processSessions, agent string, pid int, want string) {
	t.Helper()
	for start := time.Now(); time.Since(start) < 5*time.Second; time.Sleep(5 * time.Millisecond) {
		got := s.of(agent, pid, "/work/api")
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

	if got := s.of("claude", 100, "/work/api"); got != nil {
		t.Fatalf("a process never read answered %+v at once", got)
	}
	sessionAfter(t, s, "claude", 100, "sid-a")
	if got := s.of("gemini", 300, "/work/api"); got != nil || s.of("claude", 1, "/") != nil {
		t.Fatal("an agent the lookup cannot read, or no process, is asked about")
	}
	reads.mu.Lock()
	reads.answers[200] = "sid-b"
	reads.mu.Unlock()
	sessionAfter(t, s, "codex", 200, "sid-b") // a new process is read at once, the answer it has not changing
	n := reads.reads()
	s.of("claude", 100, "/work/api")
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
	if got := s.of("claude", 100, "/work/api"); got == nil || got.Value != "sid-c" {
		t.Fatalf("a failed read lost what was known: %+v", got)
	}
	if got := s.of("codex", 100, "/work/api"); got != nil {
		t.Fatalf("another agent under the same pid took the old one's session: %+v", got)
	}
}

func TestProcessesNobodyAsksAboutAreForgotten(t *testing.T) {
	reads := &sessionReads{answers: map[int]string{100: "sid-a", 200: "sid-b"}}
	s := newProcessSessions("gog", reads.read)
	s.fresh = 0
	sessionAfter(t, s, "claude", 100, "sid-a")
	sessionAfter(t, s, "claude", 200, "sid-b")
	s.mu.Lock()
	s.procs[200].asked = time.Now().Add(-sessionsForget - time.Minute)
	s.mu.Unlock()
	for start := time.Now(); ; time.Sleep(5 * time.Millisecond) {
		s.mu.Lock()
		_, kept := s.procs[200]
		s.mu.Unlock()
		if !kept {
			break
		}
		if time.Since(start) > 5*time.Second {
			t.Fatal("a process nobody asked about is kept")
		}
		s.of("claude", 100, "/work/api")
	}
}

// On a throwaway core, a stand-in Claude Code that keeps its session file where Claude Code does is named by it in the
// machine's snapshot, read off its process.
func TestAnAgentOnTheLocalCoreIsNamedByItsSessionFile(t *testing.T) {
	const sid = "9b1c2d3e-4f50-4a6b-8c7d-0e1f2a3b4c5d"
	bin := t.TempDir()
	script := `#!/bin/sh
mkdir -p "$HOME/.claude/sessions"
printf '{"pid":%s,"sessionId":"` + sid + `","cwd":"%s"}' $$ "$(pwd -P)" > "$HOME/.claude/sessions/$$.json"
printf '\033]7501;state=idle:app=claude-code\033\\'
while IFS= read -r line; do printf 'got: %s\n' "$line"; done
`
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
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
