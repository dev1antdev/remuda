package opinion

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
)

// Service is the store with what a reviewer's finish sets off: its answer read, and its tab closed once that answer is
// read when the opinion asked for that. An unreadable answer keeps the tab, so the person can read what it said.
type Service struct {
	*Store
	// Close takes down the reviewer's tab, only while it still holds the reviewer's pane; set by the app, nil keeps
	// every tab
	Close func(machine, tab, pane string) error
	// Read is the reviewer's answer: the text the conversation opening with the opinion's request ended with, its pane's
	// own session read first, else the conversation found by the request; set by the app, nil reads nothing
	Read      func(op Opinion) (answer string, ok bool)
	retry     time.Duration
	readEvery time.Duration // how often a reviewer at rest is read again when no status move told of its finish

	mu      sync.Mutex
	reading map[string]bool      // machine|pane being read now
	tried   map[string]time.Time // machine|pane read last at
}

// NewService is the store in the state dir with nothing wired yet.
func NewService(dir string) *Service {
	return &Service{Store: NewStore(dir), retry: 3 * time.Second, readEvery: 20 * time.Second, reading: map[string]bool{}, tried: map[string]time.Time{}}
}

// Hear takes an answer read for a pane; a running reviewer's is its opinion.
func (svc *Service) Hear(machine, pane, text string) {
	op, ok := svc.Answer(machine, pane, text)
	if !ok || !op.Close || op.State != Done || svc.Close == nil {
		return
	}
	if err := svc.Close(machine, op.Tab, op.Pane); err != nil {
		log.Printf("opinion: the reviewer's tab %s on %s stays: %v", op.Tab, machine, err)
	}
}

// OnStatus hears every pane's status move: a reviewer's pane gone is its opinion gone; one at rest is read here, its
// conversation the one opening with the request — another conversation of the folder's is never taken for the review.
func (svc *Service) OnStatus(e events.PaneStatus) {
	if e.To == events.Gone {
		svc.Gone(e.Pane.Machine, e.Pane.Pane)
		return
	}
	if (e.To != "done" && e.To != "idle") || svc.Read == nil || !svc.Waiting(e.Pane.Machine, e.Pane.Pane) {
		return
	}
	if svc.start(e.Pane.Machine, e.Pane.Pane, 0) {
		go svc.readAnswer(e.Pane.Machine, e.Pane.Pane)
	}
}

// Observe looks at every snapshot of a machine for the finishes no status move told of — remuda was not running
// then, or the move came before the request was kept: a reviewer at rest is read, at most every readEvery, and one
// whose pane is gone from the machine is gone.
func (svc *Service) Observe(e events.Snapshot) {
	if e.Snap == nil || svc.Read == nil {
		return
	}
	for _, op := range svc.runningOn(e.Machine) {
		pane := e.Snap.Pane(op.Pane)
		if pane == nil {
			if e.At.After(op.Asked.Add(30 * time.Second)) { // a snapshot from before the tab was made does not count
				svc.Gone(op.Machine, op.Pane)
			}
			continue
		}
		if pane.AgentStatus != "done" && pane.AgentStatus != "idle" {
			continue
		}
		if svc.start(op.Machine, op.Pane, svc.readEvery) {
			go svc.readAnswer(op.Machine, op.Pane)
		}
	}
}

// start claims a read of the pane: none in flight, and none begun within the last `every`.
func (svc *Service) start(machine, pane string, every time.Duration) bool {
	k := key(machine, pane)
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.reading[k] || (every > 0 && time.Since(svc.tried[k]) < every) {
		return false
	}
	svc.reading[k], svc.tried[k] = true, time.Now()
	return true
}

// readAnswer reads the reviewer's answer, a few seconds apart while the file catches up with the finish
func (svc *Service) readAnswer(machine, pane string) {
	defer func() {
		svc.mu.Lock()
		delete(svc.reading, key(machine, pane))
		svc.mu.Unlock()
	}()
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(svc.retry)
		}
		op, waiting := svc.waitingOn(machine, pane)
		if !waiting {
			return
		}
		answer, ok := svc.Read(op)
		if ok && strings.TrimSpace(answer) != "" {
			svc.Hear(machine, pane, answer)
			return
		}
	}
}

// IsRequest says whether a conversation's first prompt is the request that named this ref: a second opinion asked
// before in the same folder is another conversation.
func IsRequest(prompt, ref string) bool {
	return ref != "" && strings.HasPrefix(strings.TrimSpace(prompt), PromptStart) && strings.Contains(prompt, refLine(ref))
}
