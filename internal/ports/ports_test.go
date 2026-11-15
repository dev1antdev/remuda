package ports

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

func line(fields ...string) string { return strings.Join(fields, "\x1f") + "\n" }

// gog's listeners: vite in a worktree, next in the repository, a server outside every workspace, remuda's own
var gogAnswer = line("5173", "[::1]", "4100", "node", "/home/me/.herdr/worktrees/app/task-1", "/home/me/.herdr/worktrees/app/task-1", "node node_modules/.bin/vite") +
	line("3000", "*", "4200", "node", "/home/me/code/app/web", "/home/me/code/app", "node node_modules/.bin/next dev") +
	line("8000", "127.0.0.1", "4300", "python3", "/home/me/scratch", "/home/me", "python3 -m http.server 8000") +
	line("7317", "127.0.0.1", "900", "remuda", "/home/me", "/", "remuda -addr 127.0.0.1:7317")

var folders = map[string]string{"w1": "/home/me/code/app", "w2": "/home/me/.herdr/worktrees/app/task-1"}

// recorded answers the script, noting each one and which pids it was told it knows
type recorded struct {
	mu      sync.Mutex
	answer  string
	err     error
	scripts []string
}

func (r *recorded) host() *shell.Fake {
	return &shell.Fake{Name: "gog", Remote: true, Answer: func(script string) (string, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.scripts = append(r.scripts, script)
		return r.answer, r.err
	}}
}

func (r *recorded) reads() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.scripts)
}

// settle runs a sweep and waits for its reads to end, what they tell OnChange included
func settle(t *testing.T, tracker *Tracker) {
	t.Helper()
	tracker.Sweep()
	ended := make(chan struct{})
	go func() {
		tracker.reads.Wait()
		close(ended)
	}()
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("the reads did not end")
	}
}

func TestTheTrackerReadsOnlyWhileAPageLooks(t *testing.T) {
	gog := &recorded{answer: gogAnswer}
	host := gog.host()
	clock := time.Unix(1000, 0)
	tracker := New(func() []Target { return []Target{{ID: "gog", Host: host, Folders: folders}} }, func() map[int]bool { return map[int]bool{7317: true} })
	tracker.now = func() time.Time { return clock }
	changes := 0
	tracker.OnChange = func() { changes++ }

	settle(t, tracker)
	if gog.reads() != 0 {
		t.Fatal("nothing is read for a page nobody looks at")
	}
	tracker.Touch()
	settle(t, tracker)
	if gog.reads() != 1 || changes != 1 {
		t.Fatalf("a look reads the ports: %d reads, %d changes", gog.reads(), changes)
	}
	app := tracker.For("gog", "w1")
	if len(app) != 1 || app[0].Port != 3000 || app[0].Process != "node" || app[0].Label != "next" || app[0].PID != 4200 || app[0].Address != "*:3000" || app[0].Workspace != "" {
		t.Fatalf("the repository's ports %+v", app)
	}
	if task := tracker.For("gog", "w2"); len(task) != 1 || task[0].Address != "localhost:5173" {
		t.Fatalf("the worktree's ports %+v", task)
	}
	whole, ok := tracker.Of("gog")
	if !ok || len(whole.Ports) != 3 || whole.Ports[2].Port != 8000 || whole.Ports[2].Workspace != "" {
		t.Fatalf("the machine's ports, outside the workspaces too, remuda's never: %+v", whole)
	}

	clock = clock.Add(5 * time.Second)
	settle(t, tracker)
	if gog.reads() != 1 {
		t.Fatal("the roster's pace is slower than the status bar's")
	}
	tracker.Hurry("gog")
	settle(t, tracker)
	if gog.reads() != 2 || changes != 1 {
		t.Fatalf("the status bar's pace: %d reads, %d changes for ports that stood", gog.reads(), changes)
	}
	if last := gog.scripts[1]; !strings.Contains(last, `known=" 900 4100 4200 4300 "`) {
		t.Fatal("a second read looks up again the folders it knows")
	}

	gog.err, gog.answer = errors.New("ssh: connection lost"), ""
	clock = clock.Add(QuickEvery)
	settle(t, tracker)
	whole, _ = tracker.Of("gog")
	if whole.Error == "" || len(whole.Ports) != 3 || changes != 2 {
		t.Fatalf("a failed read keeps the ports before it and says why: %+v", whole)
	}

	clock = clock.Add(StaleAfter + time.Second)
	if tracker.For("gog", "w1") != nil {
		t.Fatal("a reading nobody refreshed for long says nothing on the roster")
	}
}

