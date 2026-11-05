package tasks

// The pipeline: a project's stages after the first agent's work. Every prompt of a run in a pipeline asks the agent to
// end its answer with StageMarker once its part is done; such an answer starts the next stage — its agent in a new tab
// of the same worktree, its prompt with the task, the previous stage's answer and the branch filled in, the card in the
// stage's column — and the last one's ends the pipeline, the card going where a finish sends it. An answer without the
// marker (a question asked in words too), a failed start, a card the person moved or a task done stops it for that run;
// a permission prompt or a choice dialog only waits, the answer after it decides. While it runs, only the pipeline moves
// the card: a named pull request and an earlier stage's tab answering again leave it where it is.

import (
	"fmt"
	"strings"

	"github.com/dev1antdev/remuda/internal/said"
)

// StageMarker is the line a pipeline stage's agent ends its answer with once its part of the work is done.
const StageMarker = "STAGE DONE"

// The pipeline states a run is in.
const (
	PipelineOn     = "on"     // the run's stage is under way
	PipelinePassed = "passed" // its stage is done and the next one runs in another tab
	PipelineDone   = "done"   // the last stage is done
	pipelineStop   = "stopped: "
)

// StageAsk ends every prompt of a run in a pipeline.
const StageAsk = "When your part of this work is done, end your answer with a line reading " + StageMarker + ", and the next stage takes over. If you need the person instead, ask, and leave that line out."

// PRAsk is a PR stage's request, the review's own wording for an approval that opens a pull request.
const PRAsk = "Commit the work now, with a message that says what changed for the person, push the branch, and open a pull request with `gh pr create` against the repository's usual base branch; name the pull request's link in your answer."

// StageDone says whether an answer ends with the marker: its last line outside a code fence, the marks stripped, is the
// marker alone or after a sentence or a label ("All tests pass. STAGE DONE", "Status: STAGE DONE") — never a phrase that
// only mentions it ("this is not STAGE DONE yet"), since the next stage's agent starts on it.
func StageDone(answer string) bool {
	before, ok := strings.CutSuffix(lastLine(answer), StageMarker)
	if !ok {
		return false
	}
	before = strings.TrimSpace(before)
	return before == "" || strings.ContainsAny(before[len(before)-1:], ".!:;-") || strings.HasSuffix(before, "—") || strings.HasSuffix(before, "–") || strings.HasSuffix(before, "→")
}

// lastLine is an answer's last non-empty line outside a fence, its Markdown marks and closing punctuation stripped.
func lastLine(answer string) string {
	inFence := false
	last := ""
	for _, raw := range strings.Split(answer, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || line == "" {
			continue
		}
		last = strings.TrimRight(said.Plain(line), " .!")
	}
	return last
}

