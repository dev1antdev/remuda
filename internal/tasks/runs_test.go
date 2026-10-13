package tasks

import (
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/push"
)

func TestPromptFlattensHTML(t *testing.T) {
	tk := Task{Title: " Fix login ", Identifier: "#7", Description: "<p>Steps:</p><ul><li>open &lt;app&gt;</li><li>click<br>twice</li></ul>"}
	got := Prompt(tk, Project{Title: "Work"}, true)
	want := "/grill-me Fix login\n\nSteps:\n- open <app>\n- click\ntwice\n\n(remuda task #7 in Work)"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if Prompt(Task{Title: "x", Identifier: "#1"}, Project{}, false) != "x\n\n(remuda task #1)" {
		t.Fatal("plain")
	}
	project := Project{Title: "Work", Fields: []Field{
		{ID: 1, Title: "Area", Kind: FieldSelect, InPrompt: true},
		{ID: 2, Title: "Secret", Kind: FieldText, InPrompt: false},
		{ID: 3, Title: "Estimate", Kind: FieldNumber, InPrompt: true},
		{ID: 4, Title: "Tags", Kind: FieldMulti, InPrompt: true},
		{ID: 5, Title: "Empty", Kind: FieldText, InPrompt: true},
		{ID: 6, Title: "Blocking", Kind: FieldCheck, InPrompt: true},
		{ID: 7, Title: "Due", Kind: FieldDate, InPrompt: true},
	}}
	tk = Task{Title: "x", Identifier: "#1", Fields: map[string]any{"1": "web", "2": "hidden", "3": 2.5, "4": []any{"a", "b"}, "6": true, "7": "2026-10-12T00:00:00Z"}}
	want = "x\n\nArea: web\nEstimate: 2.5\nTags: a, b\nBlocking: yes\nDue: 2026-10-12\n\n(remuda task #1 in Work)"
	if got := Prompt(tk, project, false); got != want {
		t.Fatalf("with fields:\n%q\nwant\n%q", got, want)
	}
}

func TestRunLifecycleAndSweep(t *testing.T) {
	st := open(t)
	clock := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	st.now = func() time.Time { return clock }
	inbox, _ := st.Projects()
	tk, _ := st.Create(inbox[0].ID, Patch{Title: str("Ship it")})
	run, err := st.StartRun(Run{TaskID: tk.ID, Machine: "studio", Workspace: "wX", Pane: "wX:p1", Dir: "/w/x", Branch: "task-1-ship-it", Agent: "claude"})
	if err != nil || run.State != RunStarting || run.ID == 0 {
		t.Fatalf("start %+v %v", run, err)
	}
	project, _ := st.Project(inbox[0].ID)
	start := flagged(project.Statuses, func(s Status) bool { return s.OnStart })
	finish := flagged(project.Statuses, func(s Status) bool { return s.OnFinish })
	got, _ := st.Get(tk.ID)
	if got.Run == nil || got.Run.ID != run.ID || got.StatusID != start.ID {
		t.Fatalf("task after start %+v, column %d want %d", got.Run, got.StatusID, start.ID)
	}
	list, _, _ := st.List(Query{View: "open"})
	if list[0].Run == nil {
		t.Fatal("the list carries no run")
	}

	state, answer, answerID := "working", "", ""
	gone := false
	var pushed []push.Payload
	w := &RunWatch{
		Store:      st,
		PaneState:  func(m, p string) (string, time.Time, bool) { return state, clock, !gone },
		LastAnswer: func(m, p string) (string, string) { return answer, answerID },
		Notify:     func(p push.Payload) { pushed = append(pushed, p) },
		Now:        func() time.Time { return clock },
	}
	w.Sweep()
	clock = clock.Add(time.Minute)
	if err := st.SetRun(run.ID, RunRunning, ""); err != nil {
		t.Fatal(err)
	}
	w.Sweep()
	if cs, _ := st.Comments(tk.ID); len(cs) != 0 {
		t.Fatalf("comments while working: %+v", cs)
	}
	state, answer, answerID = "idle", "Done: shipped in #42.\n\nTests pass.", "rec-a1"
	clock = clock.Add(time.Minute)
	w.Sweep()
	cs, _ := st.Comments(tk.ID)
	if len(cs) != 1 || cs[0].Author != "agent" || !strings.HasPrefix(cs[0].Body, "Done: shipped") {
		t.Fatalf("comments after a finish: %+v", cs)
	}
	if r, _ := st.Run(run.ID); r.State != RunFinished || r.Finished == "" {
		t.Fatalf("run %+v", r)
	}
	if len(pushed) != 1 || !strings.Contains(pushed[0].Title, "#1") || !strings.Contains(pushed[0].Title, "→ Testing") || pushed[0].Kind != "task" {
		t.Fatalf("pushed %+v", pushed)
	}
	if got, _ = st.Get(tk.ID); got.StatusID != finish.ID || got.Done {
		t.Fatalf("the finish did not move the task: column %d want %d", got.StatusID, finish.ID)
	}
	w.Sweep()
	if cs, _ := st.Comments(tk.ID); len(cs) != 1 {
		t.Fatalf("reported twice: %+v", cs)
	}
	w2 := &RunWatch{Store: st, PaneState: w.PaneState, LastAnswer: w.LastAnswer, Notify: w.Notify, Now: w.Now}
	w2.Sweep()
	if cs, _ := st.Comments(tk.ID); len(cs) != 1 {
		t.Fatalf("reported again after a restart: %+v", cs)
	}
	answer, answerID = "Also fixed the typo.", "rec-b7"
	clock = clock.Add(11 * time.Minute)
	w.Sweep()
	if cs, _ := st.Comments(tk.ID); len(cs) != 2 {
		t.Fatalf("second finish: %+v", cs)
	}
	if got, _ = st.Get(tk.ID); got.StatusID != finish.ID {
		t.Fatalf("a second finish moved the task: column %d", got.StatusID)
	}
	got, _ = st.Update(tk.ID, Patch{Done: boolp(true)})
	if !got.Done || got.StatusID != doneStatus(project.Statuses).ID {
		t.Fatalf("done: %+v", got)
	}
	gone = true
	w.Sweep()
	if r, _ := st.Run(run.ID); r.State != RunFinished {
		t.Fatalf("gone after one look: %+v", r)
	}
	clock = clock.Add(3 * time.Minute)
	w.Sweep()
	if r, _ := st.Run(run.ID); r.State != RunGone {
		t.Fatalf("gone: %+v", r)
	}
	if cs, _ := st.Comments(tk.ID); len(cs) != 3 || cs[2].Author != "remuda" {
		t.Fatalf("gone comment: %+v", cs)
	}
	w.Sweep()
	if cs, _ := st.Comments(tk.ID); len(cs) != 3 {
		t.Fatal("gone twice")
	}
}

