package undo

import (
	"strings"
	"testing"
)

func TestUndoRevertsAMergeTheTurnMade(t *testing.T) {
	dir := repo(t, map[string]string{"a.txt": "one\n"})
	git(t, dir, "checkout", "-qb", "side")
	write(t, dir, "side.txt", "from the side\n")
	git(t, dir, "add", "side.txt")
	git(t, dir, "commit", "-qm", "side work")
	git(t, dir, "checkout", "-q", "main")
	f := newFixture(t)
	turn := f.turn(t, dir, func() { git(t, dir, "merge", "-q", "--no-ff", "-m", "merge side", "side") })
	if len(turn.Commits) != 1 || !turn.Commits[0].Merge {
		t.Fatalf("commits %+v: the first-parent line holds the merge alone", turn.Commits)
	}
	if _, err := f.service.Undo("local", turn.ID, true); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if got := readFile(t, dir, "side.txt"); got != "<gone>" {
		t.Errorf("side.txt is %q after the merge's revert", got)
	}
	if got := status(t, dir); got != "" {
		t.Errorf("status %q", got)
	}
}

func TestUndoRevertsTheTurnsCommits(t *testing.T) {
	dir := repo(t, map[string]string{"a.txt": "one\n", "b.txt": "bee\n"})
	write(t, dir, "b.txt", "bee, mine\n") // the person's, uncommitted, which the agent will commit
	f := newFixture(t)
	first := git(t, dir, "rev-parse", "HEAD")
	turn := f.turn(t, dir, func() {
		write(t, dir, "a.txt", "one\nagent\n")
		git(t, dir, "commit", "-qam", "the agent's work")
		write(t, dir, "c.txt", "after the commit\n")
		write(t, dir, "a.txt", "one\nagent\nmore\n")
	})
	if len(turn.Commits) != 1 || turn.Commits[0].Subject != "the agent's work" {
		t.Fatalf("commits %+v", turn.Commits)
	}
	plan, err := f.service.Plan("local", turn.ID)
	if err != nil || plan.Refused != nil || plan.RevertWhy != "" {
		t.Fatalf("plan: %+v %q %v", plan.Refused, plan.RevertWhy, err)
	}
	if len(plan.LeftAsIs) != 1 || plan.LeftAsIs[0] != "b.txt" {
		t.Errorf("without the reverts b.txt cannot be told apart from the person's: %v", plan.LeftAsIs)
	}
	result, err := f.service.Undo("local", turn.ID, true)
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if len(result.Reverted) != 1 {
		t.Fatalf("reverted %v", result.Reverted)
	}
	log := git(t, dir, "log", "--format=%s")
	if !strings.HasPrefix(log, `Revert "the agent's work"`) || !strings.Contains(log, "the agent's work\nfirst") {
		t.Errorf("history: %q — a revert on top, nothing lost", log)
	}
	if git(t, dir, "rev-parse", "HEAD~2") != first {
		t.Error("the revert did not land on top of the turn's commit")
	}
	if got := readFile(t, dir, "a.txt"); got != "one\n" {
		t.Errorf("a.txt is %q", got)
	}
	if got := readFile(t, dir, "b.txt"); got != "bee, mine\n" {
		t.Errorf("b.txt is %q: the person's uncommitted work comes back uncommitted", got)
	}
	if got := readFile(t, dir, "c.txt"); got != "<gone>" {
		t.Errorf("c.txt is %q", got)
	}
	if got := status(t, dir); got != "M b.txt" {
		t.Errorf("status %q", got)
	}
}

func TestUndoWithoutTheRevertsKeepsTheCommits(t *testing.T) {
	dir := repo(t, map[string]string{"a.txt": "one\n", "b.txt": "bee\n"})
	f := newFixture(t)
	turn := f.turn(t, dir, func() {
		write(t, dir, "a.txt", "one\nagent\n")
		git(t, dir, "commit", "-qam", "the agent's work")
		write(t, dir, "a.txt", "one\nagent\nuncommitted\n")
		write(t, dir, "b.txt", "bee\nagent\n")
	})
	head := git(t, dir, "rev-parse", "HEAD")
	if _, err := f.service.Undo("local", turn.ID, false); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if git(t, dir, "rev-parse", "HEAD") != head {
		t.Error("an undo without the reverts moved HEAD")
	}
	if got := readFile(t, dir, "a.txt"); got != "one\nagent\n" {
		t.Errorf("a.txt is %q: the commit stays, the uncommitted part goes", got)
	}
	if got := status(t, dir); got != "" {
		t.Errorf("status %q", got)
	}
}

func TestRevertIsRefusedWithSomethingElseStaged(t *testing.T) {
	dir := repo(t, map[string]string{"a.txt": "one\n", "mine.txt": "x\n"})
	f := newFixture(t)
	turn := f.turn(t, dir, func() {
		write(t, dir, "a.txt", "agent\n")
		git(t, dir, "commit", "-qam", "agent")
	})
	write(t, dir, "mine.txt", "staged by the person\n")
	git(t, dir, "add", "mine.txt")
	plan, _ := f.service.Plan("local", turn.ID)
	if plan.Refused != nil || !strings.Contains(plan.RevertWhy, "mine.txt") {
		t.Fatalf("the staged file does not hold the revert back: %+v %q", plan.Refused, plan.RevertWhy)
	}
	if _, err := f.service.Undo("local", turn.ID, true); err == nil {
		t.Fatal("the revert went through over the person's staged work")
	}
}

func TestPlanSaysTheCommitsWerePushed(t *testing.T) {
	dir := repo(t, map[string]string{"a.txt": "one\n"})
	f := newFixture(t)
	turn := f.turn(t, dir, func() {
		write(t, dir, "a.txt", "agent\n")
		git(t, dir, "commit", "-qam", "agent")
	})
	git(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	plan, _ := f.service.Plan("local", turn.ID)
	if strings.Join(plan.Pushed, ",") != "origin/main" {
		t.Fatalf("pushed %v", plan.Pushed)
	}
	result, err := f.service.Undo("local", turn.ID, true)
	if err != nil || len(result.Reverted) != 1 || strings.Join(result.Pushed, ",") != "origin/main" {
		t.Fatalf("undo %+v %v", result, err)
	}
}
