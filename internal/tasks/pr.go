package tasks

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/dev1antdev/remuda/internal/push"
)

// PRNote is what noting a pull request on a task came to: the run that holds it, the task, and the column the task moved to, nil when it stayed.
type PRNote struct {
	Run   Run
	Task  Task
	Moved *Status
}

// OnPullRequest notes the pull request a pane's agent named on the live run of that pane: the run keeps the address and
// the label, the task gets a comment by "agent", and it moves to the project's on_finish column as a finish would (a card
// the person moved stays). Nil when no live run is on the pane, or the run already names that very request.
func (st *Store) OnPullRequest(machine, pane, url, label string) (*PRNote, error) {
	if url == "" {
		return nil, nil
	}
	r, err := scanRun(st.db.QueryRow(`SELECT `+runCols+` FROM task_runs WHERE machine = ? AND pane = ? AND state IN (?, ?, ?) ORDER BY id DESC LIMIT 1`,
		machine, pane, RunStarting, RunRunning, RunFinished))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if r.PRURL == url {
		return nil, nil
	}
	if label == "" {
		label = "a pull request"
	}
	if _, err := st.db.Exec(`UPDATE task_runs SET pr_url = ?, pr_label = ? WHERE id = ?`, url, label, r.ID); err != nil {
		return nil, err
	}
	r.PRURL, r.PRLabel = url, label
	if _, err := st.AddComment(r.TaskID, "agent", "Opened "+label+": "+url); err != nil {
		return nil, err
	}
	moved, err := st.OnFinish(r.TaskID)
	if err != nil {
		return nil, err
	}
	t, err := st.Get(r.TaskID)
	if err != nil {
		return nil, err
	}
	return &PRNote{Run: r, Task: t, Moved: moved}, nil
}

// OnPullRequest hears that a pane's agent named a pull request: the task it runs on notes it and the person gets a push.
func (w *RunWatch) OnPullRequest(machine, pane, url, label string) {
	note, err := w.Store.OnPullRequest(machine, pane, url, label)
	if err != nil || note == nil || w.Notify == nil {
		return
	}
	title := fmt.Sprintf("Task %s: %s opened", note.Task.Identifier, note.Run.PRLabel)
	if note.Moved != nil {
		title += " → " + note.Moved.Title
	}
	w.Notify(push.Payload{
		Machine: machine, Pane: pane, Kind: "task",
		Title:   title,
		Body:    firstLine(note.Task.Title) + " — " + url,
		Tag:     fmt.Sprintf("task|%d", note.Task.ID),
		Actions: []push.Action{{Action: "open", Title: "Open"}},
	})
}