func TestFreshPaneIsNotGoneYet(t *testing.T) {
	st := open(t)
	clock := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	st.now = func() time.Time { return clock }
	inbox, _ := st.Projects()
	tk, _ := st.Create(inbox[0].ID, Patch{Title: str("x")})
	run, _ := st.StartRun(Run{TaskID: tk.ID, Machine: "m", Workspace: "w", Pane: "w:p1", Agent: "claude"})
	w := &RunWatch{Store: st, PaneState: func(m, p string) (string, time.Time, bool) { return "", time.Time{}, false }, LastAnswer: func(m, p string) (string, string) { return "", "" }, Now: func() time.Time { return clock }}
	clock = clock.Add(30 * time.Second)
	w.Sweep()
	if r, _ := st.Run(run.ID); r.State != RunStarting {
		t.Fatalf("given up on a pane herdr has not shown yet: %+v", r)
	}
	clock = clock.Add(3 * time.Minute)
	w.Sweep()
	if r, _ := st.Run(run.ID); r.State != RunGone {
		t.Fatalf("not gone: %+v", r)
	}
	tk2, _ := st.Create(inbox[0].ID, Patch{Title: str("y")})
	run2, _ := st.StartRun(Run{TaskID: tk2.ID, Machine: "m", Workspace: "w", Pane: "w:p2", Agent: "claude"})
	w.PaneState = func(m, p string) (string, time.Time, bool) { return "unknown", time.Time{}, true }
	clock = clock.Add(5 * time.Minute)
	w.Sweep()
	if r, _ := st.Run(run2.ID); r.State != RunStarting {
		t.Fatalf("failed too soon: %+v", r)
	}
	clock = clock.Add(2 * time.Minute)
	w.Sweep()
	if r, _ := st.Run(run2.ID); r.State != RunFailed {
		t.Fatalf("not failed: %+v", r)
	}
	if _, err := st.AddComment(tk.ID, "you", "  "); err == nil {
		t.Fatal("an empty comment")
	}
	c, _ := st.AddComment(tk.ID, "you", "note")
	if err := st.DeleteComment(c.ID); err != nil {
		t.Fatal(err)
	}
}
