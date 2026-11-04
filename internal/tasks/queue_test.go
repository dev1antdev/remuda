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
			h.starts = append(h.starts, req)
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
