package mirror

import (
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/limits"
	"github.com/dev1antdev/remuda/internal/prstatus"
	"github.com/dev1antdev/remuda/internal/radar"
	"github.com/dev1antdev/remuda/internal/transcript"
	"github.com/dev1antdev/remuda/internal/usage"
)

// Herdr is one machine's herdr as the mirror writes to it; *herdr.Client is one.
type Herdr interface {
	PaneReportMetadata(herdr.PaneMetadata) error
	WorkspaceReportMetadata(herdr.WorkspaceMetadata) error
	NotificationShow(herdr.Toast) (herdr.Shown, error)
}

// Machine is a machine of the fleet the mirror can reach: connected, its herdr answering.
type Machine struct {
	ID    string
	Label string
}

// Inputs are remuda's services as the mirror reads them, set by the app; a nil one reads as nothing known.
type Inputs struct {
	Herdr    func(machineID string) (Herdr, bool)
	Machines func() []Machine // the connected machines of the fleet: a -server host is none
	Snapshot func(machineID string) *herdr.Snapshot
	Radar    func(machineID string, pane *herdr.PaneInfo) radar.Display
	Usage    func(machineID, paneID string) (usage.Display, bool)
	Said     func(machineID, paneID string) string
	Link     func(machineID, paneID string) (transcript.Link, bool)
	PR       func(machineID, paneID string) (prstatus.Status, bool)
	Limit    func(machineID, paneID string) *limits.PaneLimit
	Review   func(machineID, dir string) (fresh int, ready bool)
	Tasks    func(machineID string) (map[string]int64, error) // by pane, the task its live run is for
}

// figures are the values every agent pane and every workspace of a snapshot should carry; a shell carries none.
func (in Inputs) figures(machineID string, snap *herdr.Snapshot, tasks map[string]int64) (map[string]Figures, map[string]Figures) {
	panes := map[string]Figures{}
	spaces := map[string]WorkspaceFacts{}
	for i := range snap.Panes {
		pane := &snap.Panes[i]
		if pane.Agent == "" {
			continue
		}
		facts := in.paneFacts(machineID, pane)
		facts.Task = tasks[pane.PaneID]
		panes[pane.PaneID] = PaneFigures(facts)
		space := spaces[pane.WorkspaceID]
		if facts.Usage != nil && !facts.Usage.Unknown {
			space.Cost += facts.Usage.Cost
		}
		if space.Link == nil && facts.Link != nil {
			space.Link, space.PR = facts.Link, facts.PR
		}
		spaces[pane.WorkspaceID] = space
	}
	workspaces := map[string]Figures{}
	for i := range snap.Workspaces {
		ws := &snap.Workspaces[i]
		facts := spaces[ws.WorkspaceID]
		if dir := snap.WorkspaceDir(ws); dir != "" && in.Review != nil {
			facts.Fresh, facts.Ready = in.Review(machineID, dir)
		}
		workspaces[ws.WorkspaceID] = WorkspaceFigures(facts)
	}
	return panes, workspaces
}

func (in Inputs) paneFacts(machineID string, pane *herdr.PaneInfo) PaneFacts {
	facts := PaneFacts{Status: pane.AgentStatus}
	if in.Radar != nil {
		facts.Note = in.Radar(machineID, pane).Alert
	}
	if in.Usage != nil {
		if meter, ok := in.Usage(machineID, pane.PaneID); ok {
			facts.Usage = &meter
		}
	}
	if in.Said != nil {
		facts.Said = in.Said(machineID, pane.PaneID)
	}
	if in.Link != nil {
		if link, ok := in.Link(machineID, pane.PaneID); ok {
			facts.Link = &link
			if in.PR != nil {
				if status, ok := in.PR(machineID, pane.PaneID); ok {
					facts.PR = &status
				}
			}
		}
	}
	if in.Limit != nil {
		facts.Limit = in.Limit(machineID, pane.PaneID)
	}
	return facts
}
