package triggers

import (
	"log"
	"sort"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/coreapi"
	"github.com/dev1antdev/remuda/internal/watch"
)

// MaxWatches is how many output watches one machine's stream holds: every pane gets the two built-in ones and the
// person's rules that cover it, the person's first; past the cap the person's rules come before the built-ins, and
// within either class the watches the stream subscribes to now before the ones it does not — so a pane that just
// appeared takes no watch of a pane that stands, and a rule just saved still takes its place from a built-in. The
// rest are left out, said once an hour.
const MaxWatches = 200

var (
	overMu   sync.Mutex
	overSaid = map[string]time.Time{}
)

// Watches is the output watches for a machine's snapshot: the person's rules where they look, then the built-in two
// on every pane, at most MaxWatches — standing being the watches the machine's stream subscribes to now.
func (s *Service) Watches(machine string, snap *coreapi.Snapshot, standing []watch.Output) []watch.Output {
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
				person = append(person, watch.Output{Key: rulePrefix + r.ID, Pane: pane.PaneID, Match: coreapi.OutputMatch{Type: r.Match, Value: r.Pattern}, Lines: lines, Person: true})
			}
		}
		builtin = append(builtin,
			watch.Output{Key: WatchServe, Pane: pane.PaneID, Match: coreapi.OutputMatch{Type: "regex", Value: ServePattern}, Lines: lines},
			watch.Output{Key: WatchTests, Pane: pane.PaneID, Match: coreapi.OutputMatch{Type: "regex", Value: TestsPattern}, Lines: lines})
	}
	all := append(person, builtin...)
	if len(all) > MaxWatches {
		overMu.Lock()
		if time.Since(overSaid[machine]) > time.Hour {
			overSaid[machine] = time.Now()
			log.Printf("%s: %d output watches asked for, %d kept", machine, len(all), MaxWatches)
		}
		overMu.Unlock()
		all = admitted(all, standing)
	}
	return all
}

// admitted cuts the set to MaxWatches: the person's rules before the built-ins, and within either class the watches
// the stream subscribes to now before the ones it does not.
func admitted(all, standing []watch.Output) []watch.Output {
	held := make(map[string]bool, len(standing))
	for _, o := range standing {
		held[watchKey(o)] = true
	}
	rank := func(o watch.Output) int {
		switch {
		case o.Person && held[watchKey(o)]:
			return 0
		case o.Person:
			return 1
		case held[watchKey(o)]:
			return 2
		}
		return 3
	}
	kept := append([]watch.Output(nil), all...)
	sort.SliceStable(kept, func(i, j int) bool { return rank(kept[i]) < rank(kept[j]) })
	if len(kept) > MaxWatches {
		kept = kept[:MaxWatches]
	}
	return kept
}

// watchKey names one watch: a pane's watch of one key.
func watchKey(o watch.Output) string { return o.Pane + "|" + o.Key }
