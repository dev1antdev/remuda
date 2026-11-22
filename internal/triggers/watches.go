package triggers

import (
	"log"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/watch"
)

// MaxWatches is how many output watches one machine's stream holds: every pane gets the two built-in ones and the
// person's rules that cover it, the person's first; past the cap the rest are left out, said once an hour.
const MaxWatches = 200

var (
	overMu   sync.Mutex
	overSaid = map[string]time.Time{}
)

// Watches is the output watches for a machine's snapshot: the person's rules where they look, then the built-in two
// on every pane, at most MaxWatches.
func (s *Service) Watches(machine string, snap *herdr.Snapshot) []watch.Output {
	rules := s.Rules.List()
	var person, builtin []watch.Output
	for i := range snap.Panes {
		pane := &snap.Panes[i]
		dir := ""
		if ws := snap.Workspace(pane.WorkspaceID); ws != nil {
			dir = snap.WorkspaceDir(ws)
		}
		for _, r := range rules {
			if r.Covers(machine, dir, pane.Dir()) {
				person = append(person, watch.Output{Key: rulePrefix + r.ID, Pane: pane.PaneID, Match: herdr.OutputMatch{Type: r.Match, Value: r.Pattern}, Lines: lines, Person: true})
			}
		}
		builtin = append(builtin,
			watch.Output{Key: WatchServe, Pane: pane.PaneID, Match: herdr.OutputMatch{Type: "regex", Value: ServePattern}, Lines: lines},
			watch.Output{Key: WatchTests, Pane: pane.PaneID, Match: herdr.OutputMatch{Type: "regex", Value: TestsPattern}, Lines: lines})
	}
	all := append(person, builtin...)
	if len(all) > MaxWatches {
		overMu.Lock()
		if time.Since(overSaid[machine]) > time.Hour {
			overSaid[machine] = time.Now()
			log.Printf("%s: %d output watches asked for, %d kept", machine, len(all), MaxWatches)
		}
		overMu.Unlock()
		all = all[:MaxWatches]
	}
	return all
}
