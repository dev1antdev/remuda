package app

import (
	"github.com/dev1antdev/remuda/internal/gitops"
	"github.com/dev1antdev/remuda/internal/gitstage"
	"github.com/dev1antdev/remuda/internal/gitsync"
	"github.com/dev1antdev/remuda/internal/undo"
)

// herdrUnseen is who a checkout write waits for while herdr's server still runs on the machine: its agents there are
// not the ones this remuda sees, so a write could land under their hands.
const herdrUnseen = "an agent of herdr's (its server still runs there, and this remuda does not see its agents)"

// holdCheckoutWrites makes every git write on a checkout — the Git lens, the Changes lens, a pull, an undo, a worktree
// removed — wait where herdr's server still runs on the machine, as while an agent works there: the move off herdr is
// pressed machine by machine (Stop herdr's server), and until then herdr's agents may be in that checkout. Once the
// server is stopped there, the reason is gone with it, and the write passes or meets the agents this remuda does see.
func holdCheckoutWrites(ops *gitops.Service, stage *gitstage.Service, repos *gitsync.Service, turns *undo.Service, herdr func(machineID string) string) {
	busy := ops.Busy
	ops.Busy = func(machineID, top string, worktrees []string) string {
		if reason := herdr(machineID); reason != "" {
			return reason
		}
		if busy != nil {
			return busy(machineID, top, worktrees)
		}
		return ""
	}
	stageBusy := stage.Busy
	stage.Advisory = stageBusy // a reader keeps the agents this remuda sees, never the probe
	stage.Busy = func(machineID, dir, top string) string {
		if reason := herdr(machineID); reason != "" {
			return reason
		}
		if stageBusy != nil {
			return stageBusy(machineID, dir, top)
		}
		return ""
	}
	repoBusy := repos.Busy
	repos.Busy = func(machineID, checkout string) (string, string) {
		if reason := herdr(machineID); reason != "" {
			return reason, "working"
		}
		if repoBusy != nil {
			return repoBusy(machineID, checkout)
		}
		return "", ""
	}
	working := turns.Working
	turns.Working = func(machineID, dir string) string {
		if reason := herdr(machineID); reason != "" {
			return reason
		}
		if working != nil {
			return working(machineID, dir)
		}
		return ""
	}
}
