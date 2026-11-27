// Package checkout is one lock per checkout, shared by every service that writes into it: two presses — a merge and a
// checkout, a commit and an undo — run one after another, so the state each checked first is still true when it writes.

package checkout

import "sync"

var (
	mu    sync.Mutex
	locks = map[string]*sync.Mutex{} // machine|top → its lock
)

func mutex(machineID, top string) *sync.Mutex {
	key := machineID + "|" + top
	mu.Lock()
	defer mu.Unlock()
	m := locks[key]
	if m == nil {
		m = &sync.Mutex{}
		locks[key] = m
	}
	return m
}

// TryLock takes the checkout's lock without waiting: the answer says whether it was free, and release is nil otherwise.
func TryLock(machineID, top string) (release func(), ok bool) {
	m := mutex(machineID, top)
	if !m.TryLock() {
		return nil, false
	}
	return m.Unlock, true
}

// Lock waits for the checkout's lock and answers its release.
func Lock(machineID, top string) (release func()) {
	m := mutex(machineID, top)
	m.Lock()
	return m.Unlock
}
