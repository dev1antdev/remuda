package transcript

import (
	"log"
	"sync"

	"github.com/dev1antdev/remuda/internal/state"
)

// KeptRoots is what a machine's reader carries from one run to the next: the folders its agents named for themselves,
// and the file each conversation was read in.
type KeptRoots struct {
	Roots    map[Format][]string `json:"roots,omitempty"`
	Sessions map[string]string   `json:"sessions,omitempty"`
}

// RootsFile is where those are kept, in remuda's state dir.
const RootsFile = "transcript-roots.json"

// RootStore keeps every machine's KeptRoots in one file of remuda's state dir: the folders and conversations a reader
// found, so the ones an agent left behind are still read after that agent, or remuda, stops.
type RootStore struct {
	dir string
	mu  sync.Mutex
	all map[string]KeptRoots
}

// OpenRootStore reads what was kept; a damaged file keeps nothing, said once.
func OpenRootStore(dir string) *RootStore {
	s := &RootStore{dir: dir, all: map[string]KeptRoots{}}
	if dir == "" {
		return s
	}
	if err := state.LoadJSON(dir, RootsFile, &s.all); err != nil {
		log.Printf("%s is unreadable (%v): the folders and sessions its agents named are learned again as they run", RootsFile, err)
		s.all = map[string]KeptRoots{}
	}
	return s
}

// Loaded is one machine's kept folders and conversations.
func (s *RootStore) Loaded(machine string) KeptRoots {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyKept(s.all[machine])
}

// Keep saves one machine's set.
func (s *RootStore) Keep(machine string, kept KeptRoots) {
	if s.dir == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.all[machine] = copyKept(kept)
	if err := state.SaveJSON(s.dir, RootsFile, s.all); err != nil {
		log.Printf("%s: %v", RootsFile, err)
	}
}

// copyKept is a set of its own, so a reader and the file never share a map.
func copyKept(kept KeptRoots) KeptRoots {
	out := KeptRoots{Roots: make(map[Format][]string, len(kept.Roots)), Sessions: make(map[string]string, len(kept.Sessions))}
	for f, dirs := range kept.Roots {
		out.Roots[f] = append([]string(nil), dirs...)
	}
	for id, path := range kept.Sessions {
		out.Sessions[id] = path
	}
	return out
}
