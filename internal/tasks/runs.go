package tasks

import (
	"fmt"
	"strings"
	"time"

	"github.com/dev1antdev/herdr-ui/internal/push"
)

// RunWatch follows the agents started on tasks: once the prompt is in, every
// finish of the agent (a turn after the prompt while it is not working — an
// answer, or a question it stopped at) comes back to the task as a comment,
// and a push says so; a pane that is no more closes the run. What it asks of
// herdr is injected, so this stays testable.
type RunWatch struct {
	Store *Store
	// PaneState is the radar's view of a pane: working | blocked | done |
	// idle | unknown, since when; ok is false when the pane is gone.
	PaneState func(machine, pane string) (state string, since time.Time, ok bool)
	// LastAnswer is what the agent's latest turn ended with — its text after
	// the last tool call — and the id of the record it ended with; "" when
	// there is none, or the turn ends in a tool call (a permission stop, a
	// question asked through a tool: not an answer).
	LastAnswer func(machine, pane string) (text string, id string)
	Notify     func(push.Payload)
	Now        func() time.Time
}

// startWindow is how long a starting run may wait for its prompt before
// it is given up (the deliverer's five minutes, and a margin for a restart
// that lost the deliverer).
const startWindow = 6 * time.Minute

// Sweep looks at every open run once.
func (w *RunWatch) Sweep() {
	now := time.Now
	if w.Now != nil {
		now = w.Now
	}
	runs, err := w.Store.OpenRuns()
	if err != nil {
		return
	}
	for _, r := range runs {
		state, _, ok := w.PaneState(r.Machine, r.Pane)
		started, _ := time.Parse(time.RFC3339, r.Started)
		if !ok {
			// herdr needs a moment to show a new pane; after that, gone is gone
			if now().Sub(started) > 2*time.Minute {
				_ = w.Store.SetRun(r.ID, RunGone, "the pane is no more")
				_, _ = w.Store.AddComment(r.TaskID, "herdr-ui", "The agent's workspace was closed.")
			}
			continue
		}
		if r.State == RunStarting {
			if now().Sub(started) > startWindow { // the deliverer gave up, or a restart lost it
				_ = w.Store.SetRun(r.ID, RunFailed, "the prompt was never delivered")
				_, _ = w.Store.AddComment(r.TaskID, "herdr-ui", "The task was never handed to the agent. The worktree is there; send it by hand.")
			}
			continue
		}
		if state == "working" || state == "unknown" {
			continue
		}
		// a finish: the agent is not working and its latest turn ended in
		// text (a fresh session has no turn before the prompt, so any turn
		// is after it) that was not reported yet
		answer, id := w.LastAnswer(r.Machine, r.Pane)
		answer = strings.TrimSpace(answer)
		if answer == "" || id == "" || id == r.Reported {
			continue
		}
		if rs := []rune(answer); len(rs) > 6000 {
			answer = "…" + string(rs[len(rs)-6000:]) // the end is the answer
		}
		_, _ = w.Store.AddComment(r.TaskID, "agent", answer)
		_ = w.Store.SetRun(r.ID, RunFinished, id)
		if w.Notify != nil {
			t, _ := w.Store.Get(r.TaskID)
			w.Notify(push.Payload{
				Machine: r.Machine, Pane: r.Pane, Kind: "task",
				Title:   fmt.Sprintf("Task %s: %s finished", t.Identifier, r.Agent),
				Body:    firstLine(t.Title) + " — " + firstLine(answer),
				Tag:     fmt.Sprintf("task|%d", r.TaskID),
				Actions: []push.Action{{Action: "open", Title: "Open"}},
			})
		}
	}
}

// Loop sweeps forever.
func (w *RunWatch) Loop(every time.Duration) {
	for {
		w.Sweep()
		time.Sleep(every)
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	if r := []rune(line); len(r) > 120 {
		line = string(r[:120]) + "…"
	}
	return line
}
