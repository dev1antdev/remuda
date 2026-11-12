package app

import (
	"time"

	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/recipes"
	"github.com/dev1antdev/remuda/internal/retire"
	"github.com/dev1antdev/remuda/internal/schedule"
	"github.com/dev1antdev/remuda/internal/signin"
	"github.com/dev1antdev/remuda/internal/tasks"
	"github.com/dev1antdev/remuda/internal/usage"
	"github.com/dev1antdev/remuda/internal/watch"
)

// retirements is what the roster says is set to a model or mode being retired, with its view of the world: the
// recipes, the agent schedules and the projects' agents, every agent pane's meter, and Claude Code's defaults on each
// machine of the fleet as its vitals read them.
func retirements(cookbook *recipes.Store, schedules *schedule.Store, store *tasks.Store, fleet *machines.Manager, watcher *watch.Watcher, meter *usage.Tracker, vitals *health.Collector) *retire.Service {
	return &retire.Service{
		Settings: func() []retire.Setting {
			settings := append(retire.Recipes(cookbook.List()), retire.Schedules(schedules.List())...)
			if projects, err := store.Projects(); err == nil {
				settings = append(settings, retire.Projects(projects)...)
			}
			return settings
		},
		Panes: func() []retire.Pane {
			var panes []retire.Pane
			for _, mc := range fleet.List() {
				snap, _ := watcher.Snapshot(mc.ID())
				if snap == nil {
					continue
				}
				for _, pane := range snap.Panes {
					if pane.Agent == "" {
						continue
					}
					if reading, ok := meter.Display(mc.ID(), pane.PaneID); ok {
						panes = append(panes, retire.Pane{Machine: mc.ID(), Pane: pane.PaneID, Agent: pane.Agent, Model: reading.Model, Speed: reading.Speed})
					}
				}
			}
			return panes
		},
		Defaults: func() map[string]retire.Defaults {
			defaults := map[string]retire.Defaults{}
			for _, mc := range fleet.List() {
				if r := vitals.Report(mc.ID()); r != nil && r.Claude != "" {
					defaults[mc.ID()] = retire.Defaults{Model: r.ClaudeModel, Fast: r.ClaudeFast}
				}
			}
			return defaults
		},
	}
}

// signinWarnings hears every machine's vitals for when Claude Code's sign-in there ends; the host of a remuda serving
// the page only runs no agents and is never warned of.
func signinWarnings(signins *signin.Service, fleet *machines.Manager) func(mc *machines.Machine, r health.Report) {
	return func(mc *machines.Machine, r health.Report) {
		if mc.Local() && fleet.Server() {
			return
		}
		var expires time.Time
		if r.ClaudeLogin != nil {
			expires = r.ClaudeLogin.Expires
		}
		signins.Observe(mc.ID(), mc.Label(), expires)
	}
}
