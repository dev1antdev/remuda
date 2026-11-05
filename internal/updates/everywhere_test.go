package updates

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/jobs"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/restart"
	"github.com/dev1antdev/remuda/internal/watch"
)

func everywhereService(t *testing.T) *Service {
	bus := events.NewBus()
	fleet := machines.NewManager(t.TempDir(), "/nonexistent/herdr.sock", "herdr", nil)
	return &Service{Health: health.New("", fleet, "dev", false), Jobs: jobs.New(bus), Restart: restart.New(fleet), Watch: watch.New(fleet, bus), Machines: fleet}
}

func TestOnlyAnUpdateWithSomethingNewerIsAStepEverywhere(t *testing.T) {
	report := &health.Report{Herdr: "0.9.4", Claude: "2.1.300", Codex: "0.80.0", CodeServer: "4.139.0", OutdatedHooks: []health.Hook{{Agent: "codex"}}}
	latest := health.Latest{Herdr: "0.9.5", Claude: "2.1.300", Code: "4.140.0"}
	for what, want := range map[string]bool{
		"herdr": true, "claude": false, "codex": false, "gemini": false, "hooks": true, "code": true,
		"remuda-release": true, "mcp": false, "statusline": false, "plugins": false, "restart-claude": false,
	} {
		if got := pendingUpdate(what, report, latest); got != want {
			t.Errorf("pendingUpdate(%q) = %v, want %v", what, got, want)
		}
	}
	if pendingUpdate("hooks", &health.Report{}, latest) || pendingUpdate("code", &health.Report{}, latest) {
		t.Error("nothing installed counts as an update")
	}
}

func TestEverywhereRefusesAStepThatMovedAndRunsNoneOfThem(t *testing.T) {
	s := everywhereService(t)
	if plan := s.EverywherePlan(); len(plan) != 0 {
		t.Fatalf("nothing is known, nothing offered: %+v", plan)
	}
	if _, err := s.StartEverywhere(nil); err == nil {
		t.Fatal("a run of no step started")
	}
	_, err := s.StartEverywhere([]Step{{Machine: "local", Label: "This Mac", What: "herdr", Title: "Update herdr", Command: "brew upgrade herdr"}})
	var stale *StaleError
	if !errors.As(err, &stale) {
		t.Fatalf("a step the plan no longer holds = %v, want a StaleError", err)
	}
}

func TestARunGoesOnPastAStepThatFailsAndEndsDone(t *testing.T) {
	s := everywhereService(t)
	stepPoll = 10 * time.Millisecond
	batch := &Batch{ID: "b1", Started: time.Now(), Steps: []StepRun{
		{Step: Step{Machine: "gone", Label: "Gone", What: "herdr"}, State: "waiting"},
		{Step: Step{Machine: "local", Label: "This Mac", What: "skynet", Command: "x"}, State: "waiting"},
	}}
	s.runs.list = append(s.runs.list, batch)
	s.runBatch(batch)
	got, ok := s.Everywhere("b1")
	if !ok || !got.Done || got.Ended.IsZero() {
		t.Fatalf("the run did not end: %+v", got)
	}
	if got.Steps[0].State != "skipped" || got.Steps[1].State != "skipped" || got.Steps[1].Error != "nothing left to do: up to date now" {
		t.Fatalf("steps %+v", got.Steps)
	}
	s.runs.list = append(s.runs.list, &Batch{ID: "b2"})
	if _, err := s.StartEverywhere([]Step{{Machine: "local", What: "herdr"}}); !errors.Is(err, ErrBatchRunning) {
		t.Fatalf("a second run while one goes = %v, want ErrBatchRunning", err)
	}
}

func TestARunGoesInThePlansOrderTheHostLast(t *testing.T) {
	plan := []Step{
		{Machine: "gog", What: "herdr", Command: "herdr update --handoff"},
		{Machine: "gog", What: "remuda-release", Command: "curl gog"},
		{Machine: "local", What: "remuda-release", Command: "curl here"},
	}
	steps, err := inPlanOrder(plan, []Step{plan[2], plan[0], plan[1]})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, step := range steps {
		order = append(order, step.Machine+"/"+step.What)
	}
	if got := strings.Join(order, " "); got != "gog/herdr gog/remuda-release local/remuda-release" {
		t.Fatalf("the run's order %s: the page's own host must restart last", got)
	}
	if steps, _ := inPlanOrder(plan, []Step{plan[2]}); len(steps) != 1 || steps[0].Machine != "local" {
		t.Fatalf("only the steps asked for run: %+v", steps)
	}
	var stale *StaleError
	if _, err := inPlanOrder(plan, []Step{{Machine: "gog", What: "herdr", Command: "brew upgrade herdr"}}); !errors.As(err, &stale) {
		t.Fatalf("a command other than the one shown = %v, want a StaleError", err)
	}
}

func TestAServersHostIsOfferedItsReleaseAlone(t *testing.T) {
	offered := []Update{{What: "herdr"}, {What: "hooks"}, {What: "mcp"}, {What: "remuda-release", Command: "curl"}, {What: "restart-herdr"}}
	if kept := serverHostOnly(offered); len(kept) != 1 || kept[0].What != "remuda-release" {
		t.Fatalf("a server's host is offered %+v", kept)
	}
	if kept := serverHostOnly([]Update{{What: "herdr"}}); kept == nil || len(kept) != 0 {
		t.Fatalf("nothing to offer is an empty list, not %+v", kept)
	}
	s := everywhereService(t)
	s.Machines.SetServer(true)
	host, err := s.Machines.Get(machines.LocalID)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range s.Available(host) {
		if u.What != "remuda-release" {
			t.Errorf("a server's host is offered %s", u.What)
		}
	}
}
