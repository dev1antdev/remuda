package tasks

import (
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/push"
)

func TestPullRequestNamedOnARun(t *testing.T) {
	st := open(t)
	clock := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	st.now = func() time.Time { return clock }
	inbox, _ := st.Projects()
	tk, _ := st.Create(inbox[0].ID, Patch{Title: str("Ship it")})
	run, err := st.StartRun(Run{TaskID: tk.ID, Machine: "studio", Workspace: "wX", Pane: "wX:p1", Dir: "/w/x", Branch: "task-1-ship-it", Agent: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	project, _ := st.Project(inbox[0].ID)
	finish := flagged(project.Statuses, func(s Status) bool { return s.OnFinish })
	var pushed []push.Payload
	w := &RunWatch{Store: st, Notify: func(p push.Payload) { pushed = append(pushed, p) }}

	w.OnPullRequest("studio", "wX:p9", "https://github.com/o/r/pull/9", "PR #9") // a pane without a run
	if cs, _ := st.Comments(tk.ID); len(cs) != 0 || len(pushed) != 0 {
		t.Fatalf("a pane without a run was noted: %+v %+v", cs, pushed)
	}

	w.OnPullRequest("studio", "wX:p1", "https://github.com/o/r/pull/15", "PR #15")
	got, _ := st.Run(run.ID)
	if got.PRURL != "https://github.com/o/r/pull/15" || got.PRLabel != "PR #15" {
		t.Fatalf("the run did not note the request: %+v", got)
	}
	cs, _ := st.Comments(tk.ID)
	if len(cs) != 1 || cs[0].Author != "agent" || cs[0].Body != "Opened PR #15: https://github.com/o/r/pull/15" {
		t.Fatalf("comment: %+v", cs)
	}
	if task, _ := st.Get(tk.ID); task.StatusID != finish.ID || task.Run == nil || task.Run.PRLabel != "PR #15" {
		t.Fatalf("the task did not move with its run: %+v", task)
	}
	if len(pushed) != 1 || pushed[0].Title != "Task #1: PR #15 opened → Testing" || pushed[0].Kind != "task" || !strings.Contains(pushed[0].Body, "pull/15") || pushed[0].Pane != "wX:p1" {
		t.Fatalf("pushed %+v", pushed)
	}

	w.OnPullRequest("studio", "wX:p1", "https://github.com/o/r/pull/15", "PR #15") // the same link again
	if cs, _ := st.Comments(tk.ID); len(cs) != 1 || len(pushed) != 1 {
		t.Fatalf("noted twice: %+v %+v", cs, pushed)
	}

	start := flagged(project.Statuses, func(s Status) bool { return s.OnStart })
	if _, err := st.Update(tk.ID, Patch{StatusID: &start.ID}); err != nil {
		t.Fatal(err)
	}
	newColumn := project.Statuses[0].ID // the person files it elsewhere by hand
	if _, err := st.Update(tk.ID, Patch{StatusID: &newColumn}); err != nil {
		t.Fatal(err)
	}
	w.OnPullRequest("studio", "wX:p1", "https://github.com/o/r/pull/16", "PR #16")
	if task, _ := st.Get(tk.ID); task.StatusID != newColumn || task.Run.PRLabel != "PR #16" {
		t.Fatalf("a card the person moved must stay, the link still noted: %+v", task)
	}
	if len(pushed) != 2 || pushed[1].Title != "Task #1: PR #16 opened" {
		t.Fatalf("second push %+v", pushed)
	}

	_ = st.SetRun(run.ID, RunGone, "closed")
	w.OnPullRequest("studio", "wX:p1", "https://github.com/o/r/pull/17", "PR #17")
	if cs, _ := st.Comments(tk.ID); len(cs) != 2 {
		t.Fatalf("a gone run took a link: %+v", cs)
	}
}
