package tasks

// The queue of a project that may use any machine: the next card goes to the connected machine with the most room
// among those holding a workspace on a checkout of the project's repository (Queue.Place, internal/placement); the
// note says where it would go and why, or why no machine has room. And the offer's run: a card the plan window held,
// started now on the other vendor's agent at the person's word.

import (
	"errors"
	"fmt"

	"github.com/dev1antdev/remuda/internal/placement"
)

// stepAnywhere is one look at a project whose queue picks the machine: the slots first, then the place.
func (q *Queue) stepAnywhere(p Project, next Task, agent string, note QueueNote) QueueNote {
	a := p.Automation
	res := q.Place(a, agent)
	note.Fleet = &res
	if note.Working >= a.Max {
		note.Reason = fmt.Sprintf("every slot is taken: %d of %d working", note.Working, a.Max)
		return note
	}
	if res.Pick == nil {
		note.Reason = placement.NoRoom(res)
		if res.PlanOnly() && q.Offer != nil {
			q.Offer(p, next, "", agent, res.Others[0].Held)
		}
		return note
	}
	pick := res.Pick
	req := StartRequest{Workspace: pick.Workspace, Branch: BranchFor(next), Agent: agent, Model: a.Model, Args: a.Args, Queued: true, DenyPrompts: a.Denies(agent)}
	if err := q.Start(pick.ID, next, req); err != nil {
		q.refuse(next, err)
		note.Reason = fmt.Sprintf("%s could not start on %s: %s", next.Identifier, pick.Label, err)
		note.Next = ""
		return note
	}
	note.State, note.Reason, note.Working = "started", "started "+next.Identifier+" on "+pick.Label, note.Working+1
	return note
}

// RunOffered starts a card an offer named on the agent kind given, now, as the queue would have but for the plan
// window — on the project's machine, or the one with room for that agent — with the model given ("" for the agent's
// default): the card must still wait in the Queued column with no agent on it. Nothing saved on the project changes.
func (q *Queue) RunOffered(projectID, taskID int64, agent, model string) (string, error) {
	p, err := q.Store.Project(projectID)
	if err != nil {
		return "", err
	}
	t, err := q.Store.Get(taskID)
	if err != nil {
		return "", err
	}
	column := flagged(p.Statuses, func(s Status) bool { return s.OnQueue })
	if column == nil || t.ProjectID != projectID || t.StatusID != column.ID || t.Done {
		return "", fmt.Errorf("%s no longer waits in the Queued column", t.Identifier)
	}
	if t.Run != nil && (t.Run.State == RunStarting || t.Run.State == RunRunning) {
		return "", fmt.Errorf("an agent already works on %s", t.Identifier)
	}
	a := p.Automation
	machine, label, workspace := a.Machine, a.Machine, ""
	if a.Anywhere && q.Place != nil {
		res := q.Place(a, agent)
		if res.Pick == nil {
			return "", errors.New(placement.NoRoom(res))
		}
		machine, label, workspace = res.Pick.ID, res.Pick.Label, res.Pick.Workspace
	} else {
		var connected bool
		if label, connected = q.Machine(machine); !connected {
			return "", fmt.Errorf("%s is away", label)
		}
		if why := q.Spent(machine, agent); why != "" {
			return "", errors.New(why)
		}
		if workspace = q.Workspace(machine, a.Dir); workspace == "" {
			return "", fmt.Errorf("no workspace is open on %s to branch off", a.Dir)
		}
	}
	req := StartRequest{Workspace: workspace, Branch: BranchFor(t), Agent: agent, Model: model, Queued: true, DenyPrompts: a.Denies(agent)}
	if err := q.Start(machine, t, req); err != nil {
		return "", err
	}
	return fmt.Sprintf("started %s on %s with %s", t.Identifier, label, agentLabel(agent)), nil
}
