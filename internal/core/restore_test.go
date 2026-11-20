package core_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/core/coretest"
	"github.com/dev1antdev/remuda/internal/herdr"
)

// startIn starts the stand-in agent in a pane, waiting out a shell still coming up, and waits for its first word.
func startIn(t *testing.T, adapter *core.Adapter, pane string, args []string) {
	t.Helper()
	var err error
	for start := time.Now(); time.Since(start) < 10*time.Second; time.Sleep(200 * time.Millisecond) {
		if err = adapter.AgentStartWith(pane, "claude", "claude", args, 20*time.Second); !herdr.IsCode(err, "agent_pane_busy") {
			break
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, adapter, func(s *herdr.Snapshot) bool {
		p := s.Pane(pane)
		return p != nil && p.Agent == "claude" && p.AgentStatus == herdr.StatusIdle && !p.Restored
	})
}

// A core's stop parks the shells of remuda's panes and its next start gives them new ones in their folders, as herdr
// restores its panes after a reboot: the agents that ran there are gone, the panes stand where they were, marked
// restored so the restart memory lists them, and the agents come back in them on their sessions.
func TestPanesComeBackAsShellsAfterTheCoreRestartsAndTheirAgentsResumeInThem(t *testing.T) {
	standInAgent(t)
	c := coretest.Server(t)
	adapter := adapterOf(t, c.Client(t), core.AdapterHooks{})
	eventually(t, adapter, func(*herdr.Snapshot) bool { return true })
	folders := map[string]string{}
	for range 2 {
		folder, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		made, err := adapter.WorkspaceCreate(folder, "")
		if err != nil {
			t.Fatal(err)
		}
		folders[made.RootPane.PaneID] = folder
		startIn(t, adapter, made.RootPane.PaneID, nil)
	}

	c.Restart(t)

	eventually(t, adapter, func(s *herdr.Snapshot) bool {
		for pane, folder := range folders {
			p := s.Pane(pane)
			if p == nil || p.Agent != "" || !p.Restored || p.Dir() != folder {
				return false
			}
			if at, _ := adapter.PaneAtPrompt(pane); !at {
				return false
			}
		}
		return true
	})
	for pane := range folders {
		startIn(t, adapter, pane, []string{"--resume", "9b1c2d3e-4f50-4a6b-8c7d-0e1f2a3b4c5d"})
		screenSays(t, adapter, pane, "--resume")
	}
}