// withoutMarker is an answer handed to the next stage: the marker line gone.
func withoutMarker(answer string) string {
	lines := strings.Split(strings.TrimRight(answer, " \n"), "\n")
	n := len(lines)
	if i := strings.LastIndex(lines[n-1], StageMarker); i >= 0 {
		if lines[n-1] = strings.TrimRight(lines[n-1][:i], " *_`"); strings.TrimSpace(lines[n-1]) == "" {
			lines = lines[:n-1]
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// StagePrompt is what a stage's agent gets: its prompt with {task}, {previous} and {branch} filled in, a PR stage's
// request, and the ask to end with the marker.
func StagePrompt(stage Stage, t Task, project Project, previous, branch string) string {
	body := strings.NewReplacer("{task}", Prompt(t, project, false), "{previous}", previous, "{branch}", branch).Replace(stage.Prompt)
	var parts []string
	if body = strings.TrimSpace(body); body != "" {
		parts = append(parts, body)
	}
	if stage.PR {
		parts = append(parts, PRAsk)
	}
	return strings.Join(append(parts, StageAsk), "\n\n")
}

// StartStageRun records the next stage's agent in its new tab: a run on the same worktree and branch, the previous
// one passed, the card moved to the stage's column when it names one of the project's.
func (st *Store) StartStageRun(prev Run, index int, stage Stage, pane string) (Run, error) {
	t, err := st.Get(prev.TaskID)
	if err != nil {
		return Run{}, err
	}
	ss, err := st.statusesOf(t.ProjectID)
	if err != nil {
		return Run{}, err
	}
	r := Run{TaskID: prev.TaskID, Machine: prev.Machine, Workspace: prev.Workspace, Pane: pane, Dir: prev.Dir, Branch: prev.Branch,
		Agent: agentOr(stage.Agent), State: RunStarting, Started: st.stamp(), ByQueue: prev.ByQueue, Stage: index, Pipeline: PipelineOn, Expect: t.StatusID}
	column := statusByID(ss, stage.StatusID)
	if column != nil {
		r.Expect = column.ID
	}
	tx, err := st.db.Begin()
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO task_runs (task_id, machine, workspace, pane, dir, branch, agent, state, started, by_queue, stage, pipeline, expect_status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.TaskID, r.Machine, r.Workspace, r.Pane, r.Dir, r.Branch, r.Agent, r.State, r.Started, r.ByQueue, r.Stage, r.Pipeline, r.Expect)
	if err != nil {
		return Run{}, err
	}
	r.ID, _ = res.LastInsertId()
	if _, err := tx.Exec(`UPDATE task_runs SET pipeline = ? WHERE id = ?`, PipelinePassed, prev.ID); err != nil {
		return Run{}, err
	}
	if column != nil && column.ID != t.StatusID {
		if err := moveTo(tx, t, *column, r.Started); err != nil {
			return Run{}, err
		}
	}
	return r, tx.Commit()
}

// StopPipeline ends a run's pipeline with the reason, the task told.
func (st *Store) StopPipeline(runID int64, why string) error {
	r, err := st.Run(runID)
	if err != nil {
		return err
	}
	if r.Pipeline != PipelineOn {
		return nil
	}
	if _, err := st.db.Exec(`UPDATE task_runs SET pipeline = ? WHERE id = ?`, pipelineStop+why, runID); err != nil {
		return err
	}
	_, err = st.AddComment(r.TaskID, "remuda", "The pipeline stops at "+stageName(st, r)+": "+why+". The worktree is there; carry on by hand.")
	return err
}

// FinishPipeline ends a run's pipeline after its last stage: the card goes to the project's on_finish column when it
// still sits where the pipeline put it; the column it went to, nil when it stayed.
func (st *Store) FinishPipeline(r Run) (*Status, error) {
	if _, err := st.db.Exec(`UPDATE task_runs SET pipeline = ? WHERE id = ?`, PipelineDone, r.ID); err != nil {
		return nil, err
	}
	if _, err := st.AddComment(r.TaskID, "remuda", "The pipeline is done: every stage said STAGE DONE."); err != nil {
		return nil, err
	}
	t, err := st.Get(r.TaskID)
	if err != nil {
		return nil, err
	}
	ss, err := st.statusesOf(t.ProjectID)
	if err != nil {
		return nil, err
	}
	to := flagged(ss, func(x Status) bool { return x.OnFinish })
	if to == nil || to.ID == t.StatusID || t.Done || (r.Expect != 0 && t.StatusID != r.Expect) {
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

// stageName is a run's stage in words: the first agent's work, or the stage's name.
func stageName(st *Store, r Run) string {
	if r.Stage == 0 {
		return "the first agent's work"
	}
	t, err := st.Get(r.TaskID)
	if err != nil {
		return fmt.Sprintf("stage %d", r.Stage)
	}
	p, err := st.Project(t.ProjectID)
	if err != nil || r.Stage > len(p.Automation.Stages) {
		return fmt.Sprintf("stage %d", r.Stage)
	}
	return fmt.Sprintf("stage %d, %s", r.Stage, p.Automation.Stages[r.Stage-1].Name)
}

// onStageAnswer is what a finished answer of a run in a pipeline does: the next stage started on the marker, the
// pipeline done after the last stage, stopped when the card was moved or the answer ends without the marker. handled
// is true when the pipeline moved the card itself, so the usual finish move stays out of it; words are for the push.
func (w *RunWatch) onStageAnswer(r Run, answer string) (words string, moved *Status, handled bool) {
	t, err := w.Store.Get(r.TaskID)
	if err != nil {
		return "", nil, false
	}
	p, err := w.Store.Project(t.ProjectID)
	if err != nil {
		return "", nil, false
	}
	stop := func(why string) (string, *Status, bool) {
		_ = w.Store.StopPipeline(r.ID, why)
		return "the pipeline stops: " + why, nil, false
	}
	if t.Done {
		return stop("the task is done")
	}
	if r.Expect != 0 && t.StatusID != r.Expect {
		return stop("the card was moved by hand")
	}
	if !StageDone(answer) {
		return stop("the agent answered without " + StageMarker)
	}
	stages := p.Automation.Stages
	next := r.Stage + 1
	if next > len(stages) {
		to, err := w.Store.FinishPipeline(r)
		if err != nil {
			return "", nil, false
		}
		return "the pipeline is done", to, true
	}
	stage := stages[next-1]
	if w.Advance == nil {
		return stop("nothing here can start the next stage")
	}
	prompt := StagePrompt(stage, t, p, withoutMarker(answer), r.Branch)
	if err := w.Advance(r, next, stage, prompt); err != nil {
		return stop(fmt.Sprintf("stage %d, %s, could not start: %s", next, stage.Name, err))
	}
	_, _ = w.Store.AddComment(r.TaskID, "remuda", fmt.Sprintf("Stage %d of %d, %s: %s takes over in a new tab of the worktree.", next, len(stages), stage.Name, agentOr(stage.Agent)))
	cur, err := w.Store.Get(r.TaskID)
	var column *Status
	if err == nil && cur.StatusID != t.StatusID {
		if ss, err := w.Store.statusesOf(cur.ProjectID); err == nil {
			column = statusByID(ss, cur.StatusID)
		}
	}
	return fmt.Sprintf("stage %d of %d, %s", next, len(stages), stage.Name), column, true
}
