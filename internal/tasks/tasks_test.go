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
