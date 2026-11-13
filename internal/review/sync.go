package review

import (
	"strconv"
	"strings"
)

// Sync is how far a checkout's branch stands from its upstream as of the last fetch: the commits it has that the
// upstream lacks, and the ones the upstream has that it lacks. Nothing is fetched to know it.
type Sync struct {
	Ahead  int `json:"ahead"`
	Behind int `json:"behind"`
}

// syncScript prints, inside a checkout whose branch has an upstream, git's count of the commits on either side of it,
// "2\t1"; an empty line for a detached HEAD or a branch with no upstream.
const syncScript = `s=$(git --no-optional-locks -c core.fsmonitor=false rev-list --left-right --count 'HEAD...@{u}' 2>/dev/null); printf '%s\n' "$s"`

// parseSync reads syncScript's line; false for none.
func parseSync(line string) (Sync, bool) {
	ahead, behind, ok := strings.Cut(strings.TrimSpace(line), "\t")
	if !ok {
		return Sync{}, false
	}
	a, errA := strconv.Atoi(strings.TrimSpace(ahead))
	b, errB := strconv.Atoi(strings.TrimSpace(behind))
	if errA != nil || errB != nil || a < 0 || b < 0 {
		return Sync{}, false
	}
	return Sync{Ahead: a, Behind: b}, true
}

// Sync is the folder's branch against its upstream as the last sweep read it; false when it has none, or before the
// first sweep.
func (store *Store) Sync(machineID, dir string) (Sync, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	sync, ok := store.syncs[key(machineID, dir)]
	return sync, ok
}

func (store *Store) setSync(storeKey, line string) {
	sync, ok := parseSync(line)
	store.mu.Lock()
	defer store.mu.Unlock()
	if !ok {
		delete(store.syncs, storeKey)
		return
	}
	store.syncs[storeKey] = sync
}
