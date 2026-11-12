package tasks

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/push"
)

// queued is the Inbox with a Queued column after New and its agents set to work on studio's /w/app, the queue on
func queued(t *testing.T, st *Store, max int) Project {
	t.Helper()
	ps, _ := st.Projects()
	inbox := ps[0]
	var cols []Status
	for _, s := range inbox.Statuses {
		cols = append(cols, s)
		if s.OnNew {
			cols = append(cols, Status{Title: "Queued", Kind: KindOpen, OnQueue: true})
		}
	}
	if _, err := st.SetStatuses(inbox.ID, cols, nil); err != nil {
		t.Fatal(err)
	}
	p, err := st.SetAutomation(inbox.ID, Automation{Machine: "studio", Dir: "/w/app", Agent: "claude", Queue: true, Max: max})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func column(p Project, flag func(Status) bool) Status {
	return *flagged(p.Statuses, flag)
}

// queueHere is a queue over fakes: studio connected with a workspace on /w/app, nothing spent; every start is recorded
// and becomes a run on its own pane, or fails with startErr
type queueHere struct {
	q         *Queue
	connected bool
	workspace string
	spent     string
	crossed   string
	startErr  error
	states    map[string]string
	starts    []StartRequest
	on        []string // the machine each start went to
	started   []int64
	pushed    []push.Payload
}

func newQueue(st *Store) *queueHere {
	h := &queueHere{connected: true, workspace: "w1", states: map[string]string{}}
	h.q = &Queue{
		Store:     st,
		Machine:   func(id string) (string, bool) { return "Studio", h.connected },
		Workspace: func(machine, dir string) string { return h.workspace },
		PaneState: func(machine, pane string) (string, bool) {
			s, ok := h.states[pane]
			return s, ok
		},
		Spent:   func(machine, agent string) string { return h.spent },
		Crossed: func(machine, dir string) string { return h.crossed },
		Start: func(machine string, task Task, req StartRequest) error {
			h.starts, h.on = append(h.starts, req), append(h.on, machine)
			if h.startErr != nil {
				return h.startErr
			}
			pane := "w" + task.Identifier[1:] + ":p1"
			h.states[pane] = "working"
			h.started = append(h.started, task.ID)
			_, err := st.StartRun(Run{TaskID: task.ID, Machine: machine, Workspace: "wt", Pane: pane, Agent: req.Agent, ByQueue: req.Queued})
			return err
		},
		Notify: func(p push.Payload) { h.pushed = append(h.pushed, p) },
		Now:    func() time.Time { return time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC) },
	}
	return h
}

func into(t *testing.T, st *Store, projectID int64, title string, s Status) Task {
	t.Helper()
	tk, err := st.Create(projectID, Patch{Title: str(title)})
	if err != nil {
		t.Fatal(err)
	}
	if tk, err = st.Update(tk.ID, Patch{StatusID: &s.ID}); err != nil {
		t.Fatal(err)
	}
	return tk
}

func note(t *testing.T, st *Store, projectID int64) QueueNote {
	t.Helper()
	p, err := st.Project(projectID)
	if err != nil || p.Queue == nil {
		t.Fatalf("no note on the project: %v", err)
	}
	return *p.Queue
}

func TestTheQueueStartsTheTopQueuedCardOnceASlotIsFree(t *testing.T) {
	st := open(t)
	p := queued(t, st, 1)
	queue := column(p, func(s Status) bool { return s.OnQueue })
	first := into(t, st, p.ID, "Fix the login page", queue)
	second := into(t, st, p.ID, "Add a dark theme", queue)
	_, _ = st.Create(p.ID, Patch{Title: str("Left in New")})
	h := newQueue(st)

	h.q.Sweep()
	if len(h.starts) != 1 || h.started[0] != first.ID {
		t.Fatalf("starts %+v of %v", h.starts, h.started)
	}
	req := h.starts[0]
	if req.Workspace != "w1" || req.Agent != "claude" || !req.Queued || req.Branch != BranchFor(first) || !strings.HasPrefix(req.Branch, "task-") {
		t.Fatalf("request %+v", req)
	}
	if n := note(t, st, p.ID); n.State != "started" || n.Working != 1 {
		t.Fatalf("note after the start %+v", n)
	}
	started, _ := st.Get(first.ID)
	if started.Run == nil || !started.Run.ByQueue || started.StatusID == queue.ID {
		t.Fatalf("the started card: run %+v in column %d", started.Run, started.StatusID)
	}

	h.q.Sweep()
	if len(h.starts) != 1 {
		t.Fatal("a second card started with every slot taken")
	}
	if n := note(t, st, p.ID); n.State != "waiting" || n.Reason != "every slot is taken: 1 of 1 working" || n.Next != second.Identifier {
		t.Fatalf("note with the slot taken %+v", n)
	}

	_ = st.SetRun(started.Run.ID, RunRunning, "")
	h.states[started.Run.Pane] = "blocked"
	h.q.Sweep()
	if len(h.starts) != 1 {
		t.Fatal("an agent asking a question freed its slot")
	}
	h.states[started.Run.Pane] = "done"
	h.q.Sweep()
	if len(h.starts) != 2 || h.started[1] != second.ID {
		t.Fatalf("the next card did not start once the slot was free: %v", h.started)
	}
	h.q.Sweep()
	if n := note(t, st, p.ID); n.State != "idle" || n.Working != 1 {
		t.Fatalf("note with nothing left queued and the second at work %+v", n)
	}
	_ = st.SetRun(mustRun(t, st, second.ID).ID, RunRunning, "")
	h.states[mustRun(t, st, second.ID).Pane] = "idle"
	h.q.Sweep()
	if n := note(t, st, p.ID); n.State != "idle" || n.Working != 0 {
		t.Fatalf("note with nothing queued %+v", n)
	}
}

