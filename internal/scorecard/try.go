package scorecard

import "time"

// Try is a past task to hand to an arena: the model that ran it and the row's own, side by side on the same prompt.
type Try struct {
	TaskID     int64     `json:"task_id"`
	Identifier string    `json:"identifier"`
	Title      string    `json:"title"`
	ProjectID  int64     `json:"project_id"`
	Machine    string    `json:"machine"`
	Dir        string    `json:"dir"` // the worktree it ran in: the page finds the repository to branch off from it
	Ran        time.Time `json:"ran"`
	Lanes      []Lane    `json:"lanes"` // the model that ran it first; one lane alone when no other model is known
}

// Lane is one agent of a suggested arena.
type Lane struct {
	Agent   string `json:"agent"`
	Model   string `json:"model,omitempty"`
	ModelID string `json:"model_id,omitempty"`
}

func laneOf(agent, model string) Lane {
	return Lane{Agent: agent, Model: model, ModelID: cliModel(model)}
}

// suggest gives every row a past task to try it on: the newest finished task another model did, the two side by
// side; else the newest the row's own model did, beside the model of the row most recently at work.
func suggest(rows []Row, runs []*run) {
	var done []*run // the finished work runs, the newest first
	for i := len(runs) - 1; i >= 0; i-- {
		if r := runs[i]; r.Stage == 0 && r.Finished != "" {
			done = append(done, r)
		}
	}
	for i := range rows {
		row := &rows[i]
		mine := rowKey{row.Agent, row.Model}
		if other := newestBy(done, func(k rowKey) bool { return k != mine }); other != nil {
			row.Try = tryOf(other, laneOf(other.Agent, normalName(other.Model)), laneOf(row.Agent, row.Model))
			continue
		}
		own := newestBy(done, func(k rowKey) bool { return k == mine })
		if own == nil {
			continue
		}
		lanes := []Lane{laneOf(row.Agent, row.Model)}
		if challenger := recentOther(rows, i); challenger != nil {
			lanes = append(lanes, laneOf(challenger.Agent, challenger.Model))
		}
		row.Try = tryOf(own, lanes...)
	}
}

// newestBy is the newest run whose row matches, one with a model read before one without.
func newestBy(done []*run, match func(rowKey) bool) *run {
	var unread *run
	for _, r := range done {
		k := rowKey{r.Agent, normalName(r.Model)}
		if !match(k) {
			continue
		}
		if k.model != "" {
			return r
		}
		if unread == nil {
			unread = r
		}
	}
	return unread
}

// recentOther is the row most recently at work other than the one at index skip.
func recentOther(rows []Row, skip int) *Row {
	var found *Row
	for i := range rows {
		if i != skip && (found == nil || rows[i].Last.After(found.Last)) {
			found = &rows[i]
		}
	}
	return found
}

func tryOf(r *run, lanes ...Lane) *Try {
	return &Try{TaskID: r.TaskID, Identifier: r.Identifier(), Title: r.Title, ProjectID: r.ProjectID, Machine: r.Machine, Dir: r.Dir, Ran: r.started, Lanes: lanes}
}
