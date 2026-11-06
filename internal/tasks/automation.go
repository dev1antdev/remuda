package tasks

// A project's agents: where they work (a repository folder on a machine), which agent with which model and arguments,
// the queue that starts the tasks put in its Queued column by themselves while its switch is on (queue.go), and the
// pipeline's stages after the first agent's work (pipeline.go). Kept as one JSON column on the project.

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/dev1antdev/remuda/internal/sessions"
)

// MaxStages is how many stages a pipeline may have after the first agent's work.
const MaxStages = 8

// MaxQueued is the most agents a project's queue may keep working at once.
const MaxQueued = 10

// Automation is how a project's agents work.
type Automation struct {
	Machine string  `json:"machine"`         // the machine they run on
	Dir     string  `json:"dir"`             // the repository folder their worktrees branch off
	Agent   string  `json:"agent"`           // the agent kind, "" for Claude Code
	Model   string  `json:"model,omitempty"` // as the agent's CLI names it, "" for its default
	Args    string  `json:"args,omitempty"`  // extra arguments as the person typed them
	Queue   bool    `json:"queue"`           // start the tasks in the Queued column by themselves
	Max     int     `json:"max"`             // at most this many agents the queue started still working or asking
	Stages  []Stage `json:"stages"`          // the pipeline after the first agent's work; none, no pipeline
}

// Stage is one step of the pipeline: an agent in a new tab of the same worktree with its prompt, the card in its column.
type Stage struct {
	Name     string `json:"name"`
	Agent    string `json:"agent"`
	Model    string `json:"model,omitempty"`
	Prompt   string `json:"prompt"`              // {task}, {previous} (the last stage's answer) and {branch} filled in
	StatusID int64  `json:"status_id,omitempty"` // the column the card moves to while it runs; 0 leaves it where it is
	PR       bool   `json:"pr,omitempty"`        // the stage also commits, pushes and opens a pull request
}

// agentOr is the agent kind a setting names, Claude Code when it names none.
func agentOr(kind string) string {
	if kind == "" {
		return "claude"
	}
	return kind
}

// readAutomation is the stored column: a project from before has none, and a damaged value reads as none too.
func readAutomation(raw string) Automation {
	var a Automation
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &a)
	}
	if a.Max < 1 {
		a.Max = 1
	}
	if a.Stages == nil {
		a.Stages = []Stage{}
	}
	return a
}

// checkStart says why an agent kind with this model and these arguments cannot start, or nil.
func checkStart(kind, model, args string) error {
	if kind == "shell" || !sessions.KnownAgent(kind) {
		return fmt.Errorf("an agent is needed, not %q", kind)
	}
	split, err := sessions.SplitArgs(args)
	if err != nil {
		return err
	}
	_, err = sessions.StartArgs(kind, sessions.Start{Model: strings.TrimSpace(model), Args: split})
	return err
}

// normalize trims the settings and refuses what could not start: an agent herdr does not know, a model or arguments
// its command line cannot take, a folder that is not absolute, a queue with nowhere to work, a stage with no prompt
// or a column of another project.
func (a *Automation) normalize(statuses []Status) error {
	a.Machine, a.Dir = strings.TrimSpace(a.Machine), strings.TrimSpace(a.Dir)
	a.Agent, a.Model, a.Args = strings.TrimSpace(a.Agent), strings.TrimSpace(a.Model), strings.TrimSpace(a.Args)
	if a.Dir != "" {
		if !path.IsAbs(a.Dir) {
			return invalid("the repository folder must be an absolute path")
		}
		a.Dir = path.Clean(a.Dir)
	}
	if (a.Machine == "") != (a.Dir == "") {
		return invalid("where agents work needs both the machine and the repository folder")
	}
	if err := checkStart(agentOr(a.Agent), a.Model, a.Args); err != nil {
		return invalid("the project's agent: " + err.Error())
	}
	if a.Max < 1 {
		a.Max = 1
	}
	if a.Max > MaxQueued {
		return invalid(fmt.Sprintf("the queue keeps at most %d agents working at once", MaxQueued))
	}
	if a.Queue {
		if a.Machine == "" {
			return invalid("the queue needs to know where agents work: a machine and a repository folder")
		}
		if flagged(statuses, func(s Status) bool { return s.OnQueue }) == nil {
			return invalid("the queue needs a column marked Queued")
		}
	}
	if len(a.Stages) > MaxStages {
		return invalid(fmt.Sprintf("a pipeline has at most %d stages", MaxStages))
	}
	if a.Stages == nil {
		a.Stages = []Stage{}
	}
	for i := range a.Stages {
		s := &a.Stages[i]
		s.Name, s.Agent, s.Model, s.Prompt = strings.TrimSpace(s.Name), strings.TrimSpace(s.Agent), strings.TrimSpace(s.Model), strings.TrimSpace(s.Prompt)
		if s.Name == "" {
			s.Name = fmt.Sprintf("Stage %d", i+1)
		}
		if s.Prompt == "" && !s.PR {
			return invalid(fmt.Sprintf("stage %q needs a prompt", s.Name))
		}
		if err := checkStart(agentOr(s.Agent), s.Model, ""); err != nil {
			return invalid(fmt.Sprintf("stage %q: %s", s.Name, err))
		}
		if s.StatusID != 0 {
			column := statusByID(statuses, s.StatusID)
			if column == nil {
				return invalid(fmt.Sprintf("stage %q moves the card to a column this project does not have", s.Name))
			}
			if column.OnQueue {
				return invalid(fmt.Sprintf("stage %q moves the card to the Queued column, which only you fill", s.Name))
			}
		}
	}
	return nil
}

// SetAutomation replaces how a project's agents work, checked against its columns.
func (st *Store) SetAutomation(projectID int64, a Automation) (Project, error) {
	ss, err := st.statusesOf(projectID)
	if err != nil {
		return Project{}, err
	}
	if _, err := st.Project(projectID); err != nil {
		return Project{}, err
	}
	if err := a.normalize(ss); err != nil {
		return Project{}, err
	}
	raw, err := json.Marshal(a)
	if err != nil {
		return Project{}, err
	}
	if _, err := st.db.Exec(`UPDATE projects SET automation = ?, updated = ? WHERE id = ?`, string(raw), st.stamp(), projectID); err != nil {
		return Project{}, err
	}
	return st.Project(projectID)
}
