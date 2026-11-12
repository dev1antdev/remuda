package app

import (
	"errors"

	"github.com/dev1antdev/remuda/internal/feed"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/radar"
	"github.com/dev1antdev/remuda/internal/recipes"
	"github.com/dev1antdev/remuda/internal/scenes"
	"github.com/dev1antdev/remuda/internal/schedule"
	"github.com/dev1antdev/remuda/internal/scratch"
	"github.com/dev1antdev/remuda/internal/sent"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/usage"
	"github.com/dev1antdev/remuda/internal/watch"
	"github.com/dev1antdev/remuda/internal/worktrees"
)

// sceneMachine is a machine of the fleet as a scene sees it: its herdr, the watcher's snapshot, its worktrees.
type sceneMachine struct {
	mc    *machines.Machine
	watch *watch.Watcher
}

func (m sceneMachine) ID() string    { return m.mc.ID() }
func (m sceneMachine) Label() string { return m.mc.Label() }

func (m sceneMachine) Herdr() (scenes.Herdr, error) {
	client, _, err := m.mc.Ready()
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (m sceneMachine) Snapshot() *herdr.Snapshot {
	snap, _ := m.watch.Snapshot(m.mc.ID())
	return snap
}

func (m sceneMachine) Refresh() { m.watch.Refresh(m.mc) }

func (m sceneMachine) Leftovers(open []string) ([]worktrees.Leftover, error) {
	return worktrees.Leftovers(m.mc, open)
}

func (m sceneMachine) Tidy(folder string) worktrees.Outcome {
	return worktrees.Tidy(m.mc, folder, false)
}

func (m sceneMachine) Folder(dir string) bool {
	info, err := m.mc.ResolvePath("", dir)
	return err == nil && info.Kind == "dir"
}

// sceneSchedules is a scene schedule's way to its scene: a start scene's every step, a tidy's whole plan only while
// nobody's agent on the machines it covers works or asks, and a run already going waited for.
func sceneSchedules(runner *scenes.Service) schedule.Scenes {
	return schedule.Scenes{
		Name: func(id string) (string, bool) {
			scene, ok := runner.Store.Get(id)
			return scene.Name, ok
		},
		Run: func(id, by string) (string, string, error) {
			scene, ok := runner.Store.Get(id)
			if !ok {
				return "", "", errors.New("the scene it runs is gone")
			}
			if scene.Kind == scenes.KindTidy {
				if machine := runner.AtWork(*scene.Tidy); machine != "" {
					return "an agent works or asks on " + machine, schedule.SkipAtWork, nil
				}
			}
			run, err := runner.RunNow(id, by)
			if errors.Is(err, scenes.ErrBusy) {
				return "the scene is running already", schedule.SkipRunning, nil
			}
			if err != nil {
				return "", "", err
			}
			line, err := scenes.Summary(run)
			return line, "", err
		},
	}
}

// sceneRunner is the scenes over the fleet: the recipes they name, each pane's radar and meter readings, the floating
// terminal's shells left alone, a first prompt not delivered pushed, a /compact a quiet row of the activity.
func sceneRunner(dir string, cookbook *recipes.Store, fleet *machines.Manager, watcher *watch.Watcher, input *sessions.Input, tracker *radar.Tracker, meter *usage.Tracker, shells *scratch.Store, notify func(push.Payload), activity *feed.Service) *scenes.Service {
	recipe := func(id string) (recipes.Recipe, bool) {
		for _, r := range cookbook.List() {
			if r.ID == id {
				return r, true
			}
		}
		return recipes.Recipe{}, false
	}
	store := scenes.New(dir)
	store.Recipe = func(id string) bool {
		_, ok := recipe(id)
		return ok
	}
	return &scenes.Service{
		Store:  store,
		Recipe: recipe,
		Machines: func() []scenes.Machine {
			var out []scenes.Machine
			for _, mc := range fleet.List() {
				out = append(out, sceneMachine{mc: mc, watch: watcher})
			}
			return out
		},
		Machine: func(id string) (scenes.Machine, error) {
			mc, err := fleet.Get(id)
			if err != nil {
				return nil, err
			}
			return sceneMachine{mc: mc, watch: watcher}, nil
		},
		Readings: scenes.Readings{
			State: tracker.Display,
			Context: func(machine, pane string) (int, bool) {
				reading, ok := meter.Display(machine, pane)
				if !ok || reading.Window <= 0 {
					return 0, false
				}
				return int(reading.Context * 100 / int64(reading.Window)), true
			},
			Scratch: shells.Is,
		},
		Input:  input,
		Notify: notify,
		Sent: func(by, machine, pane, text string) {
			workspace := ""
			if snap, _ := watcher.Snapshot(machine); snap != nil {
				if info := snap.Pane(pane); info != nil {
					workspace = snap.WorkspaceLabel(info.WorkspaceID)
				}
			}
			activity.Add(sent.Entry(by, text, false, machine, pane, workspace))
		},
	}
}
