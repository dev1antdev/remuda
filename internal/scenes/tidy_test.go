package scenes

import (
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/radar"
	"github.com/dev1antdev/remuda/internal/worktrees"
)

var evening = time.Date(2026, 11, 12, 21, 0, 0, 0, time.UTC)

func hoursAgo(h int) int64 { return evening.Add(-time.Duration(h) * time.Hour).UnixMilli() }

const session = "0b6f3c2e-6a4d-4f6e-9a51-2f0e8c1d7a90"

// dusk is a machine at the end of the day: an api workspace with an idle Claude Code (with a session) and a shell
// beside it, a web workspace with a Codex at work, a Gemini asking, a Claude Code whose finish nobody saw, a Claude
// Code with no session, a shell running a dev server, and the floating terminal's shell.
func dusk() *herdr.Snapshot {
	withSession := &herdr.AgentSession{Agent: "claude", Kind: "id", Value: session}
	return &herdr.Snapshot{
		Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1", Label: "api"}, {WorkspaceID: "w2", Label: "web"}, {WorkspaceID: "w99", Label: "scratch"}},
		Tabs: []herdr.TabInfo{
			{TabID: "w1:t1", WorkspaceID: "w1", Label: "1"}, {TabID: "w1:t2", WorkspaceID: "w1", Label: "tests"},
			{TabID: "w2:t1", WorkspaceID: "w2", Label: "1"}, {TabID: "w99:t1", WorkspaceID: "w99"},
		},
		Panes: []herdr.PaneInfo{
			{PaneID: "w1:p1", TabID: "w1:t1", WorkspaceID: "w1", Agent: "claude", AgentStatus: herdr.StatusIdle, AgentSession: withSession},
			{PaneID: "w1:p2", TabID: "w1:t2", WorkspaceID: "w1"},
			{PaneID: "w2:p1", TabID: "w2:t1", WorkspaceID: "w2", Agent: "codex", AgentStatus: herdr.StatusWorking, AgentSession: withSession},
			{PaneID: "w2:p2", TabID: "w2:t1", WorkspaceID: "w2", Agent: "gemini", AgentStatus: herdr.StatusBlocked, AgentSession: withSession},
			{PaneID: "w2:p3", TabID: "w2:t1", WorkspaceID: "w2", Agent: "claude", AgentStatus: herdr.StatusDone, AgentSession: withSession},
			{PaneID: "w2:p4", TabID: "w2:t1", WorkspaceID: "w2", Agent: "claude", AgentStatus: herdr.StatusIdle},
			{PaneID: "w2:p5", TabID: "w2:t1", WorkspaceID: "w2"},
			{PaneID: "w99:p1", TabID: "w99:t1", WorkspaceID: "w99"},
		},
	}
}

func duskReadings() Readings {
	states := map[string]radar.Display{
		"w1:p1": {State: herdr.StatusIdle, LastActive: hoursAgo(10)},
		"w2:p1": {State: herdr.StatusWorking, LastActive: hoursAgo(0)},
		"w2:p2": {State: herdr.StatusBlocked, LastActive: hoursAgo(12)},
		"w2:p3": {State: herdr.StatusDone, LastActive: hoursAgo(12)},
		"w2:p4": {State: herdr.StatusIdle, LastActive: hoursAgo(12)},
	}
	return Readings{
		State:   func(_ string, pane *herdr.PaneInfo) radar.Display { return states[pane.PaneID] },
		Context: func(_, pane string) (int, bool) { return 85, true },
		Scratch: func(_, workspace string) bool { return workspace == "w99" },
	}
}

func atPrompt(panes ...string) func(string) bool {
	return func(pane string) bool {
		for _, p := range panes {
			if p == pane {
				return true
			}
		}
		return false
	}
}

func kinds(actions []Action) string {
	var out []string
	for _, a := range actions {
		out = append(out, a.Kind+" "+a.Pane)
	}
	return strings.Join(out, ", ")
}

func TestATidyNeverTouchesAnAgentThatWorksAsksOrHasANewFinish(t *testing.T) {
	everything := Tidy{Shells: true, Idle: true, IdleHours: 8, Compact: true, CompactAt: 60}
	got := kinds(planPanes(everything, "local", dusk(), atPrompt("w1:p2", "w99:p1"), duskReadings(), evening))
	// the idle Claude Code with a session closed, the one without compacted, the shell at its prompt closed; the dev
	// server's shell, the working, asking and unseen ones and the scratch shell left alone
	if got != "close-agent w1:p1, close-shell w1:p2, compact w2:p4" {
		t.Fatalf("the plan: %s", got)
	}
}

