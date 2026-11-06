package tasks

// The queue: while a project's switch is on, the top card of its Queued column is started by itself — the person's own
// Start, a worktree off the project's repository folder and its agent — once the machine is connected, fewer agents
// the queue started than the project allows still work or ask, the agent's plan window is not spent and no budget over
// the folder is crossed. Only a card the person put in the column starts; a start that was refused is not tried again
// until the card moves.

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/push"
)

// QueueNote is what a project's queue does now.
type QueueNote struct {
	State   string `json:"state"`            // idle (nothing queued) | waiting (and why) | started (and what)
	Reason  string `json:"reason,omitempty"` // why the next card waits, or which one it started
	Next    string `json:"next,omitempty"`   // the card that starts next, "#12"
	Working int    `json:"working"`          // agents the queue started that still work or ask
	Max     int    `json:"max"`
	At      string `json:"at"`
}

func (st *Store) queueNote(projectID int64) *QueueNote {
	st.notesMu.Lock()
	defer st.notesMu.Unlock()
	if note, ok := st.notes[projectID]; ok {
		return &note
	}
	return nil
}

func (st *Store) setQueueNote(projectID int64, note *QueueNote) {
	st.notesMu.Lock()
	defer st.notesMu.Unlock()
	if st.notes == nil {
		st.notes = map[int64]QueueNote{}
	}
	if note == nil {
		delete(st.notes, projectID)
		return
	}
	st.notes[projectID] = *note
}

// Queue is the loop that starts the queued tasks; its view of the machines, the agents and the money comes in as functions.
type Queue struct {
	Store *Store
	// Machine is a machine's name and whether it is connected now.
	Machine func(id string) (label string, connected bool)
	// Unusable is why no agent can ever work on the machine — the host of a remuda that only serves the page — or "".
	Unusable func(id string) string
	// Workspace is the workspace open on the machine's folder that worktrees branch off, "" when none is.
	Workspace func(machine, dir string) string
	// PaneState is the radar's state of a pane (working | blocked | done | idle | unknown); ok is false once it is gone.
	PaneState func(machine, pane string) (state string, ok bool)
	// Spent is why the agent kind cannot start on the machine now — its plan's window spent — or "".
	Spent func(machine, agent string) string
	// Crossed is the budget over the machine and folder that is spent, in words, or "".
	Crossed func(machine, dir string) string
	// Start starts the agent on the task as the person's Start does.
	Start  func(machine string, task Task, req StartRequest) error
	Notify func(push.Payload)
	Now    func() time.Time
}

// Sweep gives every project whose switch is on one look: at most one card starts per project per sweep.
func (q *Queue) Sweep() {
	projects, err := q.Store.Projects()
	if err != nil {
		return
	}
	now := time.Now
	if q.Now != nil {
		now = q.Now
	}
	for _, p := range projects {
		if !p.Automation.Queue || p.Archived {
			q.Store.setQueueNote(p.ID, nil)
			continue
		}
		note := q.step(p)
		note.At = now().UTC().Format(time.RFC3339)
		q.Store.setQueueNote(p.ID, &note)
	}
}

// step is one look at a project: nothing queued, why the next card waits, or the card it started.
func (q *Queue) step(p Project) QueueNote {
	a := p.Automation
	note := QueueNote{State: "waiting", Max: a.Max}
	column := flagged(p.Statuses, func(s Status) bool { return s.OnQueue })
	if column == nil {
		note.Reason = "no column is marked Queued"
		return note
	}
	working, err := q.Store.queueWorking(p.ID, q.PaneState)
	if err != nil {
		note.Reason = err.Error()
		return note
	}
	note.Working = working
	next, err := q.Store.nextQueued(p.ID, column.ID)
	if err != nil {
		note.Reason = err.Error()
		return note
	}
	if next == nil {
		note.State = "idle"
		return note
	}
	note.Next = next.Identifier
	agent := agentOr(a.Agent)
	if q.Unusable != nil {
		if why := q.Unusable(a.Machine); why != "" {
			note.Reason = why
			return note
		}
	}
	if label, connected := q.Machine(a.Machine); !connected {
		note.Reason = label + " is away"
		return note
	}
	if working >= a.Max {
		note.Reason = fmt.Sprintf("every slot is taken: %d of %d working", working, a.Max)
		return note
	}
	if why := q.Spent(a.Machine, agent); why != "" {
		note.Reason = why
		return note
	}
	if why := q.Crossed(a.Machine, a.Dir); why != "" {
		note.Reason = why
		return note
	}
	workspace := q.Workspace(a.Machine, a.Dir)
	if workspace == "" {
		note.Reason = "no workspace is open on " + a.Dir + " to branch off"
		return note
	}
	req := StartRequest{Workspace: workspace, Branch: BranchFor(*next), Agent: agent, Model: a.Model, Args: a.Args, Queued: true}
	if err := q.Start(a.Machine, *next, req); err != nil {
		q.refuse(*next, err)
		note.Reason = fmt.Sprintf("%s could not start: %s", next.Identifier, err)
		note.Next = ""
		return note
	}
	note.State, note.Reason, note.Working = "started", "started "+next.Identifier, working+1
	return note
}

