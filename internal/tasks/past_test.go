package tasks

import (
	"testing"
	"time"
)

func TestRunsSinceListsTheRunsOfThePeriodWithTheirTasks(t *testing.T) {
	st := open(t)
	clock := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	st.now = func() time.Time { return clock }
	inbox, _ := st.Projects()
	old, _ := st.Create(inbox[0].ID, Patch{Title: str("Old work")})
	if _, err := st.StartRun(Run{TaskID: old.ID, Machine: "studio", Workspace: "w1", Pane: "w1:p1", Dir: "/w/old", Agent: "claude"}); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(48 * time.Hour)
	fresh, _ := st.Create(inbox[0].ID, Patch{Title: str("Fresh work")})
	run, err := st.StartRun(Run{TaskID: fresh.ID, Machine: "studio", Workspace: "w2", Pane: "w2:p1", Dir: "/w/fresh", Agent: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetRunUsage(run.ID, 1.5, 900, "gpt-6.1-sol", 600); err != nil {
		t.Fatal(err)
	}

	got, err := st.RunsSince(clock.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TaskID != fresh.ID || got[0].Title != "Fresh work" || got[0].ProjectID != inbox[0].ID || got[0].Model != "gpt-6.1-sol" || got[0].Took != 600 || got[0].Identifier() != "#2" {
		t.Fatalf("runs since: %+v", got)
	}
	if all, _ := st.RunsSince(time.Time{}); len(all) != 2 || all[0].TaskID != old.ID {
		t.Fatalf("every run, the oldest first: %+v", all)
	}
	if err := st.Delete(fresh.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.RunsSince(clock.Add(-time.Hour)); len(got) != 0 {
		t.Fatalf("a deleted task's run is still listed: %+v", got)
	}
}

func TestAMergeIsKeptOnTheRunAndReadBackFromTheComment(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	st.now = func() time.Time { return clock }
	inbox, _ := st.Projects()
	tk, _ := st.Create(inbox[0].ID, Patch{Title: str("Ship it")})
	run, _ := st.StartRun(Run{TaskID: tk.ID, Machine: "studio", Workspace: "w1", Pane: "w1:p1", Dir: "/w/x", Agent: "claude"})
	if _, err := st.OnPullRequest("studio", "w1:p1", "https://github.com/o/r/pull/15", "PR #15"); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(time.Hour)
	if _, err := st.OnPullRequestMerged("studio", "w1:p1", "https://github.com/o/r/pull/15"); err != nil {
		t.Fatal(err)
	}
	if r, _ := st.Run(run.ID); r.Merged != "2026-11-02T10:00:00Z" {
		t.Fatalf("the merge was not kept on the run: %q", r.Merged)
	}

	// a run from before the column: its merge is only the comment remuda wrote
	if _, err := st.db.Exec(`UPDATE task_runs SET merged = '' WHERE id = ?`, run.ID); err != nil {
		t.Fatal(err)
	}
	other, _ := st.Create(inbox[0].ID, Patch{Title: str("Not merged")})
	open, _ := st.StartRun(Run{TaskID: other.ID, Machine: "studio", Workspace: "w2", Pane: "w2:p1", Dir: "/w/y", Agent: "claude"})
	if _, err := st.OnPullRequest("studio", "w2:p1", "https://github.com/o/r/pull/16", "PR #16"); err != nil {
		t.Fatal(err)
	}
	st.Close()
	st, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if r, _ := st.Run(run.ID); r.Merged != "2026-11-02T10:00:00Z" {
		t.Fatalf("the merge was not read back from the comment: %q", r.Merged)
	}
	if r, _ := st.Run(open.ID); r.Merged != "" {
		t.Fatalf("a request never merged reads as merged: %q", r.Merged)
	}
}