func mustRun(t *testing.T, st *Store, taskID int64) Run {
	t.Helper()
	tk, err := st.Get(taskID)
	if err != nil || tk.Run == nil {
		t.Fatalf("task %d has no run: %v", taskID, err)
	}
	return *tk.Run
}

func TestTheQueueWaitsForTheMachineTheLimitTheBudgetAndAWorkspace(t *testing.T) {
	st := open(t)
	p := queued(t, st, 2)
	into(t, st, p.ID, "Waiting", column(p, func(s Status) bool { return s.OnQueue }))
	h := newQueue(st)
	for _, c := range []struct {
		set  func()
		want string
	}{
		{func() { h.connected = false }, "Studio is away"},
		{func() { h.connected, h.spent = true, "Claude's 5 h window is spent until 14:00" }, "Claude's 5 h window is spent until 14:00"},
		{func() { h.spent, h.crossed = "", "the budget for app: $20.40 of $20.00 this week" }, "the budget for app: $20.40 of $20.00 this week"},
		{func() { h.crossed, h.workspace = "", "" }, "no workspace is open on /w/app to branch off"},
	} {
		c.set()
		h.q.Sweep()
		if n := note(t, st, p.ID); n.State != "waiting" || n.Reason != c.want {
			t.Errorf("note %+v, want the reason %q", n, c.want)
		}
	}
	if len(h.starts) != 0 {
		t.Fatalf("something started while it should wait: %+v", h.starts)
	}
}

