package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/radar"
)

func TestATaskPaneWithoutItsAgentOrMachineHoldsNoSlot(t *testing.T) {
	working := func() radar.Display { return radar.Display{State: herdr.StatusWorking} }
	agent := &herdr.PaneInfo{PaneID: "p1", Agent: "claude"}
	shell := &herdr.PaneInfo{PaneID: "p1"}
	snap := &herdr.Snapshot{}
	for _, c := range []struct {
		name    string
		pane    *herdr.PaneInfo
		snap    *herdr.Snapshot
		inFleet bool
		state   string
		ok      bool
	}{
		{"the agent at work", agent, snap, true, herdr.StatusWorking, true},
		{"the agent left the pane", shell, snap, true, herdr.StatusIdle, true},
		{"the machine not seen yet", nil, nil, true, herdr.StatusUnknown, true},
		{"the machine gone from the fleet", nil, nil, false, "", false},
		{"the pane closed", nil, snap, true, "", false},
	} {
		display, ok := readTaskPane(c.pane, c.snap, c.inFleet, working)
		if display.State != c.state || ok != c.ok {
			t.Errorf("%s: %q %v", c.name, display.State, ok)
		}
	}
}

// A machine on remuda-core whose core is missing keeps its master, so it reads as connected; the queue and the placer
// still start no card there, which would only be refused.
func TestAMachineWithoutItsCoreRunsNoAgents(t *testing.T) {
	stand := t.TempDir()
	stub := "#!/bin/sh\ncase \" $* \" in\n*\" -G \"*) exit 1 ;;\n*\" -M \"*) while [ \"$1\" != -S ]; do shift; done; : > \"$2\"; exec sleep 30 ;;\n*\" -O \"*) exit 0 ;;\nesac\nfor line; do :; done\neval \"$line\"\n"
	if err := os.WriteFile(filepath.Join(stand, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stand+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", t.TempDir())
	fleet := machines.NewManager(t.TempDir(), nil, true)
	defer fleet.Close()
	mc, err := fleet.Add("Studio", "queue-test-"+filepath.Base(stand))
	if err != nil {
		t.Fatal(err)
	}
	for start := time.Now(); ; time.Sleep(20 * time.Millisecond) {
		if state, _ := mc.State(); state == machines.StateConnected {
			break
		}
		if time.Since(start) > 10*time.Second {
			t.Fatal("the machine never connected over its master")
		}
	}
	if runsAgents(mc) {
		t.Fatal("a machine without its remuda-core counts as one an agent can start on")
	}
}
