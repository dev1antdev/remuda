// Package app is remuda's composition root: it builds every service from
// the state dir and the flags, lists who hears which event, and serves
// until the context ends. Nothing here does work of its own.
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/arena"
	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/awake"
	"github.com/dev1antdev/remuda/internal/away"
	"github.com/dev1antdev/remuda/internal/backup"
	"github.com/dev1antdev/remuda/internal/budget"
	"github.com/dev1antdev/remuda/internal/chat"
	"github.com/dev1antdev/remuda/internal/code"
	"github.com/dev1antdev/remuda/internal/complete"
	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/feed"
	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/icons"
	"github.com/dev1antdev/remuda/internal/jira"
	"github.com/dev1antdev/remuda/internal/jobs"
	"github.com/dev1antdev/remuda/internal/limits"
	"github.com/dev1antdev/remuda/internal/live"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/mirror"
	"github.com/dev1antdev/remuda/internal/opinion"
	"github.com/dev1antdev/remuda/internal/prefs"
	"github.com/dev1antdev/remuda/internal/preview"
	"github.com/dev1antdev/remuda/internal/prstatus"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/quick"
	"github.com/dev1antdev/remuda/internal/radar"
	"github.com/dev1antdev/remuda/internal/recap"
	"github.com/dev1antdev/remuda/internal/recipes"
	"github.com/dev1antdev/remuda/internal/restart"
	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/said"
	"github.com/dev1antdev/remuda/internal/schedule"
	"github.com/dev1antdev/remuda/internal/scorecard"
	"github.com/dev1antdev/remuda/internal/scratch"
	"github.com/dev1antdev/remuda/internal/sent"
	"github.com/dev1antdev/remuda/internal/server"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/setup"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/signin"
	"github.com/dev1antdev/remuda/internal/state"
	"github.com/dev1antdev/remuda/internal/subagents"
	"github.com/dev1antdev/remuda/internal/tasks"
	"github.com/dev1antdev/remuda/internal/timelapse"
	"github.com/dev1antdev/remuda/internal/transcript"
	"github.com/dev1antdev/remuda/internal/triggers"
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
	Door          bool   // place the agents' door files here and on every remote (off for a scratch instance)
	Server        bool   // serve the page only: the host is no machine of the fleet, its herdr neither needed nor kept
	MirrorClosed  string // why this remuda writes nothing into herdr (a scratch instance beside the real one); "" writes
	Record        bool   // record the agents' timelapses (off for a scratch instance: the real one records)
}