func TestARefusedStartWaitsForTheCardToMove(t *testing.T) {
	st := open(t)
	p := queued(t, st, 1)
	queue := column(p, func(s Status) bool { return s.OnQueue })
	tk := into(t, st, p.ID, "Refused", queue)
	h := newQueue(st)
	h.startErr = errors.New("herdr: no such workspace")
	h.q.Sweep()
	h.q.Sweep()
	if len(h.starts) != 1 {
		t.Fatalf("a refused start was tried %d times", len(h.starts))
	}
	got, _ := st.Get(tk.ID)
	if got.Refused != "herdr: no such workspace" || len(h.pushed) != 1 {
		t.Fatalf("refusal %q, pushes %+v", got.Refused, h.pushed)
	}
	if cs, _ := st.Comments(tk.ID); len(cs) != 1 || !strings.Contains(cs[0].Body, "could not start") {
		t.Fatalf("comments %+v", cs)
	}
	if n := note(t, st, p.ID); n.State != "idle" {
		t.Fatalf("a refused card still counts as next: %+v", n)
	}
	landing := column(p, func(s Status) bool { return s.OnNew })
	if _, err := st.Update(tk.ID, Patch{StatusID: &landing.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Update(tk.ID, Patch{StatusID: &queue.ID}); err != nil {
		t.Fatal(err)
	}
	h.startErr = nil
	h.q.Sweep()
	if len(h.starts) != 2 {
		t.Fatal("a card moved back into Queued was not tried again")
	}
}

func TestTheQueueIsOffUntilSwitchedOnAndChecksItsSettings(t *testing.T) {
	st := open(t)
	ps, _ := st.Projects()
	inbox := ps[0]
	h := newQueue(st)
	h.q.Sweep()
	if p, _ := st.Project(inbox.ID); p.Queue != nil {
		t.Fatalf("a queue that is off has a note: %+v", p.Queue)
	}
	for _, c := range []struct {
		a    Automation
		want string
	}{
		{Automation{Machine: "studio", Dir: "/w/app", Queue: true}, "a column marked Queued"},
		{Automation{Queue: true}, "where agents work"},
		{Automation{Machine: "studio"}, "both the machine and the repository folder"},
		{Automation{Machine: "studio", Dir: "app"}, "absolute"},
		{Automation{Agent: "shell"}, "an agent is needed"},
		{Automation{Max: MaxQueued + 1}, "at most"},
		{Automation{Stages: []Stage{{Name: "Tests"}}}, "needs a prompt"},
		{Automation{Stages: []Stage{{Name: "Tests", Prompt: "x", StatusID: 99999}}}, "a column this project does not have"},
		{Automation{Args: `--name "unclosed`}, "the project's agent"},
	} {
		if _, err := st.SetAutomation(inbox.ID, c.a); err == nil || !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: %v, want %q", c.a, err, c.want)
		}
	}
	p, err := st.SetAutomation(inbox.ID, Automation{Machine: " studio ", Dir: "/w/app/", Stages: []Stage{{Prompt: "Review {branch}", Agent: "codex"}}})
	if err != nil {
		t.Fatal(err)
	}
	if a := p.Automation; a.Machine != "studio" || a.Dir != "/w/app" || a.Max != 1 || a.Stages[0].Name != "Stage 1" || a.Queue {
		t.Fatalf("kept %+v", a)
	}
}

func TestBranchForReadsLikeThePagesOwn(t *testing.T) {
	for title, want := range map[string]string{
		"Fix the login page!": "task-12-fix-the-login-page",
		"  ":                  "task-12",
		"Починить вход в аккаунт":        "task-12-починить-вход-в-аккаунт",
		strings.Repeat("abcdefghij ", 6): "task-12-abcdefghij-abcdefghij-abcdefghij-abcdefg",
	} {
		if got := BranchFor(Task{ID: 12, Title: title}); got != want {
			t.Errorf("BranchFor(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestNothingLandsInTheQueuedColumnButThePersonsMove(t *testing.T) {
	st := open(t)
	ps, _ := st.Projects()
	inbox := ps[0]
	var cols []Status
	cols = append(cols, Status{Title: "Queued", Kind: KindOpen, OnQueue: true})
	for _, s := range inbox.Statuses {
		s.OnNew, s.OnAgent = false, false // no column takes new tasks or proposals: the first open one would
		cols = append(cols, s)
	}
	saved, err := st.SetStatuses(inbox.ID, cols, nil)
	if err != nil {
		t.Fatal(err)
	}
	queuedCol := *flagged(saved, func(s Status) bool { return s.OnQueue })
	mine, _ := st.CreateFrom(inbox.ID, Patch{Title: str("Mine")}, "")
	theirs, _ := st.CreateFrom(inbox.ID, Patch{Title: str("Theirs")}, "GOG · w3:p1")
	for _, tk := range []Task{mine, theirs} {
		if tk.StatusID == queuedCol.ID {
			t.Fatalf("%q landed in the Queued column", tk.Title)
		}
	}
	queuedTask, _ := st.Update(mine.ID, Patch{StatusID: &queuedCol.ID})
	done, _ := st.Update(queuedTask.ID, Patch{Done: boolp(true)})
	if undone, _ := st.Update(done.ID, Patch{Done: boolp(false)}); undone.StatusID == queuedCol.ID {
		t.Fatal("a task undone went back into the Queued column")
	}
	other, _ := st.CreateProject("Other", 0)
	var ocols []Status
	for _, s := range other.Statuses {
		ocols = append(ocols, s)
		if s.OnNew {
			ocols = append(ocols, Status{Title: "Queued", Kind: KindOpen, OnQueue: true})
		}
	}
	if _, err := st.SetStatuses(other.ID, ocols, nil); err != nil {
		t.Fatal(err)
	}
	moved, _ := st.Update(theirs.ID, Patch{StatusID: &queuedCol.ID})
	moved, err = st.Update(moved.ID, Patch{ProjectID: &other.ID})
	if err != nil {
		t.Fatal(err)
	}
	if st.QueueColumn(other.ID, moved.StatusID) {
		t.Fatal("a card moved to another project landed in its Queued column by the name")
	}
}

func TestTheQueuedColumnTakesNoOtherMark(t *testing.T) {
	st := open(t)
	ps, _ := st.Projects()
	inbox := ps[0]
	for name, change := range map[string]func(s *Status){
		"new":      func(s *Status) { s.OnNew = true },
		"proposed": func(s *Status) { s.OnAgent = true },
		"start":    func(s *Status) { s.OnStart = true },
		"finish":   func(s *Status) { s.OnFinish = true },
		"done":     func(s *Status) { s.Kind = KindDone },
	} {
		var cols []Status
		for _, s := range inbox.Statuses {
			s.OnNew, s.OnAgent, s.OnStart, s.OnFinish = false, false, false, false
			cols = append(cols, s)
		}
		q := Status{Title: "Queued", Kind: KindOpen, OnQueue: true}
		change(&q)
		cols = append(cols, q)
		if _, err := st.SetStatuses(inbox.ID, cols, nil); !errors.Is(err, ErrInvalid) {
			t.Errorf("a Queued column that also takes %s was kept: %v", name, err)
		}
	}
}

func TestAProjectKeepsAnOpenColumnBesidesTheQueuedOne(t *testing.T) {
	st := open(t)
	ps, _ := st.Projects()
	inbox := ps[0]
	cols := []Status{{Title: "Queued", Kind: KindOpen, OnQueue: true}, {Title: "Done", Kind: KindDone}}
	if _, err := st.SetStatuses(inbox.ID, cols, nil); !errors.Is(err, ErrInvalid) {
		tk, _ := st.Create(inbox.ID, Patch{Title: str("New")})
		t.Fatalf("columns with no open one but the Queued were kept (%v): a new task landed done=%v", err, tk.Done)
	}
}

func TestAStartClearsTheQueuesRefusal(t *testing.T) {
	st := open(t)
	p := queued(t, st, 1)
	q := column(p, func(s Status) bool { return s.OnQueue })
	tk := into(t, st, p.ID, "Refused", q)
	h := newQueue(st)
	h.startErr = errors.New("herdr said no")
	h.q.Sweep()
	if got, _ := st.Get(tk.ID); got.Refused == "" {
		t.Fatal("no refusal noted")
	}
	if _, err := st.StartRun(Run{TaskID: tk.ID, Machine: "studio", Workspace: "w9", Pane: "w9:p1", Agent: "claude"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.Get(tk.ID); got.Refused != "" {
		t.Fatalf("the person's start left the queue's refusal on the card: %q", got.Refused)
	}
}

func TestTheQueueSaysWhyTheServersOwnMachineRunsNothing(t *testing.T) {
	st := open(t)
	p := queued(t, st, 1)
	into(t, st, p.ID, "Waits", column(p, func(s Status) bool { return s.OnQueue }))
	h := newQueue(st)
	h.q.Unusable = func(id string) string { return "this remuda only serves the page" }
	h.q.Sweep()
	if n := note(t, st, p.ID); n.State != "waiting" || n.Reason != "this remuda only serves the page" || len(h.starts) != 0 {
		t.Fatalf("note %+v, starts %d", n, len(h.starts))
	}
}

func TestAStoreFromBeforeTheQueueOpensWithItsTasks(t *testing.T) {
	dir := t.TempDir()
	old, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ps, _ := old.Projects()
	tk, _ := old.Create(ps[0].ID, Patch{Title: str("Kept")})
	if _, err := old.StartRun(Run{TaskID: tk.ID, Machine: "studio", Workspace: "w1", Pane: "w1:p1", Agent: "claude"}); err != nil {
		t.Fatal(err)
	}
	for _, drop := range []string{ // the schema as 0.12.4 left it
		`ALTER TABLE statuses DROP COLUMN on_queue`,
		`ALTER TABLE projects DROP COLUMN automation`,
		`ALTER TABLE tasks DROP COLUMN queue_refused`,
		`ALTER TABLE task_runs DROP COLUMN by_queue`,
		`ALTER TABLE task_runs DROP COLUMN stage`,
		`ALTER TABLE task_runs DROP COLUMN pipeline`,
		`ALTER TABLE task_runs DROP COLUMN expect_status`,
	} {
		if _, err := old.db.Exec(drop); err != nil {
			t.Fatalf("%s: %v", drop, err)
		}
	}
	old.Close()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.Get(tk.ID)
	if err != nil || got.Title != "Kept" || got.Run == nil || got.Run.ByQueue || got.Run.Pipeline != "" || got.Refused != "" {
		t.Fatalf("the old task after the upgrade: %+v %v", got, err)
	}
	p, _ := st.Project(ps[0].ID)
	if p.Automation.Queue || p.Automation.Max != 1 || len(p.Automation.Stages) != 0 || flagged(p.Statuses, func(s Status) bool { return s.OnQueue }) != nil {
		t.Fatalf("the old project after the upgrade: %+v", p.Automation)
	}
}

// queuedWithoutStart is the Inbox with a Queued column after New and no column marked for started tasks, the queue on
func queuedWithoutStart(t *testing.T, st *Store) Project {
	t.Helper()
	ps, _ := st.Projects()
	inbox := ps[0]
	var cols []Status
	for _, s := range inbox.Statuses {
		s.OnStart = false
		cols = append(cols, s)
		if s.OnNew {
			cols = append(cols, Status{Title: "Queued", Kind: KindOpen, OnQueue: true})
		}
	}
	if _, err := st.SetStatuses(inbox.ID, cols, nil); err != nil {
		t.Fatal(err)
	}
	p, err := st.SetAutomation(inbox.ID, Automation{Machine: "studio", Dir: "/w/app", Agent: "claude", Queue: true, Max: 2})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestACardTheStartLeavesInQueuedIsNotStartedAgainUntilItMoves(t *testing.T) {
	st := open(t)
	p := queuedWithoutStart(t, st)
	queue := column(p, func(s Status) bool { return s.OnQueue })
	tk := into(t, st, p.ID, "Fix the login page", queue)
	h := newQueue(st)

	h.q.Sweep()
	if len(h.starts) != 1 {
		t.Fatalf("starts %d", len(h.starts))
	}
	run := mustRun(t, st, tk.ID)
	_ = st.SetRun(run.ID, RunRunning, "")
	_ = st.SetRun(run.ID, RunFinished, "rec-1")
	h.states[run.Pane] = "done"
	h.q.Sweep()
	h.q.Sweep()
	if len(h.starts) != 1 {
		t.Fatalf("the queue started another agent on a card it had started already: %d starts", len(h.starts))
	}
	if got, _ := st.Get(tk.ID); got.StatusID != queue.ID || got.Refused == "" {
		t.Fatalf("the card: column %d, note %q", got.StatusID, got.Refused)
	}

	landing := column(p, func(s Status) bool { return s.OnNew })
	if _, err := st.Update(tk.ID, Patch{StatusID: &landing.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Update(tk.ID, Patch{StatusID: &queue.ID}); err != nil {
		t.Fatal(err)
	}
	h.q.Sweep()
	if len(h.starts) != 2 {
		t.Fatalf("a card the person queued again was not started: %d starts", len(h.starts))
	}
}

func TestAStageNeverMovesTheCardIntoTheQueuedColumn(t *testing.T) {
	st := open(t)
	p := queued(t, st, 1)
	queue := column(p, func(s Status) bool { return s.OnQueue })
	if _, err := st.SetAutomation(p.ID, Automation{Machine: "studio", Dir: "/w/app", Queue: true, Stages: []Stage{{Name: "Tests", Prompt: "x", StatusID: queue.ID}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a stage moving the card into the Queued column was kept: %v", err)
	}
	review := column(p, func(s Status) bool { return s.OnFinish })
	if _, err := st.SetAutomation(p.ID, Automation{Machine: "studio", Dir: "/w/app", Stages: []Stage{{Name: "Tests", Prompt: "x", StatusID: review.ID}}}); err != nil {
		t.Fatal(err)
	}
	var cols []Status // the stage's column is made the Queued one afterwards
	for _, s := range p.Statuses {
		switch {
		case s.OnQueue:
			s.OnQueue = false
		case s.ID == review.ID:
			s.OnFinish, s.OnQueue = false, true
		}
		cols = append(cols, s)
	}
	if _, err := st.SetStatuses(p.ID, cols, nil); err != nil {
		t.Fatal(err)
	}
	tk, _ := st.Create(p.ID, Patch{Title: str("Fix login")})
	first, err := st.StartRun(Run{TaskID: tk.ID, Machine: "studio", Workspace: "w1", Pane: "w1:p1", Agent: "claude", Pipeline: PipelineOn})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := st.Get(tk.ID)
	if _, err := st.StartStageRun(first, 1, Stage{Name: "Tests", Prompt: "x", StatusID: review.ID}, "w1:p2"); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.Get(tk.ID); got.StatusID != before.StatusID || st.QueueColumn(p.ID, got.StatusID) {
		t.Fatalf("the stage moved the card from %d to %d, the Queued column", before.StatusID, got.StatusID)
	}
}
