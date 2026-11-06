package tasks

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/watch"
	"github.com/dev1antdev/remuda/internal/worktrees"
)

// ErrAgentStart is a worktree that is there without the agent in it.
var ErrAgentStart = errors.New("the agent did not start")

// StartRequest is what the page asks when it starts an agent on a task.
type StartRequest struct {
	Workspace string `json:"workspace"`
	Branch    string `json:"branch"`
	Base      string `json:"base"`
	Agent     string `json:"agent"`
	Model     string `json:"model"` // as the agent's CLI names it, "" for its default
	Args      string `json:"args"`  // extra arguments as the person typed them
	Groom     bool   `json:"groom"`
	Prompt    string `json:"prompt"`
	Queued    bool   `json:"-"` // the project's queue starts it, not the person
}

// Launcher starts an agent on a task: a worktree off a workspace, the agent in it, the prompt handed over in the background.
type Launcher struct {
	Store  *Store
	Input  *sessions.Input
	Watch  *watch.Watcher
	Notify func(push.Payload)
}

// Normalize fills the agent in, trims the names and refuses what cannot start: an unknown agent, no workspace or branch, a branch git would misread.
func (req *StartRequest) Normalize() error {
	if req.Agent == "" {
		req.Agent = "claude"
	}
	if req.Agent == "shell" || !sessions.KnownAgent(req.Agent) {
		return invalid(fmt.Sprintf("an agent is required, not %q", req.Agent))
	}
	req.Model, req.Args = strings.TrimSpace(req.Model), strings.TrimSpace(req.Args)
	if err := checkStart(req.Agent, req.Model, req.Args); err != nil {
		return invalid(err.Error())
	}
	req.Branch, req.Base = strings.TrimSpace(req.Branch), strings.TrimSpace(req.Base)
	if req.Workspace == "" || req.Branch == "" {
		return invalid("a workspace and a branch are required")
	}
	if !sessions.RefOK(req.Branch) || (req.Base != "" && !sessions.RefOK(req.Base)) {
		return invalid("branch names: no leading dash, no spaces, no ..")
	}
	return nil
}

// Start makes the worktree and starts the agent, records the run and hands the prompt over in the background; the task comes back as it stands.
// Bad input is ErrInvalid, herdr's refusal of the worktree is herdr's error, an agent that did not start is ErrAgentStart.
func (l *Launcher) Start(mc *machines.Machine, client *herdr.Client, task Task, req StartRequest) (Task, error) {
	if err := req.Normalize(); err != nil {
		return Task{}, err
	}
	res, err := client.WorktreeCreate(req.Workspace, req.Branch, req.Base, "")
	if err != nil {
		return Task{}, err
	}
	included, includeErr := worktrees.Include(mc, res.Worktree.Path)
	pane := res.RootPane.PaneID
	args, _ := sessions.SplitArgs(req.Args)
	if err := sessions.StartAgentWith(client, pane, req.Agent, sessions.Start{Model: req.Model, Args: args}); err != nil {
		l.Watch.Refresh(mc)
		return Task{}, fmt.Errorf("the worktree %s is there, but %w: %v", res.Worktree.Branch, ErrAgentStart, err)
	}
	l.Watch.Refresh(mc)
	project, _ := l.Store.Project(task.ProjectID)
	run := Run{TaskID: task.ID, Machine: mc.ID(), Workspace: res.Workspace.WorkspaceID, Pane: pane, Dir: res.Worktree.Path, Branch: res.Worktree.Branch, Agent: req.Agent, ByQueue: req.Queued}
	if len(project.Automation.Stages) > 0 {
		run.Pipeline = PipelineOn
	}
	run, err = l.Store.StartRun(run)
	if err != nil {
		return Task{}, err
	}
	if note := includeNote(included, includeErr); note != "" {
		_, _ = l.Store.AddComment(task.ID, "remuda", note)
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		prompt = Prompt(task, project, req.Groom)
	} else if req.Groom && !strings.HasPrefix(prompt, "/grill-me") {
		prompt = "/grill-me " + prompt
	}
	if run.Pipeline == PipelineOn {
		prompt += "\n\n" + StageAsk
	}
	go l.deliver(mc, client, run, task, prompt)
	return l.Store.Get(task.ID)
}

// StartStage starts a pipeline's next stage: its agent in a new tab of the previous stage's worktree, the run
// recorded and the card moved to the stage's column, the prompt handed over in the background as a start's is.
func (l *Launcher) StartStage(mc *machines.Machine, client *herdr.Client, prev Run, index int, stage Stage, prompt string) (Run, error) {
	tab, err := client.TabCreate(prev.Workspace, stage.Name)
	if err != nil {
		return Run{}, err
	}
	pane := tab.RootPane.PaneID
	if err := sessions.StartAgentWith(client, pane, agentOr(stage.Agent), sessions.Start{Model: stage.Model}); err != nil {
		l.Watch.Refresh(mc)
		return Run{}, fmt.Errorf("%w: %v", ErrAgentStart, err)
	}
	l.Watch.Refresh(mc)
	run, err := l.Store.StartStageRun(prev, index, stage, pane)
	if err != nil {
		return Run{}, err
	}
	task, err := l.Store.Get(prev.TaskID)
	if err != nil {
		return Run{}, err
	}
	go l.deliver(mc, client, run, task, prompt)
	return run, nil
}

