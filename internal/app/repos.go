package app

import (
	"fmt"

	"github.com/dev1antdev/remuda/internal/gitsync"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/undo"
	"github.com/dev1antdev/remuda/internal/watch"
)

// repositories is the Repositories place over the fleet: the main folders of every machine's workspaces as the review
// sweep read them, a pull held back while the watcher sees an agent at work or asking in the checkout or the undo's
// tracker a turn still ending there, and the secret check before a push.
func repositories(dir string, fleet *machines.Manager, watcher *watch.Watcher, reviews *review.Store, turns *undo.Tracker) *gitsync.Service {
	return &gitsync.Service{
		Fleet: func() []gitsync.Machine {
			var out []gitsync.Machine
			for _, mc := range fleet.List() {
				out = append(out, gitsync.Machine{Host: mc, Away: awayWords(mc)})
			}
			return out
		},
		Host: func(machineID string) (shell.Host, error) {
			mc, err := fleet.Get(machineID)
			if err != nil {
				return nil, err
			}
			if away := awayWords(mc); away != "" {
				return nil, fmt.Errorf("%s is away: %s", mc.Label(), away)
			}
			return mc, nil
		},
		Folders: func(machineID string) []string {
			snap, _ := watcher.Snapshot(machineID)
			if snap == nil {
				return nil
			}
			var folders []string
			seen := map[string]bool{}
			for _, folder := range snap.WorkspaceDirs() {
				if root := reviews.Root(machineID, folder); root != "" {
					folder = root
				}
				if !seen[folder] {
					seen[folder] = true
					folders = append(folders, folder)
				}
			}
			return folders
		},
		Busy: func(machineID, checkout string) (string, string) {
			snap, _ := watcher.Snapshot(machineID)
			style := rpath.Style{}
			if mc, err := fleet.Get(machineID); err == nil {
				style = mc.Paths()
			}
			return gitsync.AgentIn(snap, style, checkout, func(folder string) bool { return turns.Going(machineID, folder) })
		},
		Check: func(host shell.Host, folder string) (review.SecretScan, error) {
			return review.ScanWork(host, folder, true)
		},
		Scans: gitsync.OpenScans(dir),
	}
}

// awayWords is why a machine cannot be read now, "" while it is connected.
func awayWords(mc *machines.Machine) string {
	state, reason := mc.State()
	switch {
	case state == machines.StateConnected:
		return ""
	case reason != "":
		return reason
	}
	return state // connecting, before its first answer
}
