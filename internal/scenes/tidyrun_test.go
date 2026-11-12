package scenes

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/radar"
	"github.com/dev1antdev/remuda/internal/worktrees"
)

var errAway = errors.New("not connected")

func eveningScene(t *testing.T, svc *Service, tidy Tidy) string {
	t.Helper()
	saved, err := svc.Store.Replace([]Scene{{Name: "Evening", Kind: KindTidy, Tidy: &tidy}})
	if err != nil {
		t.Fatal(err)
	}
	return saved[0].ID
}

func TestATidyDoesWhatWasPickedAfterAFreshLook(t *testing.T) {
	local := &fakeMachine{id: "local", label: "This Mac", client: &fakeHerdr{atPrompt: []string{"w1:p2"}}, snap: dusk(),
		leftovers: []worktrees.Leftover{{Path: "/home/me/.herdr/worktrees/api/old"}}}
	svc, _, _ := fleetOf(t, local)
	svc.Readings = duskReadings()
	svc.Now = func() time.Time { return evening }
	var sent []string
	svc.Sent = func(by, machine, pane, text string) { sent = append(sent, by+" "+pane+" "+text) }
	id := eveningScene(t, svc, Tidy{Shells: true, Compact: true, CompactAt: 60, Worktrees: true})

	if _, err := svc.Start(id, nil, nil, "iPhone"); err == nil {
		t.Fatal("a tidy with nothing picked ran")
	}
	picked := []string{"compact|local|w2:p4", "close-shell|local|w1:p2", "worktree|local|/home/me/.herdr/worktrees/api/old", "close-agent|local|w2:p1"}
	started, err := svc.Start(id, nil, picked, "iPhone")
	if err != nil {
		t.Fatal(err)
	}
	run := waitDone(t, svc, started.ID)
	var states []string
	for _, item := range run.Items {
		states = append(states, item.State)
	}
	if strings.Join(states, " ") != "done done done skipped" || run.Items[3].Note != "it works now" {
		t.Fatalf("the picked ones done, the one that moved on skipped: %v %+v", states, run.Items[3])
	}
	if run.Items[0].Action.Kind != ActCompact || run.Items[0].Action.Context != 85 {
		t.Fatalf("an item carries its action as planned: %+v", run.Items[0].Action)
	}
	calls := strings.Join(local.client.said(), "; ")
	if !strings.Contains(calls, `prompt w2:p4 "/compact"`) || !strings.Contains(calls, "close w1:p2") || strings.Contains(calls, "w1:p1") || strings.Contains(calls, "close w2:p1") {
		t.Fatalf("only what was picked: %s", calls)
	}
	if len(sent) != 1 || sent[0] != "iPhone w2:p4 /compact" {
		t.Fatalf("the /compact is a row of the activity: %v", sent)
	}
	if len(local.tidied) != 1 || local.refreshed == 0 {
		t.Fatalf("the worktree tidied, the machine read again: %v %d", local.tidied, local.refreshed)
	}
}

func TestATidyNamedAMachineAwayAndAnAgentBusyWithAQuestion(t *testing.T) {
	local := &fakeMachine{id: "local", label: "This Mac", client: &fakeHerdr{refuse: "prompt"}, snap: dusk()}
	svc, _, _ := fleetOf(t, local)
	svc.Readings = duskReadings()
	svc.Now = func() time.Time { return evening }
	id := eveningScene(t, svc, Tidy{Compact: true, CompactAt: 60})
	run, err := svc.RunNow(id, "the schedule “Evening”")
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Items) != 2 || run.Items[0].State != StateRefused || !strings.Contains(run.Items[0].Note, "says no") {
		t.Fatalf("a schedule's run takes the whole plan; herdr's refusal said: %+v", run.Items)
	}
	local.away = errAway
	if plan := svc.Plan(Tidy{Shells: true}, evening); len(plan.Actions) != 0 || len(plan.Unread) != 1 || plan.Unread[0].Why != "not connected" {
		t.Fatalf("a machine away is looked at no further: %+v", plan)
	}
}

func TestAScheduledTidyWaitsForNobodysAgentAtWork(t *testing.T) {
	local := &fakeMachine{id: "local", label: "This Mac", client: &fakeHerdr{}, snap: dusk()}
	gog := &fakeMachine{id: "gog", label: "GOG", client: &fakeHerdr{}, snap: &herdr.Snapshot{}}
	svc, _, _ := fleetOf(t, local, gog)
	svc.Readings = duskReadings()
	if got := svc.AtWork(Tidy{Shells: true}); got != "This Mac" {
		t.Fatalf("the Codex at work on this Mac: %q", got)
	}
	if got := svc.AtWork(Tidy{Shells: true, Machines: []string{"gog"}}); got != "" {
		t.Fatalf("nobody at work on GOG: %q", got)
	}
	svc.Readings.State = func(string, *herdr.PaneInfo) radar.Display { return radar.Display{State: herdr.StatusIdle} }
	if got := svc.AtWork(Tidy{Shells: true}); got != "" {
		t.Fatalf("everyone at rest: %q", got)
	}
}

func TestARunInALine(t *testing.T) {
	line, err := Summary(Run{Items: []Item{{State: StateStarted}, {State: StateStarted}, {State: StateAway}}})
	if err != nil || line != "2 started, 1 away" {
		t.Fatalf("%q %v", line, err)
	}
	if _, err := Summary(Run{Items: []Item{{State: StateDone}, {State: StateRefused, Note: "herdr says no"}}}); err == nil || err.Error() != "1 done, 1 refused: herdr says no" {
		t.Fatalf("a refusal is an error: %v", err)
	}
	if line, _ := Summary(Run{}); line != "nothing to do" {
		t.Fatalf("%q", line)
	}
}
