package app

import (
	"errors"
	"fmt"

	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/limits"
	"github.com/dev1antdev/remuda/internal/recipes"
	"github.com/dev1antdev/remuda/internal/schedule"
	"github.com/dev1antdev/remuda/internal/tasks"
)

// planOffers turns a start the plan window held — a queued card, an agent schedule's run — into the person's offer to
// run it on the other vendor's agent, when that agent is on the machine and its own windows hold nothing; and runs a
// taken offer once, on the model a recipe of that agent names for the folder, else its default. Nothing saved changes.
type planOffers struct {
	limits    *limits.Service
	vitals    *health.Collector
	recipes   *recipes.Store
	queue     *tasks.Queue
	schedules *schedule.Store
}

// other is the vendor that could run what the window holds for agent, "" when none can: not installed on the machine
// (any machine, for a project whose queue picks one), or held by its own window.
func (o *planOffers) other(machine, agent string) string {
	to := limits.Other(agent)
	if to == "" {
		return ""
	}
	if machine != "" {
		if r := o.vitals.Report(machine); r != nil && !installed(r, to) {
			return ""
		}
	}
	if _, held := o.limits.Holding(machine, to); held {
		return ""
	}
	return to
}

func installed(r *health.Report, agent string) bool {
	switch agent {
	case "claude":
		return r.Claude != ""
	case "codex":
		return r.Codex != ""
	}
	return false
}

// model is the model a recipe of the agent names — one for the folder first, then one for any folder — "" for none.
func (o *planOffers) model(agent, machine, dir string) string {
	var any string
	found := false
	for _, r := range o.recipes.List() {
		if r.Agent != agent {
			continue
		}
		if r.Dir != "" && r.Dir == dir && (machine == "" || r.Machine == "" || r.Machine == machine) {
			return r.Model
		}
		if r.Dir == "" && !found {
			any, found = r.Model, true
		}
	}
	return any
}

// Task offers a queued card the window holds; machine is "" for a project whose queue picks the machine.
func (o *planOffers) Task(p tasks.Project, t tasks.Task, machine, agent, why string) {
	to := o.other(machine, agent)
	at := machine
	if at == "" {
		at = p.Automation.Machine
	}
	hold, held := o.limits.Holding(at, agent)
	if to == "" || !held {
		return
	}
	offer := hold.Offer(limits.Offer{
		Kind: limits.OfferTask, Project: p.ID, Task: t.ID, Ref: t.Identifier, Title: t.Title, Machine: machine,
		From: agent, To: to, Model: o.model(to, at, p.Automation.Dir),
	})
	offer.Why = why // the queue's own words: a fleet's are the machine's that held it
	o.limits.Offer(offer)
}

// Schedule offers an agent schedule's run the window held.
func (o *planOffers) Schedule(s schedule.Schedule, why string) {
	to := o.other(s.Machine, s.Agent)
	hold, held := o.limits.Holding(s.Machine, s.Agent)
	if to == "" || !held {
		return
	}
	offer := hold.Offer(limits.Offer{
		Kind: limits.OfferSchedule, Schedule: s.ID, Title: s.Label, Machine: s.Machine,
		From: s.Agent, To: to, Model: o.model(to, s.Machine, s.Dir),
	})
	offer.Why = why
	o.limits.Offer(offer)
}

// Hold is why an agent schedule's run waits on its plan window, "" when it does not.
func (o *planOffers) Hold(s schedule.Schedule) string {
	if hold, held := o.limits.Holding(s.Machine, s.Agent); held {
		return hold.Why
	}
	return ""
}

// Run runs a taken offer: the card on the other vendor's agent through the queue, or the schedule once with it.
func (o *planOffers) Run(off limits.Offer) (string, error) {
	switch off.Kind {
	case limits.OfferTask:
		return o.queue.RunOffered(off.Project, off.Task, off.To, off.Model)
	case limits.OfferSchedule:
		ran, err := o.schedules.RunAs(off.Schedule, schedule.Swap{Agent: off.To, Model: off.Model})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("“%s” %s", ran.Label, ran.LastResult), nil
	}
	return "", errors.New("an offer of no known kind")
}
