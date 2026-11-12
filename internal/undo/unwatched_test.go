package undo

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestTrackerLeavesATurnItDidNotSeeBegin(t *testing.T) {
	dir := repo(t, map[string]string{"a.txt": "one\n"})
	tracker, store := newTracker(t)
	at := time.Now()
	tracker.Observe("local", snapshot(dir, agentPane("p1", herdr.StatusWorking)), at, true)
	tracker.Observe("local", snapshot(dir, agentPane("p1", herdr.StatusWorking)), at.Add(time.Second), false)
	if tracker.Going("local", dir) {
		t.Fatal("a turn already under way when the machine was first seen is watched as if from its start")
	}
	write(t, dir, "a.txt", "before remuda looked\n")
	steps := []struct {
		after  time.Duration
		status string
	}{{2 * time.Second, herdr.StatusDone}, {10 * time.Second, herdr.StatusWorking}, {20 * time.Second, herdr.StatusDone}, {20*time.Second + Settle, herdr.StatusDone}, {20*time.Second + Grace, herdr.StatusDone}}
	for i, step := range steps {
		if i == 2 {
			write(t, dir, "b.txt", "the next turn\n")
		}
		tracker.Observe("local", snapshot(dir, agentPane("p1", step.status)), at.Add(step.after), false)
		idle(t, tracker)
	}
	turns := store.List("local", dir)
	if len(turns) != 1 || len(turns[0].Files) != 1 || turns[0].Files[0].Path != "b.txt" {
		t.Fatalf("turns %+v", turns)
	}
}