// refuse notes on the card why the queue could not start it, so it is not tried again before the person moves it.
func (q *Queue) refuse(t Task, err error) {
	reason := err.Error()
	if _, e := q.Store.db.Exec(`UPDATE tasks SET queue_refused = ? WHERE id = ?`, reason, t.ID); e != nil {
		return
	}
	_, _ = q.Store.AddComment(t.ID, "remuda", "The queue could not start an agent on it: "+reason+". Move the card out of Queued and back to try again.")
	if q.Notify != nil {
		q.Notify(push.Payload{
			Kind:  "task",
			Title: fmt.Sprintf("Task %s: the queue could not start it", t.Identifier),
			Body:  firstLine(t.Title) + " — " + firstLine(reason),
			Tag:   fmt.Sprintf("task|%d", t.ID),
		})
	}
}

// QueueColumn says whether a column of the project is its Queued one.
func (st *Store) QueueColumn(projectID, statusID int64) bool {
	var on bool
	err := st.db.QueryRow(`SELECT on_queue FROM statuses WHERE id = ? AND project_id = ?`, statusID, projectID).Scan(&on)
	return err == nil && on
}

// nextQueued is the top open card of the queued column that the queue may start: never refused since it last moved
// and with no agent starting or running on it.
func (st *Store) nextQueued(projectID, statusID int64) (*Task, error) {
	var id int64
	err := st.db.QueryRow(`SELECT id FROM tasks WHERE project_id = ? AND status_id = ? AND done = 0 AND queue_refused = ''
  AND NOT EXISTS (SELECT 1 FROM task_runs r WHERE r.task_id = tasks.id AND r.state IN (?, ?))
  ORDER BY position, id LIMIT 1`, projectID, statusID, RunStarting, RunRunning).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t, err := st.Get(id)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// queueWorking counts the project's tasks whose agent the queue started and that still work or ask: a start under
// way, or a pane the radar reads as working, blocked or not known yet; a pipeline's stage that passed counts once,
// through the stage after it.
func (st *Store) queueWorking(projectID int64, state func(machine, pane string) (string, bool)) (int, error) {
	rows, err := st.db.Query(`SELECT r.task_id, r.machine, r.pane, r.state FROM task_runs r JOIN tasks t ON t.id = r.task_id
  WHERE t.project_id = ? AND r.by_queue = 1 AND r.state IN (?, ?, ?) AND r.pipeline <> ?`, projectID, RunStarting, RunRunning, RunFinished, PipelinePassed)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	busy := map[int64]bool{}
	for rows.Next() {
		var task int64
		var machine, pane, run string
		if err := rows.Scan(&task, &machine, &pane, &run); err != nil {
			return 0, err
		}
		if run == RunStarting {
			busy[task] = true
			continue
		}
		if s, ok := state(machine, pane); ok && (s == "working" || s == "blocked" || s == "unknown") {
			busy[task] = true
		}
	}
	return len(busy), rows.Err()
}

var branchJunk = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// BranchFor is the branch a task's worktree gets, as the page's Start names it: "task-12-fix-the-login-page".
func BranchFor(t Task) string {
	slug := strings.Trim(branchJunk.ReplaceAllString(strings.ToLower(t.Title), "-"), "-")
	if rs := []rune(slug); len(rs) > 40 {
		slug = strings.TrimRight(string(rs[:40]), "-")
	}
	if slug == "" {
		return fmt.Sprintf("task-%d", t.ID)
	}
	return fmt.Sprintf("task-%d-%s", t.ID, slug)
}

// Loop sweeps every so often.
func (q *Queue) Loop(every time.Duration) {
	for {
		q.Sweep()
		time.Sleep(every)
	}
}
