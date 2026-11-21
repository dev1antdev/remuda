package app

import (
	"testing"

	"github.com/dev1antdev/remuda/internal/gitops"
	"github.com/dev1antdev/remuda/internal/gitstage"
	"github.com/dev1antdev/remuda/internal/gitsync"
	"github.com/dev1antdev/remuda/internal/undo"
)

// A checkout write waits while herdr's server still runs on that machine — its agents are not the ones this remuda
// sees — and meets them, or nothing, once it is stopped there. The move is pressed machine by machine.
func TestCheckoutWritesWaitWhileHerdrRunsThere(t *testing.T) {
	ops, stage, repos := &gitops.Service{}, &gitstage.Service{}, &gitsync.Service{}
	turns := &undo.Service{}
	ops.Busy = func(machineID, top string, worktrees []string) string { return "claude works there" }
	stage.Busy = func(machineID, dir, top string) string { return "claude works there" }
	repos.Busy = func(machineID, checkout string) (string, string) { return "claude", "working" }
	turns.Working = func(machineID, dir string) string { return "claude works there" }
	holdCheckoutWrites(ops, stage, repos, turns, func(machineID string) string {
		if machineID == "local" {
			return herdrUnseen
		}
		return ""
	})

	if got := ops.Busy("local", "/w/app", nil); got != herdrUnseen {
		t.Errorf("git ops on a machine with herdr = %q", got)
	}
	if got := ops.Busy("gog", "/w/app", nil); got != "claude works there" {
		t.Errorf("git ops on a machine without herdr = %q", got)
	}
	if got := stage.Busy("local", "/w/app", "/w/app"); got != herdrUnseen {
		t.Errorf("a stage on a machine with herdr = %q", got)
	}
	if agent, doing := repos.Busy("local", "/w/app"); agent != herdrUnseen || doing != "working" {
		t.Errorf("a pull on a machine with herdr = %q %q", agent, doing)
	}
	if got := turns.Working("local", "/w/app"); got != herdrUnseen {
		t.Errorf("an undo on a machine with herdr = %q", got)
	}
	if got := turns.Working("gog", "/w/app"); got != "claude works there" {
		t.Errorf("an undo on a machine without herdr = %q", got)
	}
}

// Without a reason of their own, the writes pass once herdr is stopped, and nothing is made up.
func TestCheckoutWritesPassWithNothingToWaitFor(t *testing.T) {
	ops, stage, repos := &gitops.Service{}, &gitstage.Service{}, &gitsync.Service{}
	turns := &undo.Service{}
	holdCheckoutWrites(ops, stage, repos, turns, func(string) string { return "" })
	if got := ops.Busy("local", "/w", nil); got != "" {
		t.Errorf("git ops = %q", got)
	}
	if got := stage.Busy("local", "/w", "/w"); got != "" {
		t.Errorf("stage = %q", got)
	}
	if agent, doing := repos.Busy("local", "/w"); agent != "" || doing != "" {
		t.Errorf("pull = %q %q", agent, doing)
	}
	if got := turns.Working("local", "/w"); got != "" {
		t.Errorf("undo = %q", got)
	}
}

// A check that cannot tell holds the write; herdr's own word that its server is stopped lifts it. The check runs at the
// moment of the write, on the endpoint the stop uses, so nothing is remembered past it.
func TestTheHerdrCheckIsAskedAtEveryWrite(t *testing.T) {
	ops, stage, repos := &gitops.Service{}, &gitstage.Service{}, &gitsync.Service{}
	turns := &undo.Service{}
	asked := 0
	holdCheckoutWrites(ops, stage, repos, turns, func(machineID string) string {
		asked++
		return herdrUnseen
	})
	ops.Busy("local", "/w", nil)
	stage.Busy("local", "/w", "/w")
	repos.Busy("local", "/w")
	turns.Working("local", "/w")
	if asked != 4 {
		t.Fatalf("the machine was asked %d times for four writes", asked)
	}
}