func TestEachChoiceOfATidyIsItsOwn(t *testing.T) {
	read := duskReadings()
	if got := kinds(planPanes(Tidy{Shells: true}, "local", dusk(), atPrompt("w1:p2"), read, evening)); got != "close-shell w1:p2" {
		t.Fatalf("shells alone: %s", got)
	}
	if got := kinds(planPanes(Tidy{Idle: true, IdleHours: 11}, "local", dusk(), atPrompt(), read, evening)); got != "" {
		t.Fatalf("nobody idle for 11 hours with a session: %s", got)
	}
	if got := kinds(planPanes(Tidy{Compact: true, CompactAt: 60}, "local", dusk(), atPrompt(), read, evening)); got != "compact w1:p1, compact w2:p4" {
		t.Fatalf("compacts alone, Claude Code only: %s", got)
	}
	if got := kinds(planPanes(Tidy{Compact: true, CompactAt: 90}, "local", dusk(), atPrompt(), read, evening)); got != "" {
		t.Fatalf("nobody past 90 %%: %s", got)
	}
}

func TestAnActionSaysWhatGoesWithThePane(t *testing.T) {
	actions := planPanes(Tidy{Shells: true, Idle: true, IdleHours: 8}, "local", dusk(), atPrompt("w1:p2"), duskReadings(), evening)
	if actions[0].Closes != "tab" || actions[0].Workspace != "api" || actions[0].Since != hoursAgo(10) || actions[0].ID != "close-agent|local|w1:p1" {
		t.Fatalf("the agent's tab goes with it: %+v", actions[0])
	}
	alone := &herdr.Snapshot{Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w3"}}, Tabs: []herdr.TabInfo{{TabID: "w3:t1", WorkspaceID: "w3"}}, Panes: []herdr.PaneInfo{{PaneID: "w3:p1", TabID: "w3:t1", WorkspaceID: "w3"}}}
	if got := planPanes(Tidy{Shells: true}, "local", alone, atPrompt("w3:p1"), duskReadings(), evening); got[0].Closes != "workspace" {
		t.Fatalf("a workspace's last pane takes it along: %+v", got)
	}
	if closes(dusk(), &dusk().Panes[2]) != "" {
		t.Fatal("a pane beside others takes nothing along")
	}
}

func TestOnlyCleanLeftoverWorktreesAreRemoved(t *testing.T) {
	got := planWorktrees("gog", []worktrees.Leftover{
		{Path: "/home/me/.herdr/worktrees/api/old", Branch: "old", KB: 2048},
		{Path: "/home/me/.herdr/worktrees/api/wip", Dirty: "M main.go"},
		{Path: "/home/me/.herdr/worktrees/api/odd", Broken: "not a worktree git knows"},
	})
	if len(got) != 1 || got[0].ID != "worktree|gog|/home/me/.herdr/worktrees/api/old" || got[0].KB != 2048 || got[0].Branch != "old" {
		t.Fatalf("the clean one alone: %+v", got)
	}
	if back := parseID(got[0].ID); back.Kind != ActWorktree || back.Machine != "gog" || back.Path != "/home/me/.herdr/worktrees/api/old" {
		t.Fatalf("the id read back: %+v", back)
	}
}

func TestThePlanCoversItsMachinesInTheOrderItActs(t *testing.T) {
	local := &fakeMachine{id: "local", label: "This Mac", client: &fakeHerdr{atPrompt: []string{"w1:p2"}}, snap: dusk(),
		leftovers: []worktrees.Leftover{{Path: "/home/me/.herdr/worktrees/api/old"}}}
	gog := &fakeMachine{id: "gog", label: "GOG", away: errAway}
	roofr := &fakeMachine{id: "roofr", label: "Roofr", client: &fakeHerdr{}}
	svc, _, _ := fleetOf(t, local, gog, roofr)
	svc.Readings = duskReadings()
	plan := svc.Plan(Tidy{Worktrees: true, Shells: true, Compact: true, CompactAt: 60, Machines: []string{"local", "gog"}}, evening)
	if got := kinds(plan.Actions); got != "compact w1:p1, compact w2:p4, close-shell w1:p2, worktree " {
		t.Fatalf("the compacts first, then the closes, then the worktrees: %s", got)
	}
	if len(plan.Unread) != 1 || plan.Unread[0].Label != "GOG" {
		t.Fatalf("the machine away, Roofr not covered: %+v", plan.Unread)
	}
}

func TestAPickedActionNoLongerSoIsSkippedWithWhy(t *testing.T) {
	snap := dusk()
	read := duskReadings()
	for pane, want := range map[string]string{"w2:p1": "it works now", "w2:p2": "it asks you something now", "w2:p3": "it has a finish you have not looked at", "w9:p9": "gone already"} {
		if got := staleWhy(Action{Kind: ActCloseAgent, Machine: "local", Pane: pane}, snap, read); got != want {
			t.Errorf("%s: %q, want %q", pane, got, want)
		}
	}
	if got := staleWhy(Action{Kind: ActCloseShell, Machine: "local", Pane: "w2:p5"}, snap, read); got != "something runs in it now" {
		t.Errorf("a shell busy with a command: %q", got)
	}
	if got := staleWhy(Action{Kind: ActCloseShell, Machine: "local", Pane: "w1:p1"}, snap, read); got != "an agent runs in it now" {
		t.Errorf("a shell an agent started in: %q", got)
	}
}
