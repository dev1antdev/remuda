package tasks

import (
	"fmt"
	"strings"
	"time"
)

// RunRecord is a run with the task it was on, as the models' scorecard reads it.
type RunRecord struct {
	Run
	Title     string `json:"title"`
	ProjectID int64  `json:"project_id"`
}

// Identifier is the task's "#12".
func (r RunRecord) Identifier() string { return fmt.Sprintf("#%d", r.TaskID) }

// RunsSince is every run started at or after since, the oldest first, each with its task's title and project; a deleted
// task's runs went with it.
func (st *Store) RunsSince(since time.Time) ([]RunRecord, error) {
	cols := "r." + strings.ReplaceAll(runCols, ", ", ", r.")
	rows, err := st.db.Query(`SELECT `+cols+`, t.title, t.project_id FROM task_runs r JOIN tasks t ON t.id = r.task_id WHERE r.started >= ? ORDER BY r.started, r.id`,
		since.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RunRecord
	for rows.Next() {
		var rec RunRecord
		r := &rec.Run
		if err := rows.Scan(&r.ID, &r.TaskID, &r.Machine, &r.Workspace, &r.Pane, &r.Dir, &r.Branch, &r.Agent, &r.State, &r.Started, &r.PromptSent, &r.Finished, &r.Reported, &r.Error, &r.Cost, &r.Tokens, &r.Model, &r.Took, &r.PRURL, &r.PRLabel, &r.ByQueue, &r.Stage, &r.Pipeline, &r.Expect, &r.Merged, &rec.Title, &rec.ProjectID); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// backfillMerged gives a run from before the merged column its merge: the "PR #15 merged" comment remuda wrote on its task.
func (st *Store) backfillMerged() error {
	const note = `FROM comments c WHERE c.task_id = task_runs.task_id AND c.author = 'remuda' AND c.body = task_runs.pr_label || ' merged'`
	_, err := st.db.Exec(`UPDATE task_runs SET merged = (SELECT min(c.created) ` + note + `) WHERE merged = '' AND pr_url != '' AND pr_label != '' AND EXISTS (SELECT 1 ` + note + `)`)
	return err
}
