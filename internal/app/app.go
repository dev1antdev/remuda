// Package app is remuda's composition root: it builds every service from
// the state dir and the flags, lists who hears which event, and serves
// until the context ends. Nothing here does work of its own.
package app

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/budget"
	"github.com/dev1antdev/remuda/internal/chat"
	"github.com/dev1antdev/remuda/internal/code"
	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/feed"
	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/jobs"
	"github.com/dev1antdev/remuda/internal/live"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/prefs"
	"github.com/dev1antdev/remuda/internal/prstatus"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/quick"
	"github.com/dev1antdev/remuda/internal/radar"
	"github.com/dev1antdev/remuda/internal/recap"
	"github.com/dev1antdev/remuda/internal/restart"
	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/said"
	"github.com/dev1antdev/remuda/internal/schedule"
	"github.com/dev1antdev/remuda/internal/server"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/setup"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/tasks"
	"github.com/dev1antdev/remuda/internal/tsnetx"
	"github.com/dev1antdev/remuda/internal/updates"
	"github.com/dev1antdev/remuda/internal/usage"
	"github.com/dev1antdev/remuda/internal/watch"
	"github.com/dev1antdev/remuda/internal/watchdog"
	"github.com/dev1antdev/remuda/web"
)

// Options is what the server needs to know: the flags, the state dir and the token main already has.
type Options struct {
	Dir           string // the state dir
	Token         string
	ShowSignIn    bool // print the sign-in link: a first run, or a person at the terminal
	Addr          string
	HerdrBin      string
	Socket        string
	CodePort      int
	Tailscale     bool
	TailscaleName string
	AuthKey       string // TS_AUTHKEY
	VersionCheck  bool
	Version       string
	Door          bool // place the agents' door files here and on every remote (off for a scratch instance)
}

