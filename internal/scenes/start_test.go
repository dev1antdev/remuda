package scenes

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/recipes"
	"github.com/dev1antdev/remuda/internal/sessions"
)

// apiOpen is a machine with a workspace open on /home/me/api.
func apiOpen() *herdr.Snapshot {
	return &herdr.Snapshot{
		Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w2", Label: "api"}},
		Panes:      []herdr.PaneInfo{{PaneID: "w2:p1", TabID: "w2:t1", WorkspaceID: "w2", CWD: "/home/me/api"}},
	}
}

func twoPanes() *Layout {
	return &Layout{Agent: 1, Root: Node{Type: "split", Direction: "right", Ratio: 0.5, First: &Node{Type: "pane", Dir: "web"}, Second: &Node{Type: "pane"}}}
}

func TestAStepGetsATabInTheFoldersWorkspaceElseAWorkspace(t *testing.T) {
	client := &fakeHerdr{}
	place, err := placeStep(client, apiOpen(), Step{Dir: "/home/me/api", Tab: "planner"})
	if err != nil || place.MadeWorkspace || place.Workspace != "w2" || place.Pane != "w2:p7" {
		t.Fatalf("a tab of the open workspace: %+v %v", place, err)
	}
	place, err = placeStep(client, apiOpen(), Step{Dir: "/home/me/web", Tab: "dev"})
	if err != nil || !place.MadeWorkspace || place.Pane != "w8:p1" {
		t.Fatalf("a workspace on a folder without one: %+v %v", place, err)
	}
	if got := strings.Join(client.said(), "; "); got != `tab w2 "planner"; workspace /home/me/web; rename w8:t1 "dev"` {
		t.Fatalf("asked of herdr: %s", got)
	}
}

func TestAStepsLayoutIsANewTabOrTheNewWorkspacesFirst(t *testing.T) {
	client := &fakeHerdr{}
	place, err := placeStep(client, apiOpen(), Step{Dir: "/home/me/api", Tab: "dev", Layout: twoPanes()})
	if err != nil || place.MadeWorkspace || place.Workspace != "w2" || place.Pane != "w1:p71" {
		t.Fatalf("a laid-out tab of the open workspace, the agent in its second pane: %+v %v", place, err)
	}
	place, err = placeStep(client, nil, Step{Dir: "/home/me/web", Layout: twoPanes()})
	if err != nil || !place.MadeWorkspace || place.Workspace != "w8" || place.Tab != "w1:t9" || place.Pane != "w1:p91" {
		t.Fatalf("a new workspace laid out in place of its first tab: %+v %v", place, err)
	}
	if got := strings.Join(client.said(), "; "); got != `layout w2  "dev" 2; workspace /home/me/web; layout  w8:t1 "" 2` {
		t.Fatalf("asked of herdr: %s", got)
	}
}

func TestAStepsAgentComesFromItselfOrItsRecipe(t *testing.T) {
	cookbook := func(id string) (recipes.Recipe, bool) {
		if id == "r1" {
			return recipes.Recipe{Agent: "codex", Model: "gpt-5", Args: "--full-auto", Prompt: "Plan the day"}, true
		}
		return recipes.Recipe{}, false
	}
	what, err := launchOf(Step{Recipe: "r1"}, cookbook)
	if err != nil || what.kind != "codex" || what.start.Model != "gpt-5" || what.start.Args[0] != "--full-auto" || what.prompt != "Plan the day" {
		t.Fatalf("the recipe's: %+v %v", what, err)
	}
	if _, err := launchOf(Step{Recipe: "r2"}, cookbook); err == nil {
		t.Fatal("a recipe that is gone started")
	}
	if what, _ := launchOf(Step{Agent: "shell"}, cookbook); what.kind != "shell" {
		t.Fatalf("a shell: %+v", what)
	}
	if what, _ := launchOf(Step{Agent: "claude", Model: "opus"}, cookbook); what.kind != "claude" || what.start.Model != "opus" {
		t.Fatalf("its own: %+v", what)
	}
}

