package tasks

import (
	"database/sql"
	"errors"
)

// Moved is a task a trigger moved, and the column it went to.
type Moved struct {
	Task Task
	To   Status
}

// OnTrigger moves the task whose live run is in the pane to the column one of the person's triggers names — by its
// title, in the task's own project — with a comment by remuda saying why. Never into the Queued column, which takes
// only the person's own moves; never a card a pipeline holds while it runs; never a task done back open, since a
// line the agent prints again is no undo of the person's word; nil when nothing moved.
func (st *Store) OnTrigger(machine, pane, column, why string) (*Moved, error) {
	r, err := scanRun(st.db.QueryRow(`SELECT `+runCols+` FROM task_runs WHERE machine = ? AND pane = ? AND state IN (?, ?, ?) ORDER BY id DESC LIMIT 1`,
		machine, pane, RunStarting, RunRunning, RunFinished))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if r.Pipeline == PipelineOn || r.Pipeline == PipelinePassed {
		return nil, nil
	}
	t, err := st.Get(r.TaskID)
	if err != nil {
		return nil, err
	}
	ss, err := st.statusesOf(t.ProjectID)
	if err != nil {
		return nil, err
	}
	to := statusByTitle(ss, column)
	if to == nil || to.OnQueue || to.ID == t.StatusID || t.Done && to.Kind != KindDone {
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
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if _, err := st.AddComment(t.ID, "remuda", "Moved to "+to.Title+" by "+why); err != nil {
		return nil, err
	}
	if t, err = st.Get(t.ID); err != nil {
		return nil, err
	}
	return &Moved{Task: t, To: *to}, nil
}
