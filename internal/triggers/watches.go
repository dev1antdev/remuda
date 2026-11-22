package triggers

import (
	"log"
	"sort"
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
	s.rememberWatched(machine, all) // only what was kept: a pane that got no room is no pane the machine had
	return all
}

// watchedBefore cuts the set to MaxWatches: the person's rules first, the built-ins after them, and within either
// class the watches the last set kept before the rest. A watch that stands is never taken for a pane that just
// appeared, a rule just saved still takes its place from a built-in, and a pane that leaves a rule's folder for a
// moment comes back to it.
func (s *Service) watchedBefore(machine string, all []watch.Output) []watch.Output {
	s.mu.Lock()
	before := s.watched[machine]
	s.mu.Unlock()
	rank := func(o watch.Output) int {
		stood := before[watchKey(o)]
		switch {
		case o.Person && stood:
			return 0
		case o.Person:
			return 1
		case stood:
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

// rememberWatched keeps the watches the set kept, so the next one knows which of them stand.
func (s *Service) rememberWatched(machine string, kept []watch.Output) {
	watches := make(map[string]bool, len(kept))
	for _, o := range kept {
		watches[watchKey(o)] = true
	}
	s.mu.Lock()
	s.watched[machine] = watches
	s.mu.Unlock()
}

// watchKey names one watch: a pane's watch of one key.
func watchKey(o watch.Output) string { return o.Pane + "|" + o.Key }
