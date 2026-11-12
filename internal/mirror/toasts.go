package mirror

import (
	"errors"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/push"
)

// Outcome is how one toast went on a machine: herdr's answer, or why it never got there.
type Outcome struct {
	Machine string    `json:"machine"`
	Label   string    `json:"label"`
	At      time.Time `json:"at"`
	Title   string    `json:"title"`
	Shown   bool      `json:"shown"`
	Reason  string    `json:"reason,omitempty"` // herdr's: disabled | rate_limited | no_foreground_client | busy
	Error   string    `json:"error,omitempty"`
}

// ErrClosed refuses a test toast from a remuda that writes nothing into herdr.
var ErrClosed = errors.New("this remuda writes nothing into herdr")

// toastSounds are the alerts a toast carries — a question, a finish, the watchdog's three notes — and the sound each
// plays when herdr shows it.
var toastSounds = map[string]string{"input": "request", "done": "done", "stuck": "request", "loop": "request", "waiting": "request"}

// longTurn is the shortest turn whose finish pops up: a device's usual "turns over a minute".
const longTurn = 60

// Alert pops an alert as herdr's own toast on the agent's machine. It hears what the push service records: past its
// delay, called off when answered at the desk, never for a muted folder.
func (s *Service) Alert(p push.Payload) {
	if toast, ok := s.toastOf(p, s.now()); ok {
		go s.show(p.Machine, toast)
	}
}

// toastOf is the toast an alert makes, when the person switched toasts on, its kind is one they carry, the hour is
// outside their quiet hours and the pane is not in the tab herdr has in front — herdr's own rule for its toasts.
func (s *Service) toastOf(p push.Payload, now time.Time) (herdr.Toast, bool) {
	settings := s.Settings()
	sound, carried := toastSounds[p.Kind]
	switch {
	case s.closed != "" || !settings.Toasts || !carried || p.Machine == "" || p.Pane == "":
		return herdr.Toast{}, false
	case p.Kind == "done" && p.Worked < longTurn:
		return herdr.Toast{}, false
	case settings.Quiet != nil && settings.Quiet.Covers(now):
		return herdr.Toast{}, false
	case s.inFront(p.Machine, p.Pane):
		return herdr.Toast{}, false
	}
	return herdr.Toast{Title: p.Title, Body: p.Body, Sound: sound}, true
}

func (s *Service) inFront(machineID, paneID string) bool {
	if s.in.Snapshot == nil {
		return false
	}
	snap := s.in.Snapshot(machineID)
	if snap == nil || snap.FocusedTabID == "" {
		return false
	}
	pane := snap.Pane(paneID)
	return pane != nil && pane.TabID == snap.FocusedTabID
}

// show pops one toast and keeps how it went as the machine's last.
func (s *Service) show(machineID string, toast herdr.Toast) Outcome {
	outcome := Outcome{Machine: machineID, Label: s.label(machineID), At: s.now(), Title: toast.Title}
	var client Herdr
	ok := false
	if s.in.Herdr != nil {
		client, ok = s.in.Herdr(machineID)
	}
	if !ok {
		outcome.Error = "the machine is not connected"
	} else if shown, err := client.NotificationShow(toast); err != nil {
		outcome.Error = err.Error()
	} else {
		outcome.Shown, outcome.Reason = shown.Shown, shown.Reason
	}
	s.mu.Lock()
	s.last[machineID] = outcome
	s.mu.Unlock()
	return outcome
}

// Test pops a toast on every connected machine at once, whatever the switches say, and answers how each went.
func (s *Service) Test() ([]Outcome, error) {
	if s.closed != "" {
		return nil, ErrClosed
	}
	machines := s.machines()
	out := make([]Outcome, len(machines))
	var wg sync.WaitGroup
	for i, m := range machines {
		wg.Go(func() {
			out[i] = s.show(m.ID, herdr.Toast{Title: "remuda", Body: "remuda's alerts reach herdr on " + m.Label + ".", Sound: "none"})
		})
	}
	wg.Wait()
	return out, nil
}

func (s *Service) label(machineID string) string {
	for _, m := range s.machines() {
		if m.ID == machineID {
			return m.Label
		}
	}
	return machineID
}
