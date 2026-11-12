package app

import (
	"github.com/dev1antdev/remuda/internal/collisions"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/prstatus"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/said"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/watch"
)

// collisionRadar is the radar over every connected machine's workspaces: their repositories, uncommitted files and
// branches as the review sweep read them, their agents' pull requests, the pushes and their mutes.
func collisionRadar(dir string, fleet *machines.Manager, watcher *watch.Watcher, reviews *review.Store, sayings *said.Tracker, requests *prstatus.Tracker, pushes *push.Service, changed func()) *collisions.Service {
	return collisions.New(dir, collisions.Inputs{
		Workspaces: func() []collisions.Workspace {
			var out []collisions.Workspace
			for _, mc := range fleet.List() {
				if state, _ := mc.State(); state != machines.StateConnected {
					continue
				}
				snap, _ := watcher.Snapshot(mc.ID())
				if snap == nil {
					continue
				}
				id := mc.ID()
				out = append(out, collisions.WorkspacesOf(id, snap, collisions.Lookup{
					Repo: func(dir string) string { return collisions.RepoKey(id, reviews.Root(id, dir), reviews.Remote(id, dir)) },
					Changed: func(dir string) int {
						badge, _ := reviews.Badge(id, dir)
						return badge.Changed
					},
					Branch: func(dir string) string { return reviews.Branch(id, dir) },
					// a request named and still open as gh last read it, or not read yet
					OpenPR: func(paneID string) bool {
						if _, named := sayings.Link(id, paneID); !named {
							return false
						}
						status, read := requests.Status(id, paneID)
						return !read || status.State == "OPEN"
					},
				})...)
			}
			return out
		},
		Host: func(id string) (shell.Host, bool) {
			mc, err := fleet.Get(id)
			if err != nil {
				return nil, false
			}
			return mc, true
		},
		Changed: changed,
		Notify:  pushes.Notify,
		Muted:   pushes.Muted,
	})
}
