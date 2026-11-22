package triggers

import (
	"log"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/watch"
)

// MaxWatches is how many output watches one machine's stream holds: every pane gets the two built-in ones and the
// person's rules that cover it, the person's first; past the cap the panes watched the time before keep their watches
// and a pane just appeared gets what is left, so a momentary pane takes no watch of a pane that stands; the rest are
// left out, said once an hour.
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
		all = s.watchedBefore(machine, all)
	}
	s.rememberWatched(machine, snap)
	return all
}

// watchedBefore cuts the set to MaxWatches keeping first the watches of the panes the machine had the time before: a
// pane that just appeared is served with what is left, so it never displaces a watch of a pane that stands.
func (s *Service) watchedBefore(machine string, all []watch.Output) []watch.Output {
	s.mu.Lock()
	before := s.watched[machine]
	s.mu.Unlock()
	var kept, fresh []watch.Output
	for _, o := range all {
		if before[o.Pane] {
			kept = append(kept, o)
		} else {
			fresh = append(fresh, o)
		}
	}
	kept = append(kept, fresh...)
	if len(kept) > MaxWatches {
		kept = kept[:MaxWatches]
	}
	return kept
}

// rememberWatched keeps which panes the machine had, so the next set knows the ones that just appeared.
func (s *Service) rememberWatched(machine string, snap *herdr.Snapshot) {
	panes := make(map[string]bool, len(snap.Panes))
	for i := range snap.Panes {
		panes[snap.Panes[i].PaneID] = true
	}
	s.mu.Lock()
	s.watched[machine] = panes
	s.mu.Unlock()
}