// includeNote is the task's comment on the files the new worktree got from .worktreeinclude, or why it got none
func includeNote(included worktrees.Included, err error) string {
	if err != nil {
		return "The files the repository's .worktreeinclude names were not copied into the new worktree: " + err.Error()
	}
	return included.Words()
}

// deliver waits for the fresh agent and hands it the task; Claude Code's "trust this folder"
// question for the new worktree is answered yes, any other question is pushed to the person and waited for.
func (l *Launcher) deliver(mc *machines.Machine, client *herdr.Client, run Run, task Task, prompt string) {
	lock := l.Input.Lock(mc.ID(), run.Pane)
	deadline := time.Now().Add(5 * time.Minute)
	nudged, trusted := false, false
	var last error
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		lock.Lock()
		err := client.AgentPrompt(run.Pane, prompt)
		lock.Unlock()
		switch {
		case err == nil:
			_ = l.Store.SetRun(run.ID, RunRunning, "")
			return
		case herdr.IsCode(err, "agent_not_ready") || herdr.IsCode(err, "agent_not_found"):
			last = err
		case herdr.IsCode(err, "agent_blocked"):
			last = err
			if !trusted {
				if screen, rerr := client.PaneRead(run.Pane); rerr == nil && isTrustPrompt(screen, run.Dir, mc.Home()) {
					trusted = true
					lock.Lock()
					if !cursorOnYes(screen) { // "No, exit" sits first and focused; older builds put Yes first
						_ = client.PaneSendKeys(run.Pane, []string{"down"})
						time.Sleep(150 * time.Millisecond)
					}
					_ = client.PaneSendKeys(run.Pane, []string{"enter"})
					lock.Unlock()
					_, _ = l.Store.AddComment(run.TaskID, "remuda", "Answered Claude Code's trust question for the new worktree `"+run.Dir+"`.")
					continue
				}
			}
			if !nudged && l.Notify != nil {
				nudged = true
				l.Notify(push.Payload{
					Machine: mc.ID(), Pane: run.Pane, Kind: "input",
					Title:   "Task " + task.Identifier + ": the agent asks something first",
					Body:    "Answer it in the pane and the task follows.",
					Tag:     fmt.Sprintf("task|%d", run.TaskID),
					Actions: []push.Action{{Action: "open", Title: "Open"}},
				})
			}
		default:
			l.fail(run, "The task could not be handed to the agent: "+err.Error())
			return
		}
	}
	msg := "the agent did not take the prompt within five minutes"
	if last != nil {
		msg += " (" + last.Error() + ")"
	}
	l.fail(run, "The task could not be handed to the agent: "+msg+". The worktree is there; send it by hand.")
}

// fail closes a starting run as failed, unless the sweep closed it meanwhile.
func (l *Launcher) fail(run Run, note string) {
	if cur, err := l.Store.Run(run.ID); err != nil || cur.State != RunStarting {
		return
	}
	_ = l.Store.SetRun(run.ID, RunFailed, note)
	_, _ = l.Store.AddComment(run.TaskID, "remuda", note)
	_ = l.Store.StopPipeline(run.ID, "the prompt never reached the agent")
}

// isTrustPrompt recognises Claude Code's "do you trust this folder" screen for the given folder, nothing else.
// The dialog is boxed and wrapped at the pane's width, so whitespace and box runes are dropped; the path may start with ~.
func isTrustPrompt(screen, dir, home string) bool {
	flat := squash(screen)
	if !strings.Contains(flat, "trustthisfolder") || !strings.Contains(flat, "Accessingworkspace") {
		return false
	}
	if dir == "" {
		return true
	}
	want := []string{squash(dir)}
	if home != "" && (dir == home || strings.HasPrefix(dir, home+"/")) {
		want = append(want, squash("~"+strings.TrimPrefix(dir, home)))
	}
	for _, w := range want {
		if strings.Contains(flat, w) {
			return true
		}
	}
	return false
}

// cursorOnYes says whether the dialog's cursor already sits on the yes line.
func cursorOnYes(screen string) bool {
	for _, line := range strings.Split(screen, "\n") {
		if strings.Contains(line, "❯") && strings.Contains(line, "Yes, I trust") {
			return true
		}
	}
	return false
}

// squash drops whitespace and box-drawing runes.
func squash(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' || (r >= 0x2500 && r <= 0x257F) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
