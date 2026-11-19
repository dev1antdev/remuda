package app

import (
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr"
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
