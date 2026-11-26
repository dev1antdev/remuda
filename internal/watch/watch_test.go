package watch

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/coreapi"
	"github.com/dev1antdev/remuda/internal/events"
)

func TestApplySeedsThenReportsTransitions(t *testing.T) {
	bus := events.NewBus()
	var snapshots int
	var firsts []bool
	var changes []events.PaneStatus
	events.Subscribe(bus, func(e events.Snapshot) { snapshots++; firsts = append(firsts, e.First) })
	events.Subscribe(bus, func(e events.PaneStatus) { changes = append(changes, e) })
	w := New(nil, bus)

	pane := func(status string) coreapi.PaneInfo {
		return coreapi.PaneInfo{PaneID: "w1:p1", WorkspaceID: "w1", Agent: "claude", AgentStatus: status}
	}
	workspaces := []coreapi.WorkspaceInfo{{WorkspaceID: "w1", Label: "api"}}
	w.apply("local", &coreapi.Snapshot{Workspaces: workspaces, Panes: []coreapi.PaneInfo{pane(coreapi.StatusWorking)}})
	if snapshots != 1 || len(changes) != 0 {
		t.Fatalf("the first snapshot seeds only: %d snapshots, %v", snapshots, changes)
	}
	w.apply("local", &coreapi.Snapshot{Workspaces: workspaces, Panes: []coreapi.PaneInfo{pane(coreapi.StatusDone)}})
	if len(changes) != 1 || changes[0].From != coreapi.StatusWorking || changes[0].To != coreapi.StatusDone || changes[0].Workspace != "api" || changes[0].Kind != "claude" {
		t.Fatalf("transition: %+v", changes)
	}
	w.apply("local", &coreapi.Snapshot{Workspaces: workspaces})
	if len(changes) != 2 || changes[1].To != events.Gone || changes[1].Pane.Pane != "w1:p1" {
		t.Fatalf("a closed pane is gone: %+v", changes)
	}
	// the first snapshot, and the first after a failed poll, say so: what changed in between was not watched happening
	w.mu.Lock()
	w.stateFor("local").err = errNotWatched
	w.mu.Unlock()
	w.apply("local", &coreapi.Snapshot{Workspaces: workspaces})
	if want := []bool{true, false, false, true}; len(firsts) != len(want) || firsts[0] != want[0] || firsts[1] != want[1] || firsts[2] != want[2] || firsts[3] != want[3] {
		t.Fatalf("First flags: %v, want %v", firsts, want)
	}
	if snap, err := w.Snapshot("local"); err != nil || snap == nil || len(snap.Panes) != 0 {
		t.Fatalf("snapshot: %v %v", snap, err)
	}
	if _, err := w.Snapshot("nope"); err == nil {
		t.Fatal("an unwatched machine has no snapshot")
	}
}

// TestASnapshotIsTheWatchersOwnWhenItIsAnnounced: the snapshot's ears ask the watcher about its panes — the plan
// limits' check of an agent first seen at rest, the said tracker's first read — and were told of the snapshot before,
// or of none at all on a machine's first, so an agent stopped on its limit when remuda started went unnoticed.
func TestASnapshotIsTheWatchersOwnWhenItIsAnnounced(t *testing.T) {
	bus := events.NewBus()
	w := New(nil, bus)
	var mismatches []string
	events.Subscribe(bus, func(e events.Snapshot) {
		if snap, err := w.Snapshot(e.Machine); snap != e.Snap || err != nil {
			mismatches = append(mismatches, fmt.Sprintf("announced %p, the watcher has %p (%v)", e.Snap, snap, err))
		}
	})
	pane := coreapi.PaneInfo{PaneID: "w1:p1", WorkspaceID: "w1", Agent: "claude", AgentStatus: coreapi.StatusIdle}
	w.apply("gog", &coreapi.Snapshot{Panes: []coreapi.PaneInfo{pane}})
	w.apply("gog", &coreapi.Snapshot{Panes: []coreapi.PaneInfo{pane}})
	if len(mismatches) != 0 {
		t.Fatalf("the watcher answered another snapshot than the one announced:\n%s", strings.Join(mismatches, "\n"))
	}
}
