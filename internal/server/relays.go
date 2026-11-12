package server

import "sync"

// relays counts the terminals this remuda pipes to a machine's own remuda, by machine|pane: the attach is the peer's,
// so the machine's attach manager here counts none of their viewers.
type relays struct {
	mu    sync.Mutex
	count map[string]int
}

// hold notes one viewer relayed to the pane, until the release it answers.
func (r *relays) hold(machineID, paneID string) (release func()) {
	key := machineID + "|" + paneID
	r.mu.Lock()
	if r.count == nil {
		r.count = map[string]int{}
	}
	r.count[key]++
	r.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			if r.count[key]--; r.count[key] <= 0 {
				delete(r.count, key)
			}
		})
	}
}

// Relaying says whether a viewer of this remuda has the pane's terminal open through the machine's own remuda now.
func (s *Server) Relaying(machineID, paneID string) bool {
	s.relayed.mu.Lock()
	defer s.relayed.mu.Unlock()
	return s.relayed.count[machineID+"|"+paneID] > 0
}
