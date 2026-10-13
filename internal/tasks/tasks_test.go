package tasks

import (
	"errors"
	"testing"
	"time"
)

func open(t *testing.T) *Store {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func str(s string) *string { return &s }
func boolp(b bool) *bool   { return &b }
func intp(i int) *int      { return &i }

func TestInboxExistsAndProjectsNest(t *testing.T) {
	st := open(t)
	ps, _ := st.Projects()
	if len(ps) != 1 || ps[0].Title != "Inbox" {
		t.Fatalf("got %+v", ps)
	}
	work, err := st.CreateProject("Work", 0)
	if err != nil {
		t.Fatal(err)
	}
	sub, _ := st.CreateProject("Sub", work.ID)
	if sub.ParentID != work.ID {
		t.Fatalf("parent %d", sub.ParentID)
	}
	if _, err := st.CreateProject("Orphan", 999); err == nil {
		t.Fatal("a missing parent was accepted")
	}
	if _, err := st.UpdateProject(work.ID, ProjectPatch{ParentID: &work.ID}); err == nil {
		t.Fatal("a project went inside itself")
	}
	if _, err := st.Create(sub.ID, Patch{Title: str("in sub")}); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteProject(work.ID); err != nil {
		t.Fatal(err)
	}
	ps, _ = st.Projects()
	if len(ps) != 2 || ps[1].Title != "Sub" || ps[1].ParentID != 0 {
		t.Fatalf("after delete %+v", ps)
	}
	if tasks, _, _ := st.List(Query{View: "project", Project: sub.ID}); len(tasks) != 1 {
		t.Fatalf("sub's task is gone: %+v", tasks)
	}
}

func TestTasksRoundTrip(t *testing.T) {
	st := open(t)
	st.now = func() time.Time { return time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC) }
	inbox, _ := st.Projects()
	tk, err := st.Create(inbox[0].ID, Patch{Title: str("  Write the thing  "), DueDate: str("2026-10-08T15:00:00+03:00"), Priority: intp(9)})
	if err != nil {
		t.Fatal(err)
	}
	if tk.Title != "Write the thing" || tk.DueDate != "2026-10-08T12:00:00Z" || tk.Priority != 5 || tk.Identifier != "#1" || tk.Done {
		t.Fatalf("created %+v", tk)
	}
	if _, err := st.Create(inbox[0].ID, Patch{Title: str("  ")}); err == nil {
		t.Fatal("an empty title was accepted")
	}
	if _, err := st.Create(77, Patch{Title: str("x")}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing project: %v", err)
	}
	tk, err = st.Update(tk.ID, Patch{Done: boolp(true), Description: str("<p>why</p>")})
	if err != nil || !tk.Done || tk.DoneAt != "2026-10-07T12:00:00Z" || tk.Description != "<p>why</p>" {
		t.Fatalf("done: %+v %v", tk, err)
	}
	tk, _ = st.Update(tk.ID, Patch{Done: boolp(false), DueDate: str("")})
	if tk.Done || tk.DoneAt != "" || tk.DueDate != "" {
		t.Fatalf("undone: %+v", tk)
	}
	if _, err := st.Update(tk.ID, Patch{DueDate: str("tomorrow")}); err == nil {
		t.Fatal("a bad date was accepted")
	}
	if err := st.Delete(tk.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Get(tk.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
	if err := st.Delete(tk.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

func TestLabels(t *testing.T) {
	st := open(t)
	inbox, _ := st.Projects()
	tk, _ := st.Create(inbox[0].ID, Patch{Title: str("a")})
	l, _ := st.CreateLabel("Bug", "#e74c3c")
	again, _ := st.CreateLabel("bug", "000000")
	if again.ID != l.ID || l.HexColor != "e74c3c" {
		t.Fatalf("labels %+v %+v", l, again)
	}
	if err := st.AddLabel(tk.ID, l.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.AddLabel(tk.ID, l.ID); err != nil {
		t.Fatal("adding twice should be fine")
	}
	if err := st.AddLabel(tk.ID, 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing label: %v", err)
	}
	got, _ := st.Get(tk.ID)
	if len(got.Labels) != 1 || got.Labels[0].Title != "Bug" {
		t.Fatalf("got %+v", got.Labels)
	}
	tasks, _, _ := st.List(Query{View: "open"})
	if len(tasks[0].Labels) != 1 {
		t.Fatalf("list labels %+v", tasks[0].Labels)
	}
	_ = st.RemoveLabel(tk.ID, l.ID)
	got, _ = st.Get(tk.ID)
	if len(got.Labels) != 0 {
		t.Fatalf("still %+v", got.Labels)
	}
	_ = st.AddLabel(tk.ID, l.ID)
	if err := st.DeleteLabel(l.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = st.Get(tk.ID)
	if len(got.Labels) != 0 {
		t.Fatalf("a deleted label stayed on the task: %+v", got.Labels)
	}
}

func TestViewsSearchAndPaging(t *testing.T) {
	st := open(t)
	kyiv, _ := time.LoadLocation("Europe/Kyiv")
	now := time.Date(2026, 10, 7, 22, 30, 0, 0, kyiv) // 22:30 in Kyiv is 19:30 UTC: still the 7th in both
	st.now = func() time.Time { return now }
	inbox, _ := st.Projects()
	work, _ := st.CreateProject("Work", 0)
	mk := func(p int64, title, due string) Task {
		t.Helper()
		pt := Patch{Title: str(title)}
		if due != "" {
			pt.DueDate = str(due)
		}
		tk, err := st.Create(p, pt)
		if err != nil {
			t.Fatal(err)
		}
		return tk
	}
	mk(inbox[0].ID, "Overdue", "2026-10-01T09:00:00Z")
	lateToday := mk(inbox[0].ID, "Late today in Kyiv", "2026-10-07T20:30:00Z") // 23:30 Kyiv
	mk(work.ID, "Next week", "2026-10-13T09:00:00Z")
	mk(work.ID, "Far", "2026-12-01T09:00:00Z")
	undated := mk(work.ID, "Купить молоко", "")
	done := mk(inbox[0].ID, "Done already", "2026-10-07T10:00:00Z")
	_, _ = st.Update(done.ID, Patch{Done: boolp(true)})

	titles := func(view string, q Query) []string {
		q.View = view
		q.Loc = kyiv
		tasks, _, err := st.List(q)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, x := range tasks {
			out = append(out, x.Title)
		}
		return out
	}
	if got := titles("today", Query{}); len(got) != 2 || got[0] != "Overdue" || got[1] != lateToday.Title {
		t.Fatalf("today: %v", got)
	}
	if got := titles("upcoming", Query{}); len(got) != 3 || got[2] != "Next week" {
		t.Fatalf("upcoming: %v", got)
	}
	if got := titles("open", Query{}); len(got) != 5 || got[4] != undated.Title {
		t.Fatalf("open: %v", got)
	}
	if got := titles("project", Query{Project: inbox[0].ID}); len(got) != 2 {
		t.Fatalf("project open: %v", got)
	}
	if got := titles("project", Query{Project: inbox[0].ID, Done: true}); len(got) != 3 || got[2] != "Done already" {
		t.Fatalf("project with done: %v", got)
	}
	if got := titles("open", Query{Search: "МОЛОКО"}); len(got) != 1 || got[0] != undated.Title {
		t.Fatalf("search: %v", got)
	}
	_, _ = st.Update(undated.ID, Patch{Description: str("<p>и <b>хлеб</b></p>")})
	if got := titles("open", Query{Search: "хлеб"}); len(got) != 1 {
		t.Fatalf("search in description: %v", got)
	}
	for i := 0; i < 60; i++ {
		mk(work.ID, "bulk", "")
	}
	tasks, pages, _ := st.List(Query{View: "open", Page: 2})
	if pages != 2 || len(tasks) != 65-PageSize {
		t.Fatalf("paging: %d pages, %d on page 2", pages, len(tasks))
	}
	if _, pages, _ := st.List(Query{View: "open", Page: 9}); pages != 2 {
		t.Fatalf("pages %d", pages)
	}
	if _, _, err := st.List(Query{View: "nope"}); err == nil {
		t.Fatal("unknown view accepted")
	}
}

// ---- the workflow ----

func titles(ss []Status) []string {
	var out []string
	for _, s := range ss {
		out = append(out, s.Title)
	}
	return out
}

func TestWorkflowDefaultsAndCopy(t *testing.T) {
	st := open(t)
	inbox, _ := st.Projects()
	ss := inbox[0].Statuses
	if got := titles(ss); len(got) != 4 || got[0] != "New" || got[3] != "Done" {
		t.Fatalf("default workflow %v", got)
	}
	if !ss[0].OnNew || !ss[1].OnStart || !ss[2].OnFinish || ss[3].Kind != KindDone || ss[0].Kind != KindOpen {
		t.Fatalf("default flags %+v", ss)
	}
	ss[2].Title = "Review"
	ss = append(ss, Status{Title: "Blocked", HexColor: "#e74c3c"})
	if _, err := st.SetStatuses(inbox[0].ID, ss, nil); err != nil {
		t.Fatal(err)
	}
	child, err := st.CreateProject("Child", inbox[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := titles(child.Statuses); len(got) != 5 || got[2] != "Review" || got[4] != "Blocked" {
		t.Fatalf("child workflow %v", got)
	}
	if child.Statuses[4].HexColor != "e74c3c" || child.Statuses[0].ID == inbox[0].Statuses[0].ID || child.Statuses[0].ProjectID != child.ID {
		t.Fatalf("child columns are not its own: %+v", child.Statuses)
	}
	top, _ := st.CreateProject("Top", 0)
	if got := titles(top.Statuses); len(got) != 4 || got[2] != "Testing" {
		t.Fatalf("a top-level project gets the default: %v", got)
	}
}

func TestSetStatusesRules(t *testing.T) {
	st := open(t)
	inbox, _ := st.Projects()
	id := inbox[0].ID
	ss := inbox[0].Statuses
	bad := func(what string, want []Status, moves map[int64]int64) {
		t.Helper()
		if _, err := st.SetStatuses(id, want, moves); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s: accepted (%v)", what, err)
		}
	}
	bad("no columns", nil, nil)
	bad("no done column", []Status{{Title: "A"}}, nil)
	bad("no open column", []Status{{Title: "A", Kind: KindDone}}, nil)
	bad("two columns for new tasks", []Status{{Title: "A", OnNew: true}, {Title: "B", OnNew: true}, {Title: "D", Kind: KindDone}}, nil)
	bad("a nameless column", []Status{{Title: " "}, {Title: "D", Kind: KindDone}}, nil)
	bad("a strange kind", []Status{{Title: "A", Kind: "weird"}, {Title: "D", Kind: KindDone}}, nil)
	bad("another project's column", []Status{{ID: 999, Title: "A"}, {Title: "D", Kind: KindDone}}, nil)
	tk, _ := st.Create(id, Patch{Title: str("x")})
	if tk.StatusID != ss[0].ID {
		t.Fatalf("a new task is not in New: %d", tk.StatusID)
	}
	bad("dropping a column with a task", ss[1:], nil)
	bad("moving them to a column that goes too", ss[1:], map[int64]int64{ss[0].ID: ss[0].ID})
	got, err := st.SetStatuses(id, ss[1:], map[int64]int64{ss[0].ID: ss[3].ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Title != "In progress" || got[0].Position != 1 {
		t.Fatalf("after the drop %+v", got)
	}
	if tk, _ = st.Get(tk.ID); tk.StatusID != ss[3].ID || !tk.Done || tk.DoneAt == "" {
		t.Fatalf("the moved task %+v", tk)
	}
	got[2].Kind = KindOpen
	got = append(got, Status{Title: "Shipped", Kind: KindDone})
	if got, err = st.SetStatuses(id, got, nil); err != nil {
		t.Fatal(err)
	}
	if tk, _ = st.Get(tk.ID); tk.Done || tk.DoneAt != "" || tk.StatusID != got[2].ID {
		t.Fatalf("done did not follow the kind %+v", tk)
	}
	if _, err := st.SetStatuses(999, got, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing project: %v", err)
	}
}

func TestColumnAndDoneFollowEachOther(t *testing.T) {
	st := open(t)
	inbox, _ := st.Projects()
	id := inbox[0].ID
	ss := inbox[0].Statuses
	newCol, progress, done := ss[0], ss[1], ss[3]
	tk, _ := st.Create(id, Patch{Title: str("a")})
	tk, _ = st.Update(tk.ID, Patch{Done: boolp(true)})
	if tk.StatusID != done.ID || !tk.Done {
		t.Fatalf("done %+v", tk)
	}
	tk, _ = st.Update(tk.ID, Patch{Done: boolp(false)})
	if tk.StatusID != newCol.ID || tk.Done || tk.DoneAt != "" {
		t.Fatalf("undone %+v", tk)
	}
	tk, _ = st.Update(tk.ID, Patch{StatusID: &done.ID})
	if !tk.Done || tk.DoneAt == "" {
		t.Fatalf("into the done column %+v", tk)
	}
	tk, _ = st.Update(tk.ID, Patch{StatusID: &progress.ID})
	if tk.Done || tk.StatusID != progress.ID {
		t.Fatalf("back out %+v", tk)
	}
	if _, err := st.Update(tk.ID, Patch{StatusID: intp64(999)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a foreign column: %v", err)
	}
	born, _ := st.Create(id, Patch{Title: str("b"), Done: boolp(true)})
	if born.StatusID != done.ID || !born.Done {
		t.Fatalf("born done %+v", born)
	}
	placed, _ := st.Create(id, Patch{Title: str("c"), StatusID: &progress.ID})
	if placed.StatusID != progress.ID {
		t.Fatalf("placed %+v", placed)
	}
	open, _, _ := st.List(Query{View: "open"})
	if len(open) != 2 {
		t.Fatalf("open: %d", len(open))
	}
	other, _ := st.CreateProject("Other", 0)
	os := other.Statuses
	renamed := append([]Status{}, os...)
	renamed[1].Title = "Doing"
	renamed, _ = st.SetStatuses(other.ID, renamed, nil)
	tk, _ = st.Update(tk.ID, Patch{ProjectID: &other.ID}) // was in "In progress": no such name there
	if tk.ProjectID != other.ID || tk.StatusID != renamed[0].ID {
		t.Fatalf("moved by state %+v", tk)
	}
	born, _ = st.Update(born.ID, Patch{ProjectID: &other.ID}) // "Done" → "Done"
	if born.StatusID != renamed[3].ID || !born.Done {
		t.Fatalf("moved by name %+v", born)
	}
}

func intp64(i int64) *int64 { return &i }

func TestPositionsInAColumn(t *testing.T) {
	st := open(t)
	inbox, _ := st.Projects()
	id := inbox[0].ID
	col := inbox[0].Statuses[0]
	a, _ := st.Create(id, Patch{Title: str("a")})
	b, _ := st.Create(id, Patch{Title: str("b")})
	c, _ := st.Create(id, Patch{Title: str("c")})
	order := func() []string {
		t.Helper()
		ts, _, err := st.List(Query{View: "board", Project: id})
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, x := range ts {
			out = append(out, x.Title)
		}
		return out
	}
	if got := order(); len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("new tasks go to the bottom: %v", got)
	}
	if _, err := st.Update(c.ID, Patch{After: intp64(0)}); err != nil {
		t.Fatal(err)
	}
	if got := order(); got[0] != "c" || got[1] != "a" {
		t.Fatalf("to the top: %v", got)
	}
	if _, err := st.Update(a.ID, Patch{After: &b.ID}); err != nil {
		t.Fatal(err)
	}
	if got := order(); got[0] != "c" || got[1] != "b" || got[2] != "a" {
		t.Fatalf("after b: %v", got)
	}
	if _, err := st.Update(a.ID, Patch{After: intp64(999)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("after a task that is not there: %v", err)
	}
	// no room between neighbours (c and a both at 0): the column is renumbered first
	if _, err := st.db.Exec(`UPDATE tasks SET position = 0`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Update(b.ID, Patch{After: &c.ID}); err != nil {
		t.Fatal(err)
	}
	if got := order(); got[0] != "c" || got[1] != "b" || got[2] != "a" {
		t.Fatalf("after renumbering: %v", got)
	}
	ts, _, _ := st.List(Query{View: "board", Project: id})
	if !(ts[0].Position < ts[1].Position && ts[1].Position < ts[2].Position) {
		t.Fatalf("not renumbered: %v %v %v", ts[0].Position, ts[1].Position, ts[2].Position)
	}
	progress := inbox[0].Statuses[1]
	x, _ := st.Create(id, Patch{Title: str("x"), StatusID: &progress.ID})
	moved, _ := st.Update(b.ID, Patch{StatusID: &progress.ID})
	if moved.Position <= x.Position {
		t.Fatalf("not at the bottom: %v vs %v", moved.Position, x.Position)
	}
	_ = col
}

func TestBoardView(t *testing.T) {
	st := open(t)
	inbox, _ := st.Projects()
	id := inbox[0].ID
	ss := inbox[0].Statuses
	for i := 0; i < BoardDone+5; i++ {
		tk, _ := st.Create(id, Patch{Title: str("done"), Done: boolp(true)})
		_ = tk
	}
	_, _ = st.Create(id, Patch{Title: str("testing"), StatusID: &ss[2].ID})
	_, _ = st.Create(id, Patch{Title: str("new")})
	ts, pages, err := st.List(Query{View: "board", Project: id})
	if err != nil {
		t.Fatal(err)
	}
	if pages != 1 || len(ts) != BoardDone+2 {
		t.Fatalf("board: %d tasks on %d pages", len(ts), pages)
	}
	if ts[0].Title != "new" || ts[1].Title != "testing" || !ts[2].Done {
		t.Fatalf("column order: %s %s %v", ts[0].Title, ts[1].Title, ts[2].Done)
	}
	if got, _, _ := st.List(Query{View: "board", Project: id, Search: "test"}); len(got) != 1 {
		t.Fatalf("search on the board: %d", len(got))
	}
	if _, _, err := st.List(Query{View: "board", Project: 999}); err != nil {
		t.Fatalf("an unknown project is an empty board: %v", err)
	}
}

func TestLegacyTasksFindTheirColumn(t *testing.T) {
	st := open(t)
	inbox, _ := st.Projects()
	id := inbox[0].ID
	ss := inbox[0].Statuses
	plain, _ := st.Create(id, Patch{Title: str("plain")})
	finished, _ := st.Create(id, Patch{Title: str("finished"), Done: boolp(true)})
	busy, _ := st.Create(id, Patch{Title: str("busy")})
	l, _ := st.CreateLabel("In Progress", "")
	_ = st.AddLabel(busy.ID, l.ID)
	// as a store from before the workflow left them
	if _, err := st.db.Exec(`UPDATE tasks SET status_id = 0`); err != nil {
		t.Fatal(err)
	}
	if err := st.migrate(); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		id   int64
		want int64
	}{{plain.ID, ss[0].ID}, {finished.ID, ss[3].ID}, {busy.ID, ss[1].ID}} {
		got, _ := st.Get(c.id)
		if got.StatusID != c.want {
			t.Fatalf("%s: column %d want %d", got.Title, got.StatusID, c.want)
		}
	}
	// a project from before gets the default workflow
	if _, err := st.db.Exec(`DELETE FROM statuses WHERE project_id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if err := st.migrate(); err != nil {
		t.Fatal(err)
	}
	p, _ := st.Project(id)
	if got := titles(p.Statuses); len(got) != 4 || got[0] != "New" {
		t.Fatalf("reseeded %v", got)
	}
	if got, _ := st.Get(finished.ID); got.StatusID != p.Statuses[3].ID || !got.Done {
		t.Fatalf("after the reseed %+v", got)
	}
}

func TestColoursAreHexOrNothing(t *testing.T) {
	st := open(t)
	inbox, _ := st.Projects()
	id := inbox[0].ID
	if _, err := st.CreateLabel("Bug", "fff url(https://x/p.png)"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a label with CSS in its colour: %v", err)
	}
	l, err := st.CreateLabel("Bug", "#E74C3C")
	if err != nil || l.HexColor != "e74c3c" {
		t.Fatalf("label colour %+v %v", l, err)
	}
	if _, err := st.UpdateProject(id, ProjectPatch{HexColor: str("red")}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a project colour by name: %v", err)
	}
	p, _ := st.UpdateProject(id, ProjectPatch{HexColor: str("#ABC")})
	if p.HexColor != "abc" {
		t.Fatalf("project colour %q", p.HexColor)
	}
	ss := inbox[0].Statuses
	ss[0].HexColor = "123456; background: url(x)"
	if _, err := st.SetStatuses(id, ss, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a column colour with CSS: %v", err)
	}
}
