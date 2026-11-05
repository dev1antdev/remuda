package opinion

import (
	"log"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
)

// Service is the store with what a reviewer's finish sets off: its answer read, and its tab closed once that answer is
// read when the opinion asked for that. An unreadable answer keeps the tab, so the person can read what it said.
type Service struct {
	*Store
	Close func(machine, tab string) error // set by the app; nil keeps every tab
	// Read is the reviewer pane's conversation as the chat finds it — by the session herdr names or, without one, by
	// the folder — its first prompt and the text its last turn ended with; set by the app, nil reads nothing
	Read  func(machine, pane string) (prompt, answer string, ok bool)
	retry time.Duration
}

// NewService is the store in the state dir with nothing wired yet.
func NewService(dir string) *Service { return &Service{Store: NewStore(dir), retry: 3 * time.Second} }

// Hear takes an answer read for a pane; a running reviewer's is its opinion.
func (svc *Service) Hear(machine, pane, text string) {
	op, ok := svc.Answer(machine, pane, text)
	if !ok || !op.Close || op.State != Done || svc.Close == nil {
		return
	}
	if err := svc.Close(machine, op.Tab); err != nil {
		log.Printf("opinion: the reviewer's tab %s on %s stays: %v", op.Tab, machine, err)
	}
}

// OnStatus hears every pane's status move: a reviewer's pane gone is its opinion gone; one at rest is read here, as
// the said tracker cannot read a pane herdr names no session for (a Codex pane, at times), its conversation checked
// to open with the request — another conversation of the folder's is never taken for the review.
func (svc *Service) OnStatus(e events.PaneStatus) {
	if e.To == events.Gone {
		svc.Gone(e.Pane.Machine, e.Pane.Pane)
		return
	}
	if (e.To != "done" && e.To != "idle") || svc.Read == nil || !svc.Waiting(e.Pane.Machine, e.Pane.Pane) {
		return
	}
	go svc.readAnswer(e.Pane.Machine, e.Pane.Pane)
}

// readAnswer reads the reviewer's answer, a few seconds apart while the file catches up with the finish
func (svc *Service) readAnswer(machine, pane string) {
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(svc.retry)
		}
		op, waiting := svc.waitingOn(machine, pane)
		if !waiting {
			return
		}
		prompt, answer, ok := svc.Read(machine, pane)
		if ok && IsRequest(prompt, op.Ref) && strings.TrimSpace(answer) != "" {
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