func TestAStartSceneRunsItsStepsOneAfterAnotherAndSaysHowEachWent(t *testing.T) {
	retry := sessions.FirstPromptRetry
	sessions.FirstPromptRetry = time.Millisecond
	t.Cleanup(func() { sessions.FirstPromptRetry = retry })
	local := &fakeMachine{id: "local", label: "This Mac", client: &fakeHerdr{}, snap: apiOpen()}
	gog := &fakeMachine{id: "gog", label: "GOG", away: errors.New("not connected")}
	svc, _, _ := fleetOf(t, local, gog)
	saved, err := svc.Store.Replace([]Scene{{Name: "Morning", Steps: []Step{
		{Machine: "local", Dir: "/home/me/api", Agent: "claude", Model: "opus", Prompt: "Read the overnight PRs"},
		{Machine: "gog", Dir: "/home/me/web", Agent: "codex"},
		{Machine: "local", Dir: "/home/me/docs", Agent: "shell", Tab: "notes"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	started, err := svc.Start(saved[0].ID, nil, nil, "iPhone")
	if err != nil || len(started.Items) != 3 {
		t.Fatalf("started: %+v %v", started, err)
	}
	run := waitDone(t, svc, started.ID)
	first, second, third := run.Items[0], run.Items[1], run.Items[2]
	if first.State != StateStarted || first.Pane != "w2:p7" || first.MadeWorkspace {
		t.Fatalf("the agent in a tab of api: %+v", first)
	}
	if second.State != StateAway || !strings.Contains(second.Note, "not connected") {
		t.Fatalf("a machine away is said, not tried: %+v", second)
	}
	if third.State != StateStarted || !third.MadeWorkspace {
		t.Fatalf("a shell in a new workspace: %+v", third)
	}
	calls := strings.Join(local.client.said(), "; ")
	if !strings.Contains(calls, "start w2:p7 claude --model opus") || strings.Contains(calls, "start w8") {
		t.Fatalf("the agent started with its model, no agent in the shell: %s", calls)
	}
	for i := 0; i < 100 && len(local.client.prompts()) == 0; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	if got := local.client.prompts(); len(got) != 1 || got[0] != "w2:p7: Read the overnight PRs" {
		t.Fatalf("the first prompt handed over: %v", local.client.prompts())
	}
	if local.refreshed == 0 {
		t.Fatal("the watcher reads the machine again")
	}
}

func TestOnlyThePickedStepsRunAndOneRunAtATime(t *testing.T) {
	local := &fakeMachine{id: "local", label: "This Mac", client: &fakeHerdr{}}
	svc, _, _ := fleetOf(t, local)
	saved, _ := svc.Store.Replace([]Scene{{Name: "Morning", Steps: []Step{
		{Machine: "local", Dir: "/a", Agent: "shell"}, {Machine: "local", Dir: "/b", Agent: "shell"}, {Machine: "local", Dir: "/c", Agent: "shell"},
	}}})
	if _, err := svc.Start(saved[0].ID, []int{5}, nil, ""); err == nil {
		t.Fatal("a step the scene lacks")
	}
	svc.busy = map[string]bool{saved[0].ID: true}
	if _, err := svc.Start(saved[0].ID, nil, nil, ""); !errors.Is(err, ErrBusy) {
		t.Fatalf("a second run while one goes: %v", err)
	}
	svc.busy = nil
	started, err := svc.Start(saved[0].ID, []int{2, 0, 2}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	run := waitDone(t, svc, started.ID)
	if len(run.Items) != 2 || run.Items[0].Index != 0 || run.Items[1].Index != 2 || run.Items[1].Step.Dir != "/c" {
		t.Fatalf("the picked ones, once each, in the scene's order: %+v", run.Items)
	}
}

func TestAStepHerdrRefusesIsSaidWithItsTab(t *testing.T) {
	local := &fakeMachine{id: "local", label: "This Mac", client: &fakeHerdr{refuse: "start"}, snap: apiOpen()}
	svc, _, _ := fleetOf(t, local)
	var item Item
	svc.runStep("Morning", Step{Machine: "local", Dir: "/home/me/api", Agent: "claude"}, &item)
	if item.State != StateRefused || item.Pane != "w2:p7" || !strings.Contains(item.Note, "the tab is there") {
		t.Fatalf("refused, its pane to open: %+v", item)
	}
	svc.runStep("Morning", Step{Machine: "nowhere", Dir: "/x", Agent: "shell"}, &item)
	if item.State != StateAway {
		t.Fatalf("a machine the fleet lacks: %+v", item)
	}
	// herdr opens a folder that is not there in the home folder instead, so the step never asks it to
	local.missing = []string{"/home/me/gone"}
	before := len(local.client.said())
	item = Item{}
	svc.runStep("Morning", Step{Machine: "local", Dir: "/home/me/gone", Agent: "shell"}, &item)
	if item.State != StateRefused || !strings.Contains(item.Note, "is not there") || len(local.client.said()) != before {
		t.Fatalf("a folder that is gone: %+v", item)
	}
}
