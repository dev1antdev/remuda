package tasks

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/push"
)

func TestAMergedPullRequestFinishesTheTask(t *testing.T) {
	st := open(t)
	clock := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	st.now = func() time.Time { return clock }
	inbox, _ := st.Projects()
	tk, _ := st.Create(inbox[0].ID, Patch{Title: str("Ship it")})
	run, err := st.StartRun(Run{TaskID: tk.ID, Machine: "studio", Workspace: "wX", Pane: "wX:p1", Dir: "/w/x", Branch: "task-1-ship-it", Agent: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	project, _ := st.Project(inbox[0].ID)
	done := doneStatus(project.Statuses)
	var pushed []push.Payload
	var touched []string
	w := &RunWatch{Store: st, Now: func() time.Time { return clock }, Notify: func(p push.Payload) { pushed = append(pushed, p) }, PRTouch: func(m, p string) { touched = append(touched, m+"|"+p) },
		PaneState:  func(string, string) (string, time.Time, bool) { return "done", clock, true },
		LastAnswer: func(string, string) (string, string) { return "", "" }}

	w.OnPullRequestState("studio", "wX:p1", "https://github.com/o/r/pull/15", "MERGED") // no run names it yet
	if cs, _ := st.Comments(tk.ID); len(cs) != 0 || len(pushed) != 0 {
		t.Fatalf("a request the run does not name was noted: %+v", cs)
	}

	w.Sweep()
	if len(touched) != 0 {
		t.Fatal("a run without a request asked for a read")
	}
	w.OnPullRequest("studio", "wX:p1", "https://github.com/o/r/pull/15", "PR #15")
	w.Sweep()
	if len(touched) != 1 || touched[0] != "studio|wX:p1" {
		t.Fatalf("touched %v", touched)
	}
	if task, _ := st.Get(tk.ID); task.Done {
		t.Fatal("the task is done before the merge")
	}

	w.OnPullRequestState("studio", "wX:p1", "https://github.com/o/r/pull/15", "OPEN")
	if cs, _ := st.Comments(tk.ID); len(cs) != 1 {
		t.Fatalf("an open request moved something: %+v", cs)
	}
	w.OnPullRequestState("studio", "wX:p1", "https://github.com/o/r/pull/15", "MERGED")
	task, _ := st.Get(tk.ID)
	if !task.Done || task.StatusID != done.ID {
		t.Fatalf("the task did not finish with the merge: %+v", task)
	}
	cs, _ := st.Comments(tk.ID)
	if len(cs) != 2 || cs[1].Author != "remuda" || cs[1].Body != "PR #15 merged" {
		t.Fatalf("comments: %+v", cs)
	}
	if len(pushed) != 2 || pushed[1].Title != "Task #1: PR #15 merged → Done" || pushed[1].Kind != "task" || pushed[1].Pane != "wX:p1" {
		t.Fatalf("pushed %+v", pushed)
	}

	w.OnPullRequestState("studio", "wX:p1", "https://github.com/o/r/pull/15", "MERGED") // read again after a restart
	if cs, _ := st.Comments(tk.ID); len(cs) != 2 || len(pushed) != 2 {
		t.Fatalf("noted twice: %+v %+v", cs, pushed)
	}

	// a card the person moved elsewhere stays there, the merge still noted
	other, _ := st.Create(inbox[0].ID, Patch{Title: str("Second")})
	run2, _ := st.StartRun(Run{TaskID: other.ID, Machine: "studio", Workspace: "wY", Pane: "wY:p1", Dir: "/w/y", Branch: "task-2-second", Agent: "claude"})
	w.OnPullRequest("studio", "wY:p1", "https://github.com/o/r/pull/16", "PR #16")
	byHand := project.Statuses[0].ID
	if _, err := st.Update(other.ID, Patch{StatusID: &byHand}); err != nil {
		t.Fatal(err)
	}
	w.OnPullRequestState("studio", "wY:p1", "https://github.com/o/r/pull/16", "MERGED")
	if got, _ := st.Get(other.ID); got.StatusID != byHand || got.Done {
		t.Fatalf("a card the person moved must stay: %+v", got)
	}
	if cs, _ := st.Comments(other.ID); len(cs) != 2 || cs[1].Body != "PR #16 merged" {
		t.Fatalf("comments: %+v", cs)
	}
	if len(pushed) != 4 || pushed[3].Title != "Task #2: PR #16 merged" {
		t.Fatalf("pushes %+v", pushed)
	}
	_ = run
	_ = run2
}
