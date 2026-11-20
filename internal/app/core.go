package app

import (
	"github.com/dev1antdev/remuda/internal/gitops"
	"github.com/dev1antdev/remuda/internal/gitstage"
	"github.com/dev1antdev/remuda/internal/gitsync"
	"github.com/dev1antdev/remuda/internal/undo"
)

// herdrUnseen is who a remuda on remuda-core (-core) holds every checkout write back for: herdr's agents on the same
// machine may be working there, and it does not see them.
const herdrUnseen = "an agent of herdr's (a remuda on remuda-core sees none)"

// holdCheckoutWrites makes every git write on a checkout — the Git lens, the Changes lens, a pull, an undo, a worktree
// removed — refuse, as while an agent works there, for a remuda started with -core beside the real cockpit.
func holdCheckoutWrites(ops *gitops.Service, stage *gitstage.Service, repos *gitsync.Service, turns *undo.Service) {
	ops.Busy = func(string, string, []string) string { return herdrUnseen }
	stage.Busy = func(string, string, string) string { return herdrUnseen }
	repos.Busy = func(string, string) (string, string) { return herdrUnseen, "working" }
	turns.Working = func(string, string) string { return herdrUnseen }
}
