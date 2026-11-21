package app

import (
	"testing"

	"github.com/dev1antdev/remuda/internal/gitops"
	"github.com/dev1antdev/remuda/internal/gitstage"
	"github.com/dev1antdev/remuda/internal/gitsync"
	"github.com/dev1antdev/remuda/internal/undo"
)

// A remuda on remuda-core holds every checkout write back as while an agent works there: herdr's agents on the same
// machine may, unseen. Each service refuses a write while its hook names someone (their own tests).
func TestARemudaOnCoreHoldsEveryCheckoutWriteBack(t *testing.T) {
	ops, stage, repos, turns := &gitops.Service{}, &gitstage.Service{}, &gitsync.Service{}, &undo.Service{}
	holdCheckoutWrites(ops, stage, repos, turns)
	if ops.Busy("gog", "/repo", nil) == "" || stage.Busy("gog", "/repo", "/repo") == "" || turns.Working("gog", "/repo") == "" {
		t.Fatal("a checkout write goes through")
	}
	if agent, doing := repos.Busy("gog", "/repo"); agent == "" || doing == "" {
		t.Fatal("a pull goes through")
	}
}

// The herdr a host ran before the move: the one found, else a path the person gave that is gone since — kept, so its
// server's look cannot read as none running — else herdr's own lookup.
func TestTheMovedHerdrKeepsAPathTheFlagGave(t *testing.T) {
	if got := movedHerdrBin("/usr/local/bin/herdr", "herdr"); got != "/usr/local/bin/herdr" {
		t.Fatalf("found: %q", got)
	}
	if got := movedHerdrBin("", "/opt/old/herdr"); got != "/opt/old/herdr" {
		t.Fatalf("a path given and gone: %q", got)
	}
	if got := movedHerdrBin("", "herdr"); got != "" {
		t.Fatalf("no herdr anywhere: %q", got)
	}
}
