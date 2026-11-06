package machines

import (
	"errors"
	"testing"
)

// TestTheManagerHearsARemoteMove pins that a remote's fail is told to the manager's ear once per move, never twice for one outage.
func TestTheManagerHearsARemoteMove(t *testing.T) {
	var heard []string
	m := &Manager{machines: map[string]*Machine{}, names: map[string]string{}}
	m.OnState(func(id, label, state, reason string) { heard = append(heard, id+" "+label+" "+state+" "+reason) })
	mc := newRemote(Config{ID: "box1", Label: "Box"})
	mc.onState = m.stateMoved
	mc.fail(errors.New("ssh: refused"))
	mc.fail(errors.New("ssh: refused again")) // already in error: not a move
	mc.mu.Lock()
	mc.state = StateConnected
	mc.mu.Unlock()
	mc.fail(errors.New("connection lost"))
	want := []string{"box1 Box error ssh: refused", "box1 Box error connection lost"}
	if len(heard) != len(want) || heard[0] != want[0] || heard[1] != want[1] {
		t.Fatalf("heard %q, want %q", heard, want)
	}
}
