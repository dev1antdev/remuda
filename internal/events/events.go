// Package events is the in-process bus the parts of remuda hear each other
// through: the watcher says what it saw, a job that it ended, the local herdr
// that it stopped answering, and push, the restarts, the vitals and the meters
// subscribe to what concerns them. Nothing holds a reference to its listeners;
// the wiring is one list of subscriptions in the app.
package events

import (
	"reflect"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// PaneRef names a pane across machines.
type PaneRef struct{ Machine, Pane string }

func (r PaneRef) String() string { return r.Machine + "|" + r.Pane }

// Gone is the status a closed pane moves to.
const Gone = "gone"

// PaneStatus is a pane's status transition as the watcher saw it.
type PaneStatus struct {
	Pane      PaneRef
	Workspace string
	PaneTitle string
	Agent     string
	Kind      string        // the agent kind herdr detected ("claude"), "" for a shell
	From, To  string        // herdr statuses; To is Gone when the pane closed
	Worked    time.Duration // how long the pane was working before this
}

// Snapshot is a machine's session snapshot as the watcher just read it.
type Snapshot struct {
	Machine string
	Snap    *herdr.Snapshot
	At      time.Time
	First   bool // the machine's first snapshot, or the first after its polls failed: what changed in between was not watched happening
}

// MachineRemoved says a machine left the registry and its watcher loop ended.
type MachineRemoved struct{ Machine string }

// JobDone is a job that ended on a machine.
type JobDone struct {
	Machine string
	Title   string
	OK      bool
}

// LocalHerdrDown says the local herdr has not answered for a few polls in a row.
type LocalHerdrDown struct{ Err error }

// PullRequestNamed says a pane's agent named a pull or merge request in its answers, first seen or changed.
type PullRequestNamed struct {
	Pane  PaneRef
	URL   string
	Label string
}

// PullRequestState says what gh read of a pull request an agent named: its state on the first read, and every move
// after it (OPEN → MERGED or CLOSED). Was is the state read before, "" on the first read.
type PullRequestState struct {
	Pane  PaneRef
	URL   string
	State string
	Was   string
}

// Bus delivers each event to every subscriber of its type, in subscription order, on the publisher's goroutine.
type Bus struct {
	mu   sync.RWMutex
	subs map[reflect.Type][]func(any)
}

func NewBus() *Bus { return &Bus{subs: map[reflect.Type][]func(any){}} }

// Subscribe registers fn for every event of type E.
func Subscribe[E any](bus *Bus, fn func(E)) {
	var zero E
	key := reflect.TypeOf(zero)
	bus.mu.Lock()
	defer bus.mu.Unlock()
	bus.subs[key] = append(bus.subs[key], func(event any) { fn(event.(E)) })
}

// Publish hands the event to its subscribers and returns when they are done.
func Publish[E any](bus *Bus, event E) {
	if bus == nil {
		return
	}
	bus.mu.RLock()
	subs := bus.subs[reflect.TypeOf(event)]
	bus.mu.RUnlock()
	for _, fn := range subs {
		fn(event)
	}
}