// Run builds remuda and serves until ctx ends.
func Run(ctx context.Context, o Options) error {
	bin, err := herdr.FindBinary(o.HerdrBin)
	if err != nil {
		return fmt.Errorf("herdr: %w", err)
	}
	if err := herdr.EnsureServer(bin, o.Socket); err != nil {
		log.Printf("WARNING: herdr is not answering at %s (%v); remuda will keep trying", o.Socket, err)
	} else if version, protocol, err := herdr.NewClient(o.Socket).Ping(); err == nil {
		log.Printf("connected to herdr %s (protocol %d) at %s", version, protocol, o.Socket)
	}
	ui, err := web.Assets()
	if err != nil {
		return fmt.Errorf("ui assets: %w", err)
	}

	bus := events.NewBus()
	keys, err := auth.OpenKeys(o.Dir)
	if err != nil {
		return err
	}
	// the door: every machine's agents reach the task list here with a tasks-only key, the machine's own when it still passes
	var door *machines.Door
	if o.Door {
		door = &machines.Door{LocalPort: auth.PortOf(o.Addr), Key: func(name, have string) (string, error) {
			if _, ok := keys.Check(have); ok {
				return have, nil
			}
			key, _, err := keys.Mint(name)
			return key, err
		}}
	} else {
		log.Print("the agents' door stays closed: no agent-key / agent-url files are placed here or on a remote machine (-door opens it)")
	}
	fleet := machines.NewManager(o.Dir, o.Socket, bin, door)
	defer fleet.Close()
	if local, err := fleet.Get(machines.LocalID); err == nil && o.Door {
		if err := auth.EnsureDoor(o.Dir, keys, local.Label(), auth.LoopbackURL(o.Addr)); err != nil {
			log.Printf("WARNING: %v; the agents on this machine cannot reach the tasks", err)
		}
	}
	pushSvc, err := push.New(o.Dir)
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}
	devices, err := auth.OpenDevices(o.Dir)
	if err != nil {
		return err
	}
	activity := feed.New(o.Dir)
	taskStore, err := tasks.Open(o.Dir)
	if err != nil {
		return fmt.Errorf("tasks: %w", err)
	}
	defer taskStore.Close()

	watcher := watch.New(fleet, bus)
	chats := &chat.Service{Machines: fleet, Watch: watcher}
	var srv *server.Server // named here: the live hub fingerprints the roster as the server serves it
	hub := live.New(live.Inputs{
		Fingerprint: func() string { return srv.RosterETag() },
		Version:     chats.ConversationVersion,
		Remote:      func(id string) bool { mc, err := fleet.Get(id); return err == nil && !mc.Local() },
	})
	defer hub.Close()
	pushSvc.OnAlert = func(p push.Payload) {
		activity.Add(feed.Entry{Kind: p.Kind, Machine: p.Machine, Pane: p.Pane, Title: p.Title, Body: p.Body})
		hub.RosterChanged() // the bell's dot rides on the roster
	}
	tracker := radar.New(o.Dir, chats.LastWriteOf)
	defer tracker.Flush()
	sayings := said.New(said.Inputs{Version: chats.SessionVersion, Answer: chats.LastAnswer, Link: chats.PullRequest}, bus)
	requests := prstatus.New(func(machineID string) (shell.Shell, bool) {
		mc, err := fleet.Get(machineID)
		if err != nil {
			return nil, false
		}
		return mc, true
	})
	reviews := review.New(o.Dir)
	meter := usage.NewTracker()
	budgets := budget.New(o.Dir, func() []shell.Host {
		var hosts []shell.Host
		for _, mc := range fleet.List() {
			if state, _ := mc.State(); state == machines.StateConnected {
				hosts = append(hosts, mc)
			}
		}
		return hosts
	}, pushSvc.Notify)
	input := sessions.NewInput(fleet, watcher)
	health.PinnedCode = setup.CodeVersion
	vitals := health.New(fleet, o.Version, o.VersionCheck)
	runner := jobs.New(bus)
	restarter := restart.New(fleet)
	restarter.Later = func(mc *machines.Machine, title string, fn func(say func(string)) error) {
		runner.Run(mc, title, "", fn)
	}
	restarter.Lock = input.Lock
	keeper := &herdr.Keeper{Bin: bin, Socket: o.Socket}
	recaps := &recap.Service{Machines: fleet, Watch: watcher, Chat: chats, Radar: tracker}
	schedules := schedule.New(o.Dir, input.SubmitToFolder, pushSvc.Broadcast, func(since time.Time) (string, string, bool) {
		return recap.Digest(recaps.Standup(since), since, time.Now())
	})
	scouts := &sessions.Launcher{Machines: fleet, Watch: watcher, Input: input, Notify: pushSvc.Broadcast}
	schedules.Agents = schedule.Agents{
		Start: func(s schedule.Schedule) (schedule.Started, error) {
			args, err := sessions.SplitArgs(s.Args)
			if err != nil {
				return schedule.Started{}, err
			}
			place, err := scouts.StartInFolder(s.Machine, s.Dir, s.Label, s.Agent, sessions.Start{Model: s.Model, Args: args}, s.Prompt)
			return schedule.Started{Pane: place.Pane, Tab: place.Tab, Workspace: place.Workspace, MadeWorkspace: place.MadeWorkspace}, err
		},
		Close: func(run schedule.Started) error {
			return scouts.Close(run.Machine, sessions.Place{Pane: run.Pane, Tab: run.Tab, Workspace: run.Workspace, MadeWorkspace: run.MadeWorkspace})
		},
		Answer: func(machine, pane string) string {
			answer, _ := chats.LastAnswer(machine, pane)
			return answer
		},
	}

	// who hears what
	events.Subscribe(bus, func(e events.Snapshot) { tracker.Observe(e.Machine, e.Snap, e.At) })
	events.Subscribe(bus, func(e events.Snapshot) {
		if mc, err := fleet.Get(e.Machine); err == nil {
			reviews.RefreshBadges(mc, e.Snap.WorkspaceDirs())
			meter.ObserveSnapshot(mc.ID(), mc.Transcripts, e.Snap)
		}
	})
	events.Subscribe(bus, func(e events.Snapshot) { sayings.Observe(e.Machine, e.Snap) })
	events.Subscribe(bus, func(e events.Snapshot) { hub.RosterChanged() })
	events.Subscribe(bus, func(e events.MachineRemoved) {
		tracker.Forget(e.Machine)
		sayings.Forget(e.Machine)
		hub.RosterChanged()
		requests.Forget(e.Machine)
	})
	events.Subscribe(bus, pushSvc.OnStatus)
	events.Subscribe(bus, sayings.OnStatus)
	events.Subscribe(bus, requests.OnNamed)
	events.Subscribe(bus, requests.OnStatus)
	events.Subscribe(bus, restarter.OnStatus)
	events.Subscribe(bus, schedules.OnStatus)
	events.Subscribe(bus, func(e events.PaneStatus) { hub.PaneChanged(e.Pane.Machine, e.Pane.Pane) })
	events.Subscribe(bus, func(e events.JobDone) {
		if mc, err := fleet.Get(e.Machine); err == nil {
			vitals.Refresh(mc)
		}
		hub.RosterChanged()
	})
	events.Subscribe(bus, func(e events.LocalHerdrDown) { keeper.Revive(e.Err) })

	// the loops
	watcher.Start()
	vitals.Start(15 * time.Minute)
	go watchdog.New(watchdog.Inputs{
		Episodes: tracker.Episodes,
		Pane: func(machineID, paneID string) (watchdog.PaneInfo, bool) {
			pane, snap := chats.Pane(machineID, paneID)
			if pane == nil {
				return watchdog.PaneInfo{}, false
			}
			return watchdog.PaneInfo{Title: pane.DisplayTitle(), Workspace: snap.WorkspaceLabel(pane.WorkspaceID), Agent: pane.AgentName()}, true
		},
		LastWrite: chats.LastWriteOfPane,
		LastTools: chats.LastToolCalls,
		Note:      tracker.SetAlert,
		Notify:    pushSvc.Notify,
	}).Run(time.Minute)
	runWatch := &tasks.RunWatch{
		Store: taskStore,
		PaneState: func(machineID, paneID string) (string, time.Time, bool) {
			pane, snap := chats.Pane(machineID, paneID)
			if snap == nil {
				return "unknown", time.Time{}, true // not seen yet (a restart, a reconnect): not gone
			}
			if pane == nil {
				return "", time.Time{}, false
			}
			display := tracker.Display(machineID, pane)
			return display.State, time.UnixMilli(display.Since), true
		},
		LastAnswer: chats.LastAnswer,
		Usage: func(machineID, paneID string) (float64, int64, string, bool) {
			d, ok := meter.Display(machineID, paneID)
			if !ok {
				return 0, 0, "", false
			}
			tokens := d.Tokens
			return d.Cost, tokens.Input + tokens.Output + tokens.CacheRead + tokens.CacheWrite5m + tokens.CacheWrite1h, d.Name, true
		},
		Notify: pushSvc.Notify,
	}
	go runWatch.Loop(15 * time.Second)
	events.Subscribe(bus, func(e events.PullRequestNamed) { runWatch.OnPullRequest(e.Pane.Machine, e.Pane.Pane, e.URL, e.Label) })
	go schedules.Loop()
	go hub.Run(ctx)
	go budgets.Loop(5 * time.Minute)

	srv = &server.Server{
		Machines:  fleet,
		Watch:     watcher,
		Input:     input,
		Chat:      chats,
		Updates:   &updates.Service{Health: vitals, Jobs: runner, Restart: restarter, Watch: watcher},
		Tasks:     taskStore,
		Launcher:  &tasks.Launcher{Store: taskStore, Input: input, Watch: watcher, Notify: pushSvc.Notify},
		Push:      pushSvc,
		Feed:      activity,
		Said:      sayings,
		PRs:       requests,
		Recap:     recaps,
		Radar:     tracker,
		Usage:     meter,
		Budgets:   budgets,
		Health:    vitals,
		Review:    reviews,
		Schedules: schedules,
		Quick:     quick.New(o.Dir),
		Prefs:     prefs.New(o.Dir),
		Code:      code.New(o.CodePort),
		Guard:     auth.NewGuard(o.Token, devices, keys),
		Keys:      keys,
		UI:        ui,
		Live:      hub,
	}
	if o.Tailscale {
		var node *tsnetx.Node
		srv.Tailscale = func() tsnetx.Status { return node.Status() } // set before the handler is handed out
		node = tsnetx.Start(o.Dir, o.TailscaleName, o.AuthKey, srv.Handler())
		defer node.Close()
	}
	log.Printf("remuda %s listening on http://%s", o.Version, o.Addr)
	// the sign-in link goes to a person at a terminal or into the first run's log, never the service's log at every restart
	if o.ShowSignIn {
		log.Printf("sign in with the token from `remuda -print-token`, or open:")
		log.Printf("  http://%s/?token=%s", o.Addr, o.Token)
	} else {
		log.Printf("sign in with the token from `remuda -print-token`")
	}
	return server.ListenAndServe(ctx, o.Addr, srv.Handler())
}
