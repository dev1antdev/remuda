package mirror

import (
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/herdr/herdrtest"
	"github.com/dev1antdev/remuda/internal/radar"
	"github.com/dev1antdev/remuda/internal/usage"
)

// countingHerdr is a real herdr client that counts the writes it carries.
type countingHerdr struct {
	*herdr.Client
	panes, spaces atomic.Int32
}

func (c *countingHerdr) PaneReportMetadata(m herdr.PaneMetadata) error {
	c.panes.Add(1)
	return c.Client.PaneReportMetadata(m)
}

func (c *countingHerdr) WorkspaceReportMetadata(m herdr.WorkspaceMetadata) error {
	c.spaces.Add(1)
	return c.Client.WorkspaceReportMetadata(m)
}

// TestAWriteNeverBringsAnotherOnARealHerdr is the feedback loop the watcher's stream could close: every metadata write
// raises herdr's pane_updated (a label's pane.agent_status_changed), the stream turns it into a snapshot, the snapshot
// reaches the writer. On herdr itself, with values herdr normalizes (spaces, a tab, a line past 80 characters) and a
// snapshot after every event and every half second past the stale check, the figures are written once.
func TestAWriteNeverBringsAnotherOnARealHerdr(t *testing.T) {
	sock := herdrtest.Server(t)
	client := &countingHerdr{Client: herdr.NewClient(sock)}
	made, err := client.WorkspaceCreate(os.TempDir(), "mirror")
	if err != nil {
		t.Fatal(err)
	}
	pane := made.RootPane.PaneID
	if err := client.Call("pane.report_agent", map[string]any{"pane_id": pane, "source": "remuda-test", "agent": "claude", "state": "blocked"}, nil); err != nil {
		t.Fatal(err)
	}
	svc := New(t.TempDir(), Inputs{
		Herdr: func(string) (Herdr, bool) { return client, true },
		Radar: func(string, *herdr.PaneInfo) radar.Display {
			return radar.Display{Alert: "waiting for your answer for 20 min"}
		},
		Usage: func(string, string) (usage.Display, bool) {
			return usage.Display{Name: "Opus 4.5 ✨", Cost: 4.2, Context: 62_000, Window: 200_000}, true
		},
		Said: func(string, string) string {
			return "  the answer\twith a tab, " + strings.Repeat("and more words ", 10)
		},
		Review: func(string, string) (int, bool) { return 3, false },
	}, "")
	w := newWriter("local", svc)
	w.warm = time.Time{}

	stream, err := client.Subscribe([]herdr.Subscription{{Type: "pane.updated"}, {Type: "workspace.metadata_updated"}, {Type: herdr.EventAgentStatus, PaneID: pane}})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	heard := make(chan struct{}, 64)
	go func() {
		for {
			if _, err := stream.Next(); err != nil {
				return
			}
			heard <- struct{}{}
		}
	}()
	snapshotNow := func() {
		snap, err := client.SessionSnapshot()
		if err != nil {
			t.Fatal(err)
		}
		w.sync(snap, time.Now())
	}
	snapshotNow()
	if client.panes.Load() != 1 || client.spaces.Load() != 1 {
		t.Fatalf("the first sync writes the pane and its workspace once: %d and %d", client.panes.Load(), client.spaces.Load())
	}
	events := 0
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for end := time.After(staleAfter + 2*time.Second); ; {
		select {
		case <-heard:
			events++
			snapshotNow()
		case <-tick.C:
			snapshotNow()
		case <-end:
			if events == 0 {
				t.Fatal("herdr raised no event for the writes, so this proves nothing")
			}
			if n, m := client.panes.Load(), client.spaces.Load(); n != 1 || m != 1 {
				t.Fatalf("after %d events and the snapshots they raised: %d pane writes and %d workspace writes, want one each", events, n, m)
			}
			snap, _ := client.SessionSnapshot()
			if got := snap.Pane(pane).StateLabels[herdr.StatusBlocked]; got != "waiting 20 min" {
				t.Errorf("the note's label: %q", got)
			}
			return
		}
	}
}
