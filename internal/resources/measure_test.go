package resources

import (
	"errors"
	"math"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func TestMeasureSumsEachPanesTreeIntoItsWorkspace(t *testing.T) {
	table, err := Parse(macAnswer)
	if err != nil {
		t.Fatal(err)
	}
	roots := []Root{{Pane: "w1:p1", Workspace: "w1", PID: 410}, {Pane: "w2:p1", Workspace: "w2", PID: 420}, {Pane: "w1:p2", Workspace: "w1", PID: 430}, {Pane: "w3:p1", Workspace: "w3", PID: 999}}
	usage := Measure(table, func(p Proc) float64 { return p.CPU }, roots, true, time.Unix(0, 0))
	if usage.Cores != 8 || usage.MemKB != 16777216 || usage.Unread != 1 || len(usage.Workspaces) != 2 {
		t.Fatalf("usage %+v", usage)
	}
	claude, app := usage.Workspaces[0], usage.Workspaces[1]
	if claude.ID != "w2" || claude.RSSKB != 4096+2097152 || !near(claude.CPU, 60.0/8) || claude.Top.Name != "claude" {
		t.Fatalf("the heaviest workspace first: %+v", claude)
	}
	if app.ID != "w1" || app.RSSKB != 4096+524288+102400+4096 || !near(app.CPU, 27.0/8) || app.Top.Name != "node" || app.Top.RSSKB != 524288 {
		t.Fatalf("a workspace's panes summed: %+v", app)
	}
	if len(app.Terminals) != 2 || app.Terminals[0].Pane != "w1:p1" || app.Terminals[1].Top.Name != "zsh" {
		t.Fatalf("its terminals, the heaviest first: %+v", app.Terminals)
	}
	if usage.Herdr.RSSKB != 40960 || !near(usage.Herdr.CPU, 1.5/8) {
		t.Fatalf("herdr's own, past the panes it holds: %+v", usage.Herdr)
	}
	// Chrome runs outside herdr and counts for nothing
	if usage.Total.RSSKB != claude.RSSKB+app.RSSKB+40960 || !near(usage.Total.CPU, (60+27+1.5)/8) {
		t.Fatalf("the total %+v", usage.Total)
	}
}

// fakeMachine answers the script with one table after another, counting the reads
type fakeMachine struct {
	answers []string
	reads   atomic.Int32
	onRead  func()
}

func (f *fakeMachine) host() *shell.Fake {
	return &shell.Fake{Name: "gog", Remote: true, Answer: func(string) (string, error) {
		if f.onRead != nil {
			f.onRead()
		}
		n := int(f.reads.Add(1)) - 1
		if n >= len(f.answers) {
			n = len(f.answers) - 1
		}
		return f.answers[n], nil
	}}
}

func TestALinuxMachinesCPUIsToldAgainstTheReadingBefore(t *testing.T) {
	FirstGap = 0
	clock := time.Unix(1000, 0)
	later := strings.Replace(strings.Replace(linuxAnswer, "711\x1f710\x1f90000", "711\x1f710\x1f90400", 1), "700\x1f1\x1f5000", "700\x1f1\x1f5020", 1)
	machine := &fakeMachine{answers: []string{linuxAnswer, linuxAnswer, later}, onRead: func() { clock = clock.Add(time.Second) }}
	asked := 0
	s := New(Inputs{
		Panes: func(string) []Pane { return []Pane{{ID: "w1:p1", Workspace: "w1"}, {ID: "w1:p2", Workspace: "w1"}} },
		ShellPID: func(_, pane string) (int, error) {
			asked++
			if pane == "w1:p1" {
				return 710, nil
			}
			return 720, nil
		},
	})
	s.now = func() time.Time { return clock }
	host := machine.host()
	first, err := s.Read("gog", host)
	if err != nil {
		t.Fatal(err)
	}
	if machine.reads.Load() != 2 || !first.CPUKnown || first.Total.CPU != 0 {
		t.Fatalf("a first read samples twice: %d reads, %+v", machine.reads.Load(), first.Total)
	}
	again, _ := s.Read("gog", host)
	if machine.reads.Load() != 2 || again.At != first.At {
		t.Fatal("a fresh reading answers a second asker as it is")
	}
	clock = clock.Add(5 * time.Second)
	next, err := s.Read("gog", host)
	if err != nil {
		t.Fatal(err)
	}
	// the reading before was taken at 1002 s, this one at 1008 s: node spent 400 ticks of 600, two thirds of a core —
	// a sixth of four — and herdr 20
	node, herdr := 400.0/600*100/4, 20.0/600*100/4
	if !near(next.Workspaces[0].CPU, node) || !near(next.Herdr.CPU, herdr) || !near(next.Total.CPU, node+herdr) {
		t.Fatalf("the CPU told against the reading before: %+v, herdr %+v", next.Workspaces[0], next.Herdr)
	}
	if asked != 2 {
		t.Fatalf("herdr asked %d times for two panes' shells", asked)
	}
}

func TestAPaneWhoseShellIsGoneIsAskedAgainNowAndThen(t *testing.T) {
	clock := time.Unix(1000, 0)
	machine := &fakeMachine{answers: []string{macAnswer}}
	asked := 0
	pid := 0
	s := New(Inputs{
		Panes: func(string) []Pane { return []Pane{{ID: "w1:p1", Workspace: "w1"}} },
		ShellPID: func(string, string) (int, error) {
			asked++
			if pid == 0 {
				return 0, errors.New("pane_not_found")
			}
			return pid, nil
		},
	})
	s.now = func() time.Time { return clock }
	host := machine.host()
	for i := 0; i < 3; i++ {
		usage, err := s.Read("mac", host)
		if err != nil {
			t.Fatal(err)
		}
		if usage.Unread != 1 {
			t.Fatalf("a pane herdr names no shell for is unread: %+v", usage)
		}
		clock = clock.Add(5 * time.Second)
	}
	if asked != 1 {
		t.Fatalf("herdr asked %d times within %s", asked, AskAgainAfter)
	}
	pid = 410
	clock = clock.Add(AskAgainAfter)
	usage, _ := s.Read("mac", host)
	if asked != 2 || usage.Unread != 0 || len(usage.Workspaces) != 1 {
		t.Fatalf("asked again after a while: %d, %+v", asked, usage)
	}
}
