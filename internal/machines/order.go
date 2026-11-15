package machines

import (
	"fmt"

	"github.com/dev1antdev/remuda/internal/state"
)

// The person's order of the machines: the ids as they dragged them in the sidebar, kept in order.json. A machine the
// order does not name, one added since, comes after the ones it does, in the order it arrived.

const (
	orderFile = "order.json"
	maxPlaced = 256 // more ids than any fleet has
)

// arranged is the arrival order with the placed ids first, in the person's order.
func arranged(arrival, placed []string) []string {
	here := make(map[string]bool, len(arrival))
	for _, id := range arrival {
		here[id] = true
	}
	out := make([]string, 0, len(arrival))
	taken := make(map[string]bool, len(arrival))
	for _, id := range placed {
		if here[id] && !taken[id] {
			out = append(out, id)
			taken[id] = true
		}
	}
	for _, id := range arrival {
		if !taken[id] {
			out = append(out, id)
		}
	}
	return out
}

// SetOrder keeps the order the person gave the machines: every id one the registry has, each once. List follows it
// from then on, here and after a restart.
func (m *Manager) SetOrder(ids []string) error {
	if len(ids) > maxPlaced {
		return fmt.Errorf("an order names %d machines at most", maxPlaced)
	}
	m.mu.Lock()
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if m.machines[id] == nil {
			m.mu.Unlock()
			return fmt.Errorf("machine %q not found", id)
		}
		if seen[id] {
			m.mu.Unlock()
			return fmt.Errorf("machine %q is named twice", id)
		}
		seen[id] = true
	}
	placed := append([]string{}, ids...)
	m.placed = placed
	m.mu.Unlock()
	if err := state.SaveJSON(m.dir, orderFile, placed); err != nil {
		return fmt.Errorf("the order could not be kept: %w", err)
	}
	return nil
}
