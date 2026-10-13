package watch

import (
	"testing"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestApplySeedsThenReportsTransitions(t *testing.T) {
	bus := events.NewBus()
	var snapshots int
	var changes []events.PaneStatus
	events.Subscribe(bus, func(events.Snapshot) { snapshots++ })
	events.Subscribe(bus, func(e events.PaneStatus) { changes = append(changes, e) })
	w := New(nil, bus)

	pane := func(status string) herdr.PaneInfo {
		return herdr.PaneInfo{PaneID: "w1:p1", WorkspaceID: "w1", Agent: "claude", AgentStatus: status}
	}
	workspaces := []herdr.WorkspaceInfo{{WorkspaceID: "w1", Label: "api"}}
	w.apply("local", &herdr.Snapshot{Workspaces: workspaces, Panes: []herdr.PaneInfo{pane(herdr.StatusWorking)}})
	if snapshots != 1 || len(changes) != 0 {
		t.Fatalf("the first snapshot seeds only: %d snapshots, %v", snapshots, changes)
	}
	w.apply("local", &herdr.Snapshot{Workspaces: workspaces, Panes: []herdr.PaneInfo{pane(herdr.StatusDone)}})
	if len(changes) != 1 || changes[0].From != herdr.StatusWorking || changes[0].To != herdr.StatusDone || changes[0].Workspace != "api" || changes[0].Kind != "claude" {
		t.Fatalf("transition: %+v", changes)
	}
	w.apply("local", &herdr.Snapshot{Workspaces: workspaces})
	if len(changes) != 2 || changes[1].To != events.Gone || changes[1].Pane.Pane != "w1:p1" {
		t.Fatalf("a closed pane is gone: %+v", changes)
	}
	if snap, err := w.Snapshot("local"); err != nil || snap == nil || len(snap.Panes) != 0 {
		t.Fatalf("snapshot: %v %v", snap, err)
	}
	if _, err := w.Snapshot("nope"); err == nil {
		t.Fatal("an unwatched machine has no snapshot")
	}
}
