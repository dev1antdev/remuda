package scorecard

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/arena"
	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/tasks"
)

// TestTheCardReadsTheCockpitsOwnStores runs the scorecard over a real task list, review store and arena store.
func TestTheCardReadsTheCockpitsOwnStores(t *testing.T) {
	taskStore, err := tasks.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer taskStore.Close()
	projects, _ := taskStore.Projects()
	title := "Fix the login page"
	task, _ := taskStore.Create(projects[0].ID, tasks.Patch{Title: &title})
	run, err := taskStore.StartRun(tasks.Run{TaskID: task.ID, Machine: "studio", Workspace: "w1", Pane: "w1:p1", Dir: "/w/task-1", Agent: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	_ = taskStore.SetRun(run.ID, tasks.RunFinished, "rec-1")
	_ = taskStore.SetRunUsage(run.ID, 3.5, 12000, "Opus 4.5", 900)

	reviews := review.New(t.TempDir())
	host := &shell.Fake{Name: "studio", Answer: func(string) (string, error) { return "0123456789abcdef0123456789abcdef01234567\n\x1e\n", nil }}
	if _, err := reviews.MarkSeen(host, "/w/task-1", review.VerdictApproved); err != nil {
		t.Fatal(err)
	}

	arenas := arena.Open(t.TempDir())
	a, _ := arenas.Create("x", "x", 0, []arena.LaneRequest{{Machine: "studio", Workspace: "w1", Agent: "claude"}, {Machine: "studio", Workspace: "w1", Agent: "codex", Model: "gpt-6.1-sol"}}, time.Now())
	_ = arenas.SetLane(a.ID, 1, func(l *arena.Lane) { l.Pane = "w5:p1" })
	_ = arenas.SetLane(a.ID, 2, func(l *arena.Lane) { l.Pane = "w6:p1" })
	decided, _, _ := arenas.Update(a.ID, func(x *arena.Arena) { x.Kept = 2 })
	if err := arenas.Decide(decided, map[int]string{1: "Opus 4.5"}, time.Now()); err != nil {
		t.Fatal(err)
	}

	card, err := (&Service{Tasks: taskStore, Review: reviews, Arenas: arenas}).Card(30, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	opus := rowOf(t, card, "claude", "Opus 4.5")
	if opus.Runs != 1 || opus.Finished.Count != 1 || opus.FirstApproved.Count != 1 || *opus.CostPerRun.Value != 3.5 || *opus.MinutesPerRun.Value != 15 || opus.Kept.Of != 1 || opus.Kept.Count != 0 {
		t.Fatalf("opus: %+v", opus)
	}
	gpt := rowOf(t, card, "codex", "gpt-6.1-sol")
	if gpt.Kept.Count != 1 || gpt.Try == nil || gpt.Try.Title != title || gpt.Try.Lanes[0].Model != "Opus 4.5" {
		t.Fatalf("gpt: %+v", gpt)
	}
	if card.Days != 30 || card.Verdicts != 1 || card.Unplaced != 0 {
		t.Fatalf("card: %+v", card)
	}
}
