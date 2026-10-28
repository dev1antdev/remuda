package tasks

import (
	"fmt"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/text"
)

// RunWatch follows the agents started on tasks: every finish of the agent comes back to the task as a comment and a push; a pane that is no more closes the run.
type RunWatch struct {
	Store *Store
	// PaneState is the radar's view of a pane (working | blocked | done | idle | unknown) and since when; ok is false when the pane is gone.
	PaneState func(machine, pane string) (state string, since time.Time, ok bool)
	// LastAnswer is the text the agent's latest turn ended with and the id of its last record;
	// "" when the turn ends in a tool call, since a permission stop or a question asked through a tool is no answer.
	LastAnswer func(machine, pane string) (text string, id string)
	// Usage is the meter's reading of a pane: what its agent spent so far; ok is false before anything was read.
	Usage  func(machine, pane string) (cost float64, tokens int64, model string, ok bool)
	Notify func(push.Payload)
	Now    func() time.Time

	missing map[int64]time.Time
}

// startWindow is how long a starting run may wait for its prompt before it is given up.
// The deliverer's five minutes, and a margin for a restart that lost the deliverer.
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
	if w.missing == nil {
		w.missing = map[int64]time.Time{}
	}
	for _, r := range runs {
		state, _, ok := w.PaneState(r.Machine, r.Pane)
		started, _ := time.Parse(time.RFC3339, r.Started)
		if !ok {
			// herdr needs a moment to show a new pane: gone is two minutes of absence, not one snapshot without it
			first, seen := w.missing[r.ID]
			if !seen {
				w.missing[r.ID] = now()
				continue
			}
			if now().Sub(first) > 2*time.Minute && now().Sub(started) > 2*time.Minute {
				_ = w.Store.SetRun(r.ID, RunGone, "the pane is no more")
				_, _ = w.Store.AddComment(r.TaskID, "remuda", "The agent's workspace was closed.")
				delete(w.missing, r.ID)
			}
			continue
		}
		delete(w.missing, r.ID)
		if r.State == RunStarting {
			if now().Sub(started) > startWindow {
				_ = w.Store.SetRun(r.ID, RunFailed, "the prompt was never delivered")
				_, _ = w.Store.AddComment(r.TaskID, "remuda", "The task was never handed to the agent. The worktree is there; send it by hand.")
			}
			continue
		}
		w.noteUsage(r, started, now())
		if state == "working" || state == "unknown" {
			continue
		}
		// a fresh session has no turn before the prompt, so any turn ending in text and not yet reported is a finish
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
		r.State, r.Finished = RunFinished, now().UTC().Format(time.RFC3339)
		w.noteUsage(r, started, now())
		moved, _ := w.Store.OnFinish(r.TaskID)
		if w.Notify != nil {
			t, _ := w.Store.Get(r.TaskID)
			title := fmt.Sprintf("Task %s: %s finished", t.Identifier, r.Agent)
			if moved != nil {
				title += " → " + moved.Title
			}
			w.Notify(push.Payload{
				Machine: r.Machine, Pane: r.Pane, Kind: "task",
				Title:   title,
				Body:    firstLine(t.Title) + " — " + firstLine(answer),
				Tag:     fmt.Sprintf("task|%d", r.TaskID),
				Actions: []push.Action{{Action: "open", Title: "Open"}},
			})
		}
	}
}

// noteUsage writes the meter's figures on the run when they moved: every sweep while the pane is there, so they stay
// once it is gone; took is the seconds from the start to the finish, 0 while the agent still works.
func (w *RunWatch) noteUsage(r Run, started, now time.Time) {
	if w.Usage == nil {
		return
	}
	cost, tokens, model, ok := w.Usage(r.Machine, r.Pane)
	if !ok {
		return
	}
	took := 0
	if r.State == RunFinished {
		finished, err := time.Parse(time.RFC3339, r.Finished)
		if err != nil {
			finished = now
		}
		took = int(finished.Sub(started).Round(time.Second).Seconds())
	}
	if cost == r.Cost && tokens == r.Tokens && model == r.Model && took == r.Took {
		return
	}
	_ = w.Store.SetRunUsage(r.ID, cost, tokens, model, took)
}

// Loop sweeps forever.
func (w *RunWatch) Loop(every time.Duration) {
	for {
		w.Sweep()
		time.Sleep(every)
	}
}

func firstLine(s string) string {
	return text.FirstLine(s, 120)
}