// A page back after a while away: the reading went stale, so the roster carried no ports and the machine's reading
// said it was not read; the next read brings them back to the roster at once, though they did not move.
func TestAReadAfterAStaleOneMovesTheRoster(t *testing.T) {
	gog := &recorded{answer: gogAnswer}
	host := gog.host()
	clock := time.Unix(1000, 0)
	tracker := New(func() []Target { return []Target{{ID: "gog", Host: host, Folders: folders}} }, func() map[int]bool { return nil })
	tracker.now = func() time.Time { return clock }
	changes := 0
	tracker.OnChange = func() { changes++ }
	tracker.Touch()
	settle(t, tracker)
	if changes != 1 || tracker.For("gog", "w1") == nil {
		t.Fatalf("the first read: %d changes", changes)
	}

	clock = clock.Add(StaleAfter + time.Second)
	if tracker.For("gog", "w1") != nil {
		t.Fatal("a stale reading is on the roster")
	}
	if _, read := tracker.Of("gog"); read {
		t.Fatal("a stale reading counts as read: the status bar would count 0 ports from a roster that carries none")
	}
	tracker.Touch()
	settle(t, tracker)
	if changes != 2 {
		t.Fatalf("the ports came back to the roster with %d changes told: the page keeps none until something else moves", changes)
	}
	if _, read := tracker.Of("gog"); !read || tracker.For("gog", "w1") == nil {
		t.Fatal("the fresh reading")
	}

	// away again, and the first read back fails: the old ports stay as old as they are, never on the roster as fresh
	gog.mu.Lock()
	gog.err, gog.answer = errors.New("ssh: connection lost"), ""
	gog.mu.Unlock()
	clock = clock.Add(StaleAfter + time.Second)
	tracker.Touch()
	settle(t, tracker)
	if tracker.For("gog", "w1") != nil {
		t.Fatal("a failed read put ports read minutes ago on the roster as fresh")
	}
	if whole, read := tracker.Of("gog"); read || whole.Error == "" {
		t.Fatalf("a failed read after a stale one: read %v, %+v", read, whole)
	}
}

// The status bar shows one machine's ports: that machine is read at its pace, every other at the roster's, so a bar
// on the Raspberry Pi's page does not triple the looks at every machine of the fleet.
func TestTheStatusBarHurriesItsMachineAlone(t *testing.T) {
	counts := map[string]*recorded{"gog": {}, "mac": {}, "pi": {}}
	var targets []Target
	for _, id := range []string{"gog", "mac", "pi"} {
		targets = append(targets, Target{ID: id, Host: counts[id].host()})
	}
	for _, focused := range []bool{true, false} {
		for _, r := range counts {
			r.scripts = nil
		}
		tracker := New(func() []Target { return targets }, func() map[int]bool { return nil })
		clock := time.Unix(1000, 0)
		tracker.now = func() time.Time { return clock }
		every := 5 // the bar's poll while the window has the focus, else 30 s
		if !focused {
			every = 30
		}
		for second := 0; second < 60; second++ {
			if second%3 == 0 {
				tracker.Touch()
			}
			if second%every == 0 {
				tracker.Hurry("gog")
			}
			settle(t, tracker)
			clock = clock.Add(time.Second)
		}
		gog, mac, pi := counts["gog"].reads(), counts["mac"].reads(), counts["pi"].reads()
		if focused && gog != 12 || !focused && gog != 8 {
			t.Fatalf("focused %v: the bar's machine is read %d times a minute", focused, gog)
		}
		if mac != 4 || pi != 4 {
			t.Fatalf("focused %v: the machines the bar does not show are read %d and %d times a minute, the roster's pace is 4", focused, mac, pi)
		}
	}
}

func TestStopSignalsOnlyThePidThatListensThere(t *testing.T) {
	var scripts []string
	host := &shell.Fake{Name: "gog", Remote: true, Answer: func(script string) (string, error) {
		scripts = append(scripts, script)
		if strings.HasPrefix(script, "kill ") {
			return "", nil
		}
		return gogAnswer, nil
	}}
	stopped, err := Stop(host, map[int]bool{7317: true}, 4200, 3000)
	if err != nil || stopped.Command != "node" || stopped.Port != 3000 {
		t.Fatalf("stopped %+v: %v", stopped, err)
	}
	if last := scripts[len(scripts)-1]; last != "kill -TERM 4200 2>&1" {
		t.Fatalf("the signal sent: %q", last)
	}
	scripts = nil
	for _, ask := range [][2]int{{4200, 5173}, {900, 7317}, {1, 3000}, {4999, 3000}} {
		if _, err := Stop(host, map[int]bool{7317: true}, ask[0], ask[1]); !errors.Is(err, ErrNotListening) {
			t.Fatalf("pid %d on %d: %v", ask[0], ask[1], err)
		}
	}
	for _, s := range scripts {
		if strings.HasPrefix(s, "kill") {
			t.Fatalf("a pid that does not listen there was signalled: %q", s)
		}
	}
	// Claude Code holds a loopback port in its workspace's folder: the agent herdr runs is never stopped from here
	agent := &shell.Fake{Name: "gog", Remote: true, Answer: func(script string) (string, error) {
		if strings.HasPrefix(script, "kill ") {
			t.Fatalf("the agent was signalled: %q", script)
		}
		return gogAnswer + line("49778", "127.0.0.1", "1813", "2.1.323", "/home/me/code/app", "/home/me/code/app", "claude --resume 0b6f2c1e-7d0a-4c55-9a1e-3f2d8c4b5a61"), nil
	}}
	if _, err := Stop(agent, nil, 1813, 49778); !errors.Is(err, ErrNotListening) {
		t.Fatalf("an agent's own port: %v", err)
	}
}

func TestAddressOf(t *testing.T) {
	for bound, want := range map[string]string{"127.0.0.1": "localhost:5173", "::1": "localhost:5173", "*": "*:5173", "::": "*:5173", "192.168.1.5": "192.168.1.5:5173", "fe80::1": "[fe80::1]:5173"} {
		if got := AddressOf(bound, 5173); got != want {
			t.Errorf("%s: %s, want %s", bound, got, want)
		}
	}
}
