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

// OnPullRequestMerged notes that the request a pane's live run names was merged: a comment by "remuda", and the task
// moves to the project's done column when it still sits in the start or finish column (a card the person moved stays).
// Nil when no live run on the pane names that request, or the merge was noted before.
func (st *Store) OnPullRequestMerged(machine, pane, url string) (*PRNote, error) {
	if url == "" {
		return nil, nil
	}
	r, err := scanRun(st.db.QueryRow(`SELECT `+runCols+` FROM task_runs WHERE machine = ? AND pane = ? AND pr_url = ? AND state IN (?, ?, ?) ORDER BY id DESC LIMIT 1`,
		machine, pane, url, RunStarting, RunRunning, RunFinished))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	note := r.PRLabel + " merged"
	var noted int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM comments WHERE task_id = ? AND author = 'remuda' AND body = ?`, r.TaskID, note).Scan(&noted); err != nil {
		return nil, err
	}
	if noted > 0 {
		return nil, nil
	}
	if _, err := st.AddComment(r.TaskID, "remuda", note); err != nil {
		return nil, err
	}
	moved, err := st.onMerged(r.TaskID)
	if err != nil {
		return nil, err
	}
	t, err := st.Get(r.TaskID)
	if err != nil {
		return nil, err
	}
	return &PRNote{Run: r, Task: t, Moved: moved}, nil
}

// onMerged moves a task to the done column when it still sits in the start or finish column; nil when it stays.
func (st *Store) onMerged(taskID int64) (*Status, error) {
	t, err := st.Get(taskID)
	if err != nil {
		return nil, err
	}
	ss, err := st.statusesOf(t.ProjectID)
	if err != nil {
		return nil, err
	}
	to := doneStatus(ss)
	if to == nil || to.ID == t.StatusID || t.Done {
		return nil, nil
	}
	in := func(x Status) bool { return x.ID == t.StatusID && (x.OnStart || x.OnFinish) }
	if flagged(ss, in) == nil {
		return nil, nil
	}
	tx, err := st.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := moveTo(tx, t, *to, st.stamp()); err != nil {
		return nil, err
	}
	return to, tx.Commit()
}

// OnPullRequestState hears what gh read of a request a pane's agent named: a merge closes the task's loop with a push.
func (w *RunWatch) OnPullRequestState(machine, pane, url, state string) {
	if state != "MERGED" {
		return
	}
	note, err := w.Store.OnPullRequestMerged(machine, pane, url)
	if err != nil || note == nil || w.Notify == nil {
		return
	}
	title := fmt.Sprintf("Task %s: %s merged", note.Task.Identifier, note.Run.PRLabel)
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
