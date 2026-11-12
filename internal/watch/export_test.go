package watch

import "github.com/dev1antdev/remuda/internal/machines"

// RunUntil is the machine's loop until done closes, for the tests outside the package.
func (w *Watcher) RunUntil(mc *machines.Machine, done <-chan struct{}) { w.run(mc, done) }

// Streaming says whether the machine's event stream stands.
func (w *Watcher) Streaming(machineID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stateFor(machineID).streaming.Load()
}