// Run builds remuda and serves until ctx ends.
func Run(ctx context.Context, o Options) error {
	bin, err := herdr.FindBinary(o.HerdrBin)
	if err != nil && !o.Server {
		return fmt.Errorf("herdr: %w", err)
	}
	if o.Server {
		log.Print("serving the page only (-server): this machine is none of the cockpit's; its herdr is neither asked nor kept")
	} else if err := herdr.EnsureServer(bin, o.Socket); err != nil {
		log.Printf("WARNING: herdr is not answering at %s (%v); remuda will keep trying", o.Socket, err)
	} else if version, protocol, err := herdr.NewClient(o.Socket).Ping(); err == nil {
		log.Printf("connected to herdr %s (protocol %d) at %s", version, protocol, o.Socket)
	}
	ui, err := web.Assets()
	if err != nil {
		return fmt.Errorf("ui assets: %w", err)
	}
	// the state dir is held while served, so `remuda restore` knows to refuse
	if release, err := state.Hold(o.Dir); err != nil {
		log.Printf("WARNING: %v; a restore cannot tell that this remuda serves it", err)
	} else {
		defer release()
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
	fleet.SetServer(o.Server)
	defer fleet.Close()
	if local, err := fleet.Get(machines.LocalID); err == nil && o.Door && !o.Server {
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
	defer pushSvc.Sockets.Close() // the Mac app's alert stream ends with "remuda is stopping", as the page's events do
	var mirrored *mirror.Service  // named here: an alert the feed records may pop up inside herdr too
	pushSvc.OnAlert = func(p push.Payload) {
		activity.Add(feed.Entry{Kind: p.Kind, Machine: p.Machine, Pane: p.Pane, Title: p.Title, Body: p.Body, Worked: p.Worked})
		hub.RosterChanged() // the bell's dot rides on the roster
		if mirrored != nil {
			mirrored.Alert(p)
		}
	}
	// a muted workspace: the pane's folder from the watcher's last snapshot, for the push service's silence check
	pushSvc.Folder = func(machineID, paneID string) string {
		snap, _ := watcher.Snapshot(machineID)
		if snap == nil {
			return ""
		}
		pane := snap.Pane(paneID)
		if pane == nil {
			return ""
		}
		ws := snap.Workspace(pane.WorkspaceID)
		if ws == nil {
			return ""
		}
		return snap.WorkspaceDir(ws)
	}
	// a question push carries the dialog's choices as buttons, read off the pane's screen as the push goes out
	pushSvc.Question = func(machineID, paneID string) (push.Asked, bool) {
		mc, err := fleet.Get(machineID)
		if err != nil {
			return push.Asked{}, false
		}
		client, _, err := mc.Ready()
		if err != nil {
			return push.Asked{}, false
		}
		screen, err := client.PaneRead(paneID)
		if err != nil {
			return push.Asked{}, false
		}
		q, ok := chat.ParseQuestion(screen)
		if !ok {
			return push.Asked{}, false
		}
		return push.Asked{Text: q.Text, Choices: q.Choices(), Key: chat.DialogKey(q.Text)}, true
	}
	tracker := radar.New(o.Dir, chats.LastWriteOf)
	defer tracker.Flush()
	issues := jira.New(o.Dir)
	issues.OnChange = hub.RosterChanged // an issue read from the site moves the chips on the page
	// the issue keys a conversation names, read only once the person set a Jira site
	issueKeys := func(machineID, paneID string) []string {
		if !issues.Configured() {
			return nil
		}
		return jira.KeysIn(chats.Texts(machineID, paneID, 40))
	}
	sayings := said.New(said.Inputs{Version: chats.SessionVersion, Answer: chats.LastAnswer, Link: chats.PullRequest, Keys: issueKeys}, bus)
	requests := prstatus.New(func(machineID string) (shell.Shell, bool) {
		mc, err := fleet.Get(machineID)
		if err != nil {
			return nil, false
		}
		return mc, true
	})
	requests.OnState = func(e events.PullRequestState) { events.Publish(bus, e) }
	// how many subagents each Claude Code pane's live conversation runs, read while a page reads the roster, for the Wall's zoo
	subagentCounts := subagents.New(chats.Subagents, func(machineID string) bool {
		mc, err := fleet.Get(machineID)
		if err != nil {
			return false
		}
		_, _, err = mc.Ready()
		return err == nil
	})
	subagentCounts.OnChange = hub.RosterChanged
	reviews := review.New(o.Dir)
	// the agents' turns as timelapses: a working pane's screen read every few seconds over its machine's herdr, the
	// turn written once it ends; the turns still going are written as stopped when remuda stops
	recorder := timelapse.Open(o.Dir, timelapse.Inputs{
		Screen: func(machineID, paneID string) (string, error) {
			mc, err := fleet.Get(machineID)
			if err != nil {
				return "", err
			}
			client, _, err := mc.Ready()
			if err != nil {
				return "", err
			}
			screen, err := client.PaneScreen(paneID)
			return screen.Text, err
		},
		Folder: func(machineID, dir string, ws *herdr.WorkspaceInfo) string {
			return server.FolderOf(ws.Worktree, reviews.Root(machineID, dir), dir)
		},
	})
	defer recorder.Close()
	if !o.Record {
		recorder.Scratch()
		log.Print("the agents' timelapses are left to the real instance: this one records none (-record records)")
	}
	// a second opinion's reviewer, once it has answered, has its tab closed when the opinion asked for that
	opinions := opinion.NewService(o.Dir)
	opinions.Read = func(op opinion.Opinion) (string, bool) {
		mc, err := fleet.Get(op.Machine)
		if err != nil {
			return "", false
		}
		if conv, err := chats.Open(mc, op.Pane, "", "", true); err == nil && conv.Path != "" {
			if turns, err := conv.Turns(); err == nil {
				if answer, ok := opinion.Answered(turns, op); ok {
					return answer, true
				}
			}
		}
		return opinion.Request(mc, mc.Transcripts, op)
	}
	opinions.Close = func(machineID, tabID, paneID string) error {
		mc, err := fleet.Get(machineID)
		if err != nil {
			return err
		}
		client, _, err := mc.Ready()
		if err != nil {
			return err
		}
		if snap, _ := watcher.Snapshot(machineID); snap != nil {
			if pane := snap.Pane(paneID); pane == nil || pane.TabID != tabID {
				return fmt.Errorf("tab %s no longer holds the reviewer's pane %s", tabID, paneID)
			}
		}
		defer watcher.Refresh(mc)
		return client.TabClose(tabID)
	}
	// a rejected folder whose agent answers that it is ready again: the review notes it, the person is told; a second
	// opinion's reviewer is read by the opinions themselves (OnStatus, Observe), by its request, and never counts here
	sayings.OnAnswer = func(machineID, paneID, text string) {
		if opinions.Reviewer(machineID, paneID) {
			return
		}
		snap, _ := watcher.Snapshot(machineID)
		if snap == nil {
			return
		}
		pane := snap.Pane(paneID)
		if pane == nil {
			return
		}
		ws := snap.Workspace(pane.WorkspaceID)
		if ws == nil {
			return
		}
		if dir := snap.WorkspaceDir(ws); dir != "" && reviews.OnAnswer(machineID, dir, text, time.Now()) {
			pushSvc.Notify(review.ReadyNotice(machineID, paneID, ws.Label, text))
		}
	}
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
	vitals := health.New(o.Dir, fleet, o.Version, o.VersionCheck)
	// Claude Code's sign-in on each machine, read with the vitals: warned of three days before it ends, a day before, once ended
	signins := signin.New(o.Dir)
	signins.Notify = pushSvc.Notify
	vitals.OnRead = signinWarnings(signins, fleet)
	runner := jobs.New(bus)
	restarter := restart.New(fleet)
	restarter.Later = func(mc *machines.Machine, title string, fn func(say func(string)) error) {
		runner.Run(mc, title, "", fn)
	}
	restarter.Lock = input.Lock
	// the chats a reboot stops: remembered here, offered back as one Resume, noted in the activity when they return
	restarter.Memory = restart.OpenMemory(o.Dir)
	restarter.Memory.Note = func(machineID, title, body string) {
		activity.Add(feed.Entry{Kind: "recover", Machine: machineID, Title: title, Body: body})
		hub.RosterChanged()
	}
	// what the restarts type by themselves is a quiet activity row too, like anything a device sends
	restarter.Typed = func(machineID, paneID, text string) {
		workspace := ""
		if snap, err := watcher.Snapshot(machineID); err == nil && snap != nil {
			if pane := snap.Pane(paneID); pane != nil {
				workspace = snap.WorkspaceLabel(pane.WorkspaceID)
			}
		}
		activity.Add(sent.Entry(sent.Remuda, text, false, machineID, paneID, workspace))
	}
	// the agents' subscription windows: read off every machine, warned of at 80 % and the limit, the agents stopped on
	// one noted, and the person's bookings to continue them said when due
	planLimits := limits.New(o.Dir)
	planLimits.Notify = pushSvc.Notify
	planLimits.Changed = hub.RosterChanged
	planLimits.Check = func(machineID, paneID string) (limits.Hit, bool) {
		at, text, ok := chats.RateLimited(machineID, paneID)
		return limits.Hit{At: at, Text: text}, ok
	}
	planLimits.Pane = func(machineID, paneID string) (limits.Place, bool) {
		snap, _ := watcher.Snapshot(machineID)
		if snap == nil {
			return limits.Place{}, false
		}
		pane := snap.Pane(paneID)
		if pane == nil {
			return limits.Place{}, false
		}
		place := limits.Place{Agent: pane.Agent, Status: pane.AgentStatus, Workspace: snap.WorkspaceLabel(pane.WorkspaceID)}
		if pane.AgentSession != nil {
			place.Session = pane.AgentSession.Value
		}
		return place, true
	}
	planLimits.Submit = func(machineID, paneID, text string) error {
		mc, err := fleet.Get(machineID)
		if err != nil {
			return err
		}
		client, _, err := mc.Ready()
		if err != nil {
			return err
		}
		if err := input.SubmitToAgent(client, events.PaneRef{Machine: machineID, Pane: paneID}, text); err != nil {
			return err
		}
		place, _ := planLimits.Pane(machineID, paneID)
		activity.Add(sent.Entry(sent.Remuda, text, false, machineID, paneID, place.Workspace))
		return nil
	}
	keeper := &herdr.Keeper{Bin: bin, Socket: o.Socket}
	// the machines kept awake while their agents work: the hold is let go before the fleet's masters close
	hostname, _ := os.Hostname()
	keepAwake := awake.New(o.Dir, func(id string) (shell.Host, bool) {
		mc, err := fleet.Get(id)
		if err != nil {
			return nil, false
		}
		return mc, true
	}, awake.ShellHolder{Dir: o.Dir, Name: strings.TrimSuffix(hostname, ".local")})
	defer keepAwake.Close()
	// the cockpit's state dir copied to a machine of the fleet the person picked, nightly and on demand
	backups := backup.Open(o.Dir, hostname, o.Version)
	backups.Target = func(id string) (backup.Target, error) {
		mc, err := fleet.Get(id)
		if err != nil {
			return nil, err
		}
		if mc.Local() {
			if o.Server {
				return nil, errors.New("this remuda serves the page only: a copy on its own disk would die with it; pick another machine in Settings › Backups")
			}
			return mc, nil // a file written here needs no herdr
		}
		if _, _, err := mc.Ready(); err != nil {
			return nil, fmt.Errorf("%s is not connected: %w", mc.Label(), err)
		}
		return mc, nil
	}
	backups.Notify = func(title, body string) {
		pushSvc.Broadcast(push.Payload{Kind: "backup", Title: title, Body: body, Tag: "backup"})
	}
	// what remuda knows of each agent, shown inside herdr itself: tokens on its panes and workspaces, and toasts
	mirrored = mirror.New(o.Dir, mirror.Inputs{
		Herdr: func(id string) (mirror.Herdr, bool) {
			mc, err := fleet.Get(id)
			if err != nil {
				return nil, false
			}
			client, _, err := mc.Ready()
			if err != nil {
				return nil, false
			}
			return client, true
		},
		Machines: func() []mirror.Machine {
			var out []mirror.Machine
			for _, mc := range fleet.List() {
				if state, _ := mc.State(); state == machines.StateConnected {
					out = append(out, mirror.Machine{ID: mc.ID(), Label: mc.Label()})
				}
			}
			return out
		},
		Snapshot: func(id string) *herdr.Snapshot {
			snap, _ := watcher.Snapshot(id)
			return snap
		},
		Radar: tracker.Display,
		Usage: meter.Display,
		Said:  sayings.Line,
		Link:  sayings.Link,
		PR:    requests.Status,
		Limit: planLimits.PaneOf,
		Review: func(machineID, dir string) (int, bool) {
			badge, _ := reviews.Badge(machineID, dir)
			return badge.Fresh, reviews.Ready(machineID, dir)
		},
		Tasks: taskStore.LiveTasks,
	}, o.MirrorClosed)
	defer mirrored.Close()
	if o.MirrorClosed != "" {
		log.Printf("remuda's figures and alerts stay out of herdr: %s", o.MirrorClosed)
	}
	recaps := &recap.Service{Machines: fleet, Watch: watcher, Chat: chats, Radar: tracker}
	schedules := schedule.New(o.Dir, input.SubmitToFolder, pushSvc.Broadcast, func(since time.Time) (string, string, bool) {
		return recap.Digest(recaps.Standup(since), since, time.Now())
	})
	cookbook := recipes.New(o.Dir)
	retiring := retirements(cookbook, schedules, taskStore, fleet, watcher, meter, vitals)
	scouts := &sessions.Launcher{Machines: fleet, Watch: watcher, Input: input, Notify: pushSvc.Broadcast}
	schedules.Agents = schedule.Agents{
		Start: func(s schedule.Schedule) (schedule.Started, error) {
			start, err := s.Start()
			if err != nil {
				return schedule.Started{}, err
			}
			place, err := scouts.StartInFolder(s.Machine, s.Dir, s.Label, s.Agent, start, s.Prompt)
			return schedule.Started{Pane: place.Pane, Tab: place.Tab, Workspace: place.Workspace, MadeWorkspace: place.MadeWorkspace}, err
		},
		Close: func(run schedule.Started) error {
			return scouts.Close(run.Machine, sessions.Place{Pane: run.Pane, Tab: run.Tab, Workspace: run.Workspace, MadeWorkspace: run.MadeWorkspace})
		},
		Answer: func(machine, pane string) string {
			answer, _ := chats.LastAnswer(machine, pane)
			return answer
		},
		Denied: func(machine, pane string) []transcript.Denial { return chats.Denied(machine, pane, "").Calls },
		Status: func(machine, pane string) string {
			info, snap := chats.Pane(machine, pane)
			switch {
			case snap == nil:
				return ""
			case info == nil || info.Agent == "":
				return events.Gone
			}
			return info.AgentStatus
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
	events.Subscribe(bus, func(e events.Snapshot) { subagentCounts.Observe(e.Machine, e.Snap) })
	events.Subscribe(bus, func(e events.Snapshot) { recorder.Observe(e.Machine, e.Snap, e.At) })
	events.Subscribe(bus, func(e events.MachineRemoved) { recorder.Forget(e.Machine) })
	events.Subscribe(bus, func(e events.Snapshot) {
		planLimits.Observe(e.Machine, e.Snap)
		if mc, err := fleet.Get(e.Machine); err == nil {
			planLimits.Refresh(mc, mc.Label())
		}
	})
	events.Subscribe(bus, restarter.Observe)
	events.Subscribe(bus, mirrored.Observe)
	events.Subscribe(bus, func(e events.Snapshot) { hub.RosterChanged() })
	events.Subscribe(bus, func(e events.MachineRemoved) {
		tracker.Forget(e.Machine)
		sayings.Forget(e.Machine)
		hub.RosterChanged()
		requests.Forget(e.Machine)
		restarter.Memory.Forget(e.Machine)
		planLimits.Forget(e.Machine)
		opinions.ForgetMachine(e.Machine)
		subagentCounts.Forget(e.Machine)
		signins.Forget(e.Machine)
		mirrored.Forget(e.Machine)
	})
	events.Subscribe(bus, opinions.OnStatus)
	events.Subscribe(bus, opinions.Observe)
	events.Subscribe(bus, pushSvc.OnStatus)
	events.Subscribe(bus, sayings.OnStatus)
	events.Subscribe(bus, subagentCounts.OnStatus)
	events.Subscribe(bus, planLimits.OnStatus)
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
	if !o.Server {
		events.Subscribe(bus, func(e events.LocalHerdrDown) { keeper.Revive(e.Err) })
	}
	events.Subscribe(bus, func(e events.Snapshot) { keepAwake.Observe(e.Machine, awake.Working(e.Snap)) })
	events.Subscribe(bus, func(e events.MachineRemoved) { keepAwake.Forget(e.Machine) })
	shells := scratch.Open(o.Dir) // the floating terminal's shell on each machine
	events.Subscribe(bus, func(e events.MachineRemoved) { shells.Forget(e.Machine) })

	// a remote that stays unreachable, and one that answers again: the machines publish every move, the tracker says when it matters
	absent := away.New(away.AwayAfter, func(n away.Notice) {
		title, body := away.Words(n)
		pushSvc.Notify(push.Payload{Machine: n.Machine, Kind: "machine", Title: title, Body: body, Tag: "machine|" + n.Machine})
	})
	fleet.OnState(func(id, label, state, reason string) {
		events.Publish(bus, events.MachineState{Machine: id, Label: label, State: state, Reason: reason})
	})
	events.Subscribe(bus, func(e events.MachineState) { absent.Observe(e.Machine, e.Label, e.State, e.Reason) })
	events.Subscribe(bus, func(e events.MachineRemoved) { absent.Forget(e.Machine) })
	absentDone := make(chan struct{})
	defer close(absentDone)
	go absent.Run(30*time.Second, absentDone)

	// what the panes print: herdr watches each pane's rows on the machine's event stream, the triggers say what a
	// match means — a dev server offered as a preview, failed tests a push, the person's own rules their action
	paneTriggers := triggers.New(o.Dir)
	paneTriggers.Notify = pushSvc.Notify
	paneTriggers.Feed = func(machineID, paneID, title, body string) {
		activity.Add(feed.Entry{Kind: "trigger", Machine: machineID, Pane: paneID, Title: title, Body: body})
		hub.RosterChanged()
	}
	paneTriggers.MoveTask = func(machineID, paneID, column, why string) (string, bool, error) {
		moved, err := taskStore.OnTrigger(machineID, paneID, column, why)
		if err != nil || moved == nil {
			return "", false, err
		}
		return "Task " + moved.Task.Identifier + " → " + moved.To.Title, true, nil
	}
	// someone looking at a pane's terminal through this remuda sees its tests fail: no push for it
	paneTriggers.Viewed = func(machineID, paneID string) bool {
		mc, err := fleet.Get(machineID)
		if err != nil {
			return false
		}
		_, attach, err := mc.Ready()
		pane, _ := chats.Pane(machineID, paneID)
		return err == nil && pane != nil && attach.Viewers(pane.TerminalID) > 0
	}
	// a dev server is offered only once the previews find its port listening; srv is set before any match comes
	paneTriggers.Listening = func(machineID string, port int) (string, bool) { return srv.Listening(machineID, port) }
	paneTriggers.Changed = hub.RosterChanged
	paneTriggers.Resubscribe = watcher.Resubscribe
	watcher.Outputs = paneTriggers.Watches
	watcher.Refused = paneTriggers.Refused
	events.Subscribe(bus, paneTriggers.Observe)
	events.Subscribe(bus, paneTriggers.OnMatch)
	events.Subscribe(bus, func(e events.MachineRemoved) { paneTriggers.Forget(e.Machine) })

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
		Denied:     chats.Denied,
		Usage: func(machineID, paneID string) (float64, int64, string, bool) {
			d, ok := meter.Display(machineID, paneID)
			if !ok {
				return 0, 0, "", false
			}
			tokens := d.Tokens
			return d.Cost, tokens.Input + tokens.Output + tokens.CacheRead + tokens.CacheWrite5m + tokens.CacheWrite1h, d.Name, true
		},
		Notify:  pushSvc.Notify,
		PRTouch: requests.TouchPane,
	}
	launcher := &tasks.Launcher{Store: taskStore, Input: input, Watch: watcher, Notify: pushSvc.Notify}
	// a pipeline's next stage: its agent in a new tab of the worktree, through the launcher on the run's machine
	runWatch.Advance = func(prev tasks.Run, index int, stage tasks.Stage, prompt string) error {
		mc, err := fleet.Get(prev.Machine)
		if err != nil {
			return err
		}
		client, _, err := mc.Ready()
		if err != nil {
			return err
		}
		_, err = launcher.StartStage(mc, client, prev, index, stage, prompt)
		return err
	}
	go runWatch.Loop(15 * time.Second)
	go taskQueue(taskStore, fleet, watcher, launcher, tracker, chats, planLimits, budgets, pushSvc.Notify).Loop(30 * time.Second)
	events.Subscribe(bus, func(e events.PullRequestNamed) { runWatch.OnPullRequest(e.Pane.Machine, e.Pane.Pane, e.URL, e.Label) })
	events.Subscribe(bus, func(e events.PullRequestState) {
		runWatch.OnPullRequestState(e.Pane.Machine, e.Pane.Pane, e.URL, e.State)
	})
	go schedules.Loop()
	go hub.Run(ctx)
	go budgets.Loop(5 * time.Minute)
	go planLimits.Loop(30*time.Second, ctx.Done())
	go backups.Loop(time.Minute, ctx.Done())

	arenas := &arena.Service{Store: arena.Open(o.Dir), Machines: fleet, Watch: watcher, Input: input, Tasks: taskStore, Notify: pushSvc.Notify, Dir: o.Dir,
		ModelOf: func(machineID, paneID string) string {
			d, _ := meter.Display(machineID, paneID)
			return d.Name
		}}
	srv = &server.Server{
		Machines:  fleet,
		Watch:     watcher,
		Input:     input,
		Chat:      chats,
		Updates:   &updates.Service{Health: vitals, Jobs: runner, Restart: restarter, Watch: watcher, Machines: fleet},
		Tasks:     taskStore,
		Launcher:  launcher,
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
		Recipes:   cookbook,
		Retire:    retiring,
		Prefs:     prefs.New(o.Dir),
		Code:      code.New(o.CodePort),
		Complete:  complete.New(),
		Guard:     auth.NewGuard(o.Token, devices, keys),
		Keys:      keys,
		UI:        ui,
		Live:      hub,
		Awake:     keepAwake,
		Icons:     icons.New(o.Dir),
		Arena:     arenas,
		Limits:    planLimits,
		Scratch:   shells,
		Jira:      issues,
		Backup:    backups,
		SelfPort:  auth.PortOf(o.Addr),
		Opinions:  opinions,
		Subagents: subagentCounts,
		Scorecard: &scorecard.Service{Tasks: taskStore, Review: reviews, Arenas: arenas.Store},
		Mirror:    mirrored,
		Timelapse: recorder,
		Triggers:  paneTriggers,
	}
	previews := &preview.Service{Allow: srv.Guard.SignedIn, IdleAfter: 30 * time.Minute, Listen: preview.LoopbackListen(o.Addr)}
	srv.Previews = previews
	defer previews.Shutdown()
	events.Subscribe(bus, func(e events.MachineRemoved) { previews.CloseMachine(e.Machine) })
	if o.Tailscale {
		var node *tsnetx.Node
		srv.Tailscale = func() tsnetx.Status { return node.Status() } // set before the handler is handed out
		previews.Listen = tailnetPreviews(func() *tsnetx.Node { return node })
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

// tailnetPreviews opens a preview on a port of the tailnet node's own, over TLS on its name
func tailnetPreviews(node func() *tsnetx.Node) func(port int, host string) (net.Listener, string, error) {
	return func(port int, _ string) (net.Listener, string, error) {
		n := node()
		if n == nil {
			return nil, "", preview.ErrNotReady
		}
		ln, url, err := n.ListenTLS(port)
		if errors.Is(err, tsnetx.ErrNotRunning) {
			return nil, "", preview.ErrNotReady
		}
		return ln, url, err
	}
}
