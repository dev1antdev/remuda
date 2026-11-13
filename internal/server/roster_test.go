package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/radar"
	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/subagents"
	"github.com/dev1antdev/remuda/internal/transcript"
	"github.com/dev1antdev/remuda/internal/usage"
)

func TestATabNamedByDigitsIsHerdrs(t *testing.T) {
	for label, named := range map[string]bool{"": false, "1": false, "12": false, " 2 ": false, "claude": true, "tab 2": true, "2a": true} {
		if got := tabNamed(label); got != named {
			t.Errorf("tabNamed(%q) = %v, want %v", label, got, named)
		}
	}
}

// An agent pane carries how many of its conversation's subagents work and the first of them by name; a pane with none at work carries no field at all.
func TestRosterCarriesThePanesSubagents(t *testing.T) {
	lists := map[string][]transcript.Subagent{
		"w1:p1": {{ID: "agent-a", Description: "explore · cart", Status: transcript.SubagentWorking}, {ID: "agent-b", Status: transcript.SubagentWorking}, {ID: "agent-c", Status: transcript.SubagentDone}},
		"w1:p2": {{ID: "agent-d", Status: transcript.SubagentDone}},
	}
	counts := subagents.New(func(_, paneID string, _ time.Time) (string, []transcript.Subagent, error) {
		return "s-" + paneID, lists[paneID], nil
	}, func(string) bool { return true })
	snap := &herdr.Snapshot{
		Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1", Label: "api"}},
		Tabs:       []herdr.TabInfo{{TabID: "w1:t1", WorkspaceID: "w1", Number: 1}},
		Panes: []herdr.PaneInfo{
			{PaneID: "w1:p1", TabID: "w1:t1", WorkspaceID: "w1", Agent: "claude", AgentStatus: herdr.StatusWorking},
			{PaneID: "w1:p2", TabID: "w1:t1", WorkspaceID: "w1", Agent: "claude", AgentStatus: herdr.StatusIdle},
		},
	}
	counts.Touch()
	counts.Observe("local", snap)
	deadline := time.Now().Add(2 * time.Second)
	for counts.Count("local", "w1:p1") == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	s := &Server{Radar: radar.New(t.TempDir(), nil), Usage: usage.NewTracker(), Review: review.New(t.TempDir()), Subagents: counts}
	sessions := s.sessionsFromSnapshot("local", snap)
	panes := map[string]string{}
	for _, pane := range sessions[0].Tabs[0].Panes {
		body, err := json.Marshal(pane)
		if err != nil {
			t.Fatal(err)
		}
		panes[pane.ID] = string(body)
	}
	if !strings.Contains(panes["w1:p1"], `"subagents":{"working":2,"session":"s-w1:p1","list":[{"id":"agent-a","description":"explore · cart"},{"id":"agent-b","description":""}]}`) {
		t.Fatalf("the pane with subagents: %s", panes["w1:p1"])
	}
	if strings.Contains(panes["w1:p2"], `"subagents"`) {
		t.Fatalf("a pane with none at work carries the field: %s", panes["w1:p2"])
	}
}
