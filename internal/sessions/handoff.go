package sessions

import (
	"time"

	"github.com/dev1antdev/remuda/internal/coreapi"
	"github.com/dev1antdev/remuda/internal/events"
)

// ScreenReader is a client whose agent can be prompted and whose pane's screen can be read: coreapi.API.
type ScreenReader interface {
	Prompter
	PaneRead(paneID string) (string, error)
}

// Handoff hands a fresh agent its first prompt: a try every Retry until Timeout, Claude Code's "do you trust this
// folder" question for Dir answered yes, and one Notify when the agent asks something else.
type Handoff struct {
	Input  *Input
	Client ScreenReader
	Ref    events.PaneRef
	Prompt string
	Dir    string
	Home   string
	Retry  time.Duration // 0: 2 s
	// Timeout: 0: 5 min
	Timeout time.Duration
	Now     func() time.Time // nil: time.Now
	Notify  func()           // once, on the first question that is not the trust one
	Trusted func()           // once, the trust question answered
}

// Run hands the prompt over: nil, false once it went; the refusal, false when the agent refused it; the last retry,
// true when the timeout ran out.
func (h Handoff) Run() (last error, timedOut bool) {
	now := h.Now
	if now == nil {
		now = time.Now
	}
	retry := h.Retry
	if retry == 0 {
		retry = 2 * time.Second
	}
	timeout := h.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	lock := h.Input.Lock(h.Ref.Machine, h.Ref.Pane)
	deadline := now().Add(timeout)
	nudged, trusted := false, false
	delivery := h.Input.Deliver(h.Client, h.Ref, h.Prompt)
	for now().Before(deadline) {
		time.Sleep(retry)
		err := delivery.Try()
		switch {
		case err == nil:
			return nil, false
		case coreapi.IsCode(err, "agent_not_ready") || coreapi.IsCode(err, "agent_not_found") || coreapi.IsCode(err, "agent_working"):
			last = err
		case coreapi.IsCode(err, "agent_blocked"):
			last = err
			if !trusted {
				if screen, rerr := h.Client.PaneRead(h.Ref.Pane); rerr == nil && TrustPrompt(screen, h.Dir, h.Home) {
					trusted = true
					lock.Lock()
					if !CursorOnYes(screen) { // "No, exit" sits first and focused; older builds put Yes first
						_ = h.Client.PaneSendKeys(h.Ref.Pane, []string{"down"})
						time.Sleep(150 * time.Millisecond)
					}
					_ = h.Client.PaneSendKeys(h.Ref.Pane, []string{"enter"})
					lock.Unlock()
					if h.Trusted != nil {
						h.Trusted()
					}
					continue
				}
			}
			if !nudged && h.Notify != nil {
				nudged = true
				h.Notify()
			}
		default:
			return err, false
		}
	}
	return last, true
}
