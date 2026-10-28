package updates

import (
	"errors"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/jobs"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/restart"
	"github.com/dev1antdev/remuda/internal/watch"
)

// TestStartRefusesWhatItCannotRun covers the refusals, each before any job starts: nothing here reaches a shell.
func TestStartRefusesWhatItCannotRun(t *testing.T) {
	bus := events.NewBus()
	fleet := machines.NewManager(t.TempDir(), "/nonexistent/herdr.sock", "herdr", nil)
	service := &Service{Health: health.New("", fleet, "dev", false), Jobs: jobs.New(bus), Restart: restart.New(fleet), Watch: watch.New(fleet, bus)}
	local, _ := fleet.Get("")

	for _, c := range []struct{ what, command, want string }{
		{"claude", "", "the command shown must come back"},
		{"restart-skynet", "x", "nothing to restart called"},
		{"restart-claude", "x", "no Claude Code chat with a session"},
		{"remuda-release", "x", "no newer release of remuda"},
		{"remuda", "x", "cannot be deployed there"},
		{"claude", "x", "Claude Code is not installed there"},
		{"skynet", "x", "nothing to update called"},
	} {
		_, err := service.Start(local, c.what, c.command)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("Start(%q, %q) = %v, want %q", c.what, c.command, err, c.want)
		}
		var stale *StaleError
		if errors.As(err, &stale) {
			t.Errorf("Start(%q, %q): a refusal is not a stale command", c.what, c.command)
		}
	}
	if _, err := service.RestartPane(local, "w1:p1"); err == nil || !strings.Contains(err.Error(), "no pane w1:p1") {
		t.Errorf("RestartPane with no snapshot = %v, want no pane", err)
	}
	if offered := service.Available(local); len(offered) != 0 {
		t.Fatalf("nothing is known, nothing offered: %+v", offered)
	}
	if list, pending := service.Recent(local); len(list) != 0 || pending != 0 {
		t.Fatalf("no jobs yet: %v %d", list, pending)
	}
}

func TestRestartPlanNamesThePanesItMeans(t *testing.T) {
	session := &herdr.AgentSession{Kind: "id", Value: "abc"}
	snap := &herdr.Snapshot{Panes: []herdr.PaneInfo{
		{PaneID: "w1:p1", Agent: "claude", AgentStatus: herdr.StatusIdle, AgentSession: session},
		{PaneID: "w1:p2", Agent: "claude", AgentStatus: herdr.StatusWorking, AgentSession: session},
		{PaneID: "w2:p1", Agent: "codex", AgentStatus: herdr.StatusDone, AgentSession: session},
	}}
	plan, n := restartPlan("claude", snap)
	if n != 2 || plan.What != "restart-claude" || !strings.Contains(plan.Command, "w1:p1, w1:p2") || !strings.Contains(plan.Note, "1 of them busy") {
		t.Fatalf("plan: %d %+v", n, plan)
	}
	if _, n := restartPlan("gemini", snap); n != 0 {
		t.Fatal("no gemini chats")
	}
	if plans := restartPlans(snap); len(plans) != 2 || plans[0].What != "restart-claude" || plans[1].What != "restart-codex" {
		t.Fatalf("plans: %+v", plans)
	}
	if restartNote(snap, restart.Kinds("agents")) == "" || restartNote(snap, nil) != "" {
		t.Fatal("restart note")
	}
}
