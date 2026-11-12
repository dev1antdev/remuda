package arena

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/state"
	"github.com/dev1antdev/remuda/internal/tasks"
)

func TestADecidedArenaKeepsItsLanesInTheHistory(t *testing.T) {
	dir := t.TempDir()
	st := Open(dir)
	a, err := st.Create("x", "x", 7, []LaneRequest{{Machine: "local", Workspace: "w1", Agent: "claude", Model: "opus"}, {Machine: "local", Workspace: "w1", Agent: "codex"}, {Machine: "local", Workspace: "w1", Agent: "gemini"}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_ = st.SetLane(a.ID, 1, func(l *Lane) { l.Dir, l.Pane, l.State, l.RunID = "/tmp/arena-1-1", "w2:p1", LaneRunning, 41 })
	_ = st.SetLane(a.ID, 2, func(l *Lane) { l.Dir, l.Pane, l.State = "/tmp/arena-1-2", "w3:p1", LaneRunning })
	_ = st.SetLane(a.ID, 3, func(l *Lane) { l.State, l.Note = LaneFailed, "no worktree" }) // never ran an agent
	taskStore, err := tasks.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer taskStore.Close()
	s := &Service{Store: st, Tasks: taskStore, ModelOf: func(machine, pane string) string {
		if pane == "w2:p1" {
			return "Opus 4.5"
		}
		return ""
	}}
	if _, err := s.Keep(a.ID, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Keep(a.ID, 2); err != nil { // keeping again retries the closes and notes nothing twice
		t.Fatal(err)
	}
	got := st.Outcomes(time.Time{})
	if len(got) != 2 {
		t.Fatalf("outcomes: %+v", got)
	}
	if got[0].Lane != 1 || got[0].Model != "Opus 4.5" || got[0].Kept || got[0].RunID != 41 || got[0].TaskID != 7 {
		t.Fatalf("the lane not kept: %+v", got[0])
	}
	if got[1].Lane != 2 || got[1].Agent != "codex" || got[1].Model != "" || !got[1].Kept {
		t.Fatalf("the lane kept: %+v", got[1])
	}
	if err := st.Forget(a.ID); err != nil {
		t.Fatal(err)
	}
	if after := Open(dir).Outcomes(time.Time{}); len(after) != 2 {
		t.Fatalf("forgetting the arena lost its lanes: %+v", after)
	}
	if late := st.Outcomes(time.Now().Add(time.Hour)); len(late) != 0 {
		t.Fatalf("lanes before since: %+v", late)
	}
}

func TestArenasDecidedBeforeTheHistoryAreReadIntoIt(t *testing.T) {
	dir := t.TempDir()
	created := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	old := file{Next: 3, Arenas: []Arena{
		{ID: 1, Title: "a", Created: created, Kept: 2, Lanes: []Lane{{ID: 1, Agent: "claude", Model: "sonnet", Pane: "w1:p1"}, {ID: 2, Agent: "codex", Pane: "w2:p1"}}},
		{ID: 2, Title: "b", Created: created, Lanes: []Lane{{ID: 1, Agent: "claude", Pane: "w3:p1"}}}, // undecided
	}}
	if err := state.SaveJSON(dir, fileName, old); err != nil {
		t.Fatal(err)
	}
	got := Open(dir).Outcomes(time.Time{})
	if len(got) != 2 || got[0].Model != "sonnet" || got[0].Kept || !got[1].Kept || !got[0].At.Equal(created) {
		t.Fatalf("backfilled: %+v", got)
	}
	if again := Open(dir).Outcomes(time.Time{}); len(again) != 2 {
		t.Fatalf("read in twice: %+v", again)
	}
}

func TestTheHistoryHoldsTheNewestLanes(t *testing.T) {
	st := Open(t.TempDir())
	for i := range HistoryKeep/2 + 3 {
		st.addOutcomes([]Outcome{{Arena: i + 1, Lane: 1}, {Arena: i + 1, Lane: 2}})
	}
	if len(st.history) != HistoryKeep || st.history[0].Arena != 4 {
		t.Fatalf("kept %d, the oldest of arena %d", len(st.history), st.history[0].Arena)
	}
}
