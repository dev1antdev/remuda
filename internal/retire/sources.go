package retire

import (
	"strconv"

	"github.com/dev1antdev/remuda/internal/recipes"
	"github.com/dev1antdev/remuda/internal/schedule"
	"github.com/dev1antdev/remuda/internal/tasks"
)

// Recipes are the recipes that start an agent, each on its own machine or on any.
func Recipes(list []recipes.Recipe) []Setting {
	var out []Setting
	for _, r := range list {
		if r.Agent == "" || r.Agent == "shell" {
			continue
		}
		out = append(out, Setting{Kind: "recipe", ID: r.ID, Machine: r.Machine, Agent: r.Agent, Model: r.Model, Args: r.Args})
	}
	return out
}

// Schedules are the agent schedules switched on: a scout starts its agent at every run.
func Schedules(list []schedule.Schedule) []Setting {
	var out []Setting
	for _, s := range list {
		if s.Kind != schedule.KindAgent || !s.Enabled {
			continue
		}
		out = append(out, Setting{Kind: "schedule", ID: s.ID, Machine: s.Machine, Agent: s.Agent, Model: s.Model, Args: s.Args})
	}
	return out
}

// Projects are the agents of every project that is not archived and set one up, and its pipeline's stages, which
// start on the project's machine with a model of their own and no arguments.
func Projects(list []tasks.Project) []Setting {
	var out []Setting
	for _, p := range list {
		a := p.Automation
		if p.Archived || (a.Machine == "" && a.Agent == "" && a.Model == "" && a.Args == "" && len(a.Stages) == 0) {
			continue
		}
		id := strconv.FormatInt(p.ID, 10)
		out = append(out, Setting{Kind: "project", ID: id, Machine: a.Machine, Agent: a.Agent, Model: a.Model, Args: a.Args})
		for i, stage := range a.Stages {
			out = append(out, Setting{Kind: "stage", ID: id + "/" + strconv.Itoa(i+1), Machine: a.Machine, Agent: stage.Agent, Model: stage.Model})
		}
	}
	return out
}
