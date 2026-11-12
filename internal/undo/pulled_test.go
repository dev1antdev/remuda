package undo

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestATurnThatPulledOthersCommitsIsNotReverted(t *testing.T) {
	dir := repo(t, map[string]string{"a.txt": "one\n"})
	git(t, dir, "checkout", "-qb", "upstream")
	write(t, dir, "theirs.txt", "a teammate's\n")
	git(t, dir, "add", "theirs.txt")
	earlier := exec.Command("git", "commit", "-qm", "a teammate's commit")
	earlier.Dir, earlier.Env = dir, append(os.Environ(), "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z")
	if out, err := earlier.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	git(t, dir, "checkout", "-q", "main")
	f := newFixture(t)
	turn := f.turn(t, dir, func() {
		git(t, dir, "merge", "-q", "--ff-only", "upstream")
		write(t, dir, "a.txt", "agent\n")
	})
	if !strings.Contains(turn.Why, "brought in commits made before it began") || len(turn.Commits) != 0 {
		t.Fatalf("a fast-forward of a teammate's commit is taken for the turn's own: %q %+v", turn.Why, turn.Commits)
	}
	if plan, _ := f.service.Plan("local", turn.ID); plan.Refused == nil {
		t.Fatal("such a turn is offered for an undo")
	}
}
