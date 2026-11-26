package core_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/core/coretest"
	"github.com/dev1antdev/remuda/internal/coreapi"
	"github.com/dev1antdev/remuda/internal/sessions"
)

// startIn starts the stand-in agent in a pane, on a session when one is named, and waits for its first word.
func startIn(t *testing.T, adapter *core.Adapter, pane, session string) {
	t.Helper()
	if err := sessions.StartAgentWith(adapter, pane, "claude", sessions.Start{Resume: session}); err != nil {
		t.Fatal(err)
	}
	eventually(t, adapter, func(s *coreapi.Snapshot) bool {
		p := s.Pane(pane)
		return p != nil && p.Agent == "claude" && p.AgentStatus == coreapi.StatusIdle && !p.Restored
	})
}

// A core's stop parks the shells of remuda's panes and its next start gives them new ones in their folders, as herdr
// restores its panes after a reboot: the agents that ran there are gone, the panes stand where they were, marked
// restored so the restart memory lists them, and the agents come back in them on their sessions.
func TestPanesComeBackAsShellsAfterTheCoreRestartsAndTheirAgentsResumeInThem(t *testing.T) {
	standInAgent(t)
	c := coretest.Server(t)
	adapter := adapterOf(t, c.Client(t), core.AdapterHooks{})
	eventually(t, adapter, func(*coreapi.Snapshot) bool { return true })
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
		startIn(t, adapter, made.RootPane.PaneID, "")
	}

	c.Restart(t)

	eventually(t, adapter, func(s *coreapi.Snapshot) bool {
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
		startIn(t, adapter, pane, "9b1c2d3e-4f50-4a6b-8c7d-0e1f2a3b4c5d")
		screenSays(t, adapter, pane, "--resume")
	}
}

// A pane remuda started before it kept its panes is kept once the adapter connects, so the next stop brings it back
// as well; a terminal of another program's, without remuda's labels, is left as it is.
func TestAPaneStartedBeforeTheUpgradeIsKeptToo(t *testing.T) {
	ctx := within(t, 30*time.Second)
	c := coretest.Server(t)
	client := c.Client(t)
	older, err := client.Run(ctx, core.RunParams{Rm: true, Labels: map[string]string{core.LabelSpace: "w1", core.LabelTab: "a"}})
	if err != nil {
		t.Fatal(err)
	}
	other, err := client.Run(ctx, core.RunParams{Argv: []string{"sleep", "60"}})
	if err != nil {
		t.Fatal(err)
	}
	adapterOf(t, client, core.AdapterHooks{})
	for start := time.Now(); ; time.Sleep(50 * time.Millisecond) {
		if got, err := client.Get(ctx, older.Address); err == nil && got.Keep {
			break
		}
		if time.Since(start) > 10*time.Second {
			t.Fatal("the older pane was never kept")
		}
	}
	if got, err := client.Get(ctx, other.Address); err != nil || got.Keep {
		t.Fatalf("another program's terminal: %+v %v", got, err)
	}

	c.Restart(t)
	for start := time.Now(); ; time.Sleep(50 * time.Millisecond) {
		if got, err := client.Get(ctx, older.Address); err == nil && got.State == "live" && got.Pid != 0 && got.Pid != older.Pid {
			break
		}
		if time.Since(start) > 15*time.Second {
			t.Fatal("the older pane did not come back after the core's stop")
		}
	}
}
