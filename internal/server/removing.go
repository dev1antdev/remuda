package server

import (
	"strings"
	"sync"

	"github.com/dev1antdev/remuda/internal/rpath"
)

// removing holds the folders whose worktrees a removal is taking right now, by machine: a new workspace on one of them
// is refused until the removal ends, so it cannot open on a folder git is about to take away.
type removing struct {
	mu   sync.Mutex
	dirs map[string]int
}

func removingKey(machine, dir string) string {
	return machine + "\x1f" + rpath.For(dir).Clean(dir)
}

// begin marks the folder as being removed and returns the call that ends the mark.
func (r *removing) begin(machine, dir string) func() {
	key := removingKey(machine, dir)
	r.mu.Lock()
	if r.dirs == nil {
		r.dirs = map[string]int{}
	}
	r.dirs[key]++
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		if r.dirs[key]--; r.dirs[key] <= 0 {
			delete(r.dirs, key)
		}
		r.mu.Unlock()
	}
}

// under says whether a new workspace at dir would stand on a folder being removed, or inside it.
func (r *removing) under(machine, dir string) bool {
	if dir == "" {
		return false
	}
	style := rpath.For(dir)
	r.mu.Lock()
	defer r.mu.Unlock()
	for key := range r.dirs {
		m, held, ok := strings.Cut(key, "\x1f")
		if ok && m == machine && (style.Equal(held, dir) || style.Within(held, dir)) {
			return true
		}
	}
	return false
}
