package updates

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/jobs"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/restart"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/watch"
)

// Service lists what a machine can update and starts the one asked for, as a job the person first saw as a command.
type Service struct {
	Health  *health.Collector
	Jobs    *jobs.Runner
	Restart *restart.Restarter
	Watch   *watch.Watcher
}

// StaleError is a run whose command is no longer the one shown: the person should look again.
type StaleError struct{ Reason string }

func (e *StaleError) Error() string { return e.Reason }

// Available is everything the machine can update now: the tools with a newer version noted,
// this remuda deployed over ssh, the newest release, and the restarts on their own.
func (s *Service) Available(mc *machines.Machine) []Update {
	report := s.Health.Report(mc.ID())
	if report == nil {
		report = &health.Report{}
	}
	offered := Offered(report, s.Health.Latest())
	snap, _ := s.Watch.Snapshot(mc.ID())
	for i := range offered {
		if note := restartNote(snap, restart.Kinds(offered[i].What)); note != "" {
			offered[i].Note += " " + note
		}
	}
	if u, ok := s.deployPlan(mc, report); ok {
		offered = append(offered, u)
	}
	if u, ok := s.releasePlan(mc, report); ok {
		offered = append(offered, u)
	}
	offered = append(offered, restartPlans(snap)...)
	if u, ok := serverRestartPlan(mc, report, snap); ok {
		offered = append(offered, u)
	}
	return offered
}

// Recent is the machine's latest jobs, newest first, and how many chats there wait for a restart.
func (s *Service) Recent(mc *machines.Machine) (list []jobs.Job, pendingRestarts int) {
	list = s.Jobs.List(mc.ID(), 8)
	if list == nil {
		list = []jobs.Job{}
	}
	return list, s.Restart.Pending(mc.ID())
}

// Job is the job as it stands.
func (s *Service) Job(id string) (jobs.Job, bool) { return s.Jobs.Get(id) }

// RestartPane starts a job restarting one chat on its session; what the watcher's snapshot already rules out is refused before any job.
func (s *Service) RestartPane(mc *machines.Machine, paneID string) (jobs.Job, error) {
	snap, _ := s.Watch.Snapshot(mc.ID())
	p, found := restart.Lookup(snap, paneID)
	if !found {
		return jobs.Job{}, fmt.Errorf("no pane %s on %s", paneID, mc.Label())
	}
	kind, session, err := restart.Resumable(p)
	if err != nil {
		return jobs.Job{}, err
	}
	title := "Restart " + restart.Title(kind) + " in " + p.DisplayTitle()
	command := "# in " + paneID + " on " + mc.Label() + ", through herdr:\n" + restartCommand(kind, session)
	return s.Jobs.Run(mc, title, command, func(say func(string)) error { return s.Restart.Pane(mc, paneID, say) }), nil
}

// Start runs the update called what on the machine; the command must be the one shown, or the run is a StaleError.
func (s *Service) Start(mc *machines.Machine, what, command string) (jobs.Job, error) {
	if command == "" { // the promise is "what was shown is what runs": a run without the shown text is refused
		return jobs.Job{}, fmt.Errorf("the command shown must come back with the request")
	}
	report := s.Health.Report(mc.ID())
	snap, _ := s.Watch.Snapshot(mc.ID())
	switch {
	case what == "restart-herdr":
		installed, running, err := mc.ServerVersions()
		if err != nil || running == "" {
			return jobs.Job{}, fmt.Errorf("no herdr server runs there (%v)", err)
		}
		u := serverRestartUpdate(mc, running, installed, snap)
		if command != u.Command {
			return jobs.Job{}, &StaleError{"the versions changed since this was shown — look again"}
		}
		return s.Jobs.Run(mc, u.Title, u.Command, func(say func(string)) error { return s.Restart.Server(mc, say) }), nil
	case strings.HasPrefix(what, "restart-"):
		kind := strings.TrimPrefix(what, "restart-")
		if restart.Kinds(kind) == nil {
			return jobs.Job{}, fmt.Errorf("nothing to restart called %q", what)
		}
		u, n := restartPlan(kind, snap)
		if n == 0 {
			return jobs.Job{}, fmt.Errorf("no %s chat with a session to resume there now", restart.Title(kind))
		}
		if command != u.Command {
			return jobs.Job{}, &StaleError{"the chats changed since this was shown — look again"}
		}
		return s.Jobs.Run(mc, u.Title, u.Command, func(say func(string)) error { return s.Restart.Now(mc, []string{kind}, say) }), nil
	case what == "remuda-release":
		u, ok := s.releasePlan(mc, report)
		if !ok {
			return jobs.Job{}, fmt.Errorf("no newer release of remuda is known for this machine")
		}
		if command != u.Command {
			return jobs.Job{}, &StaleError{"the release changed since this was shown — look again"}
		}
		return s.Jobs.Start(mc, u.Title, u.Command), nil
	case what == "remuda":
		u, ok := s.deployPlan(mc, report)
		if !ok {
			return jobs.Job{}, fmt.Errorf("this remuda cannot be deployed there (a remote machine of the same OS and CPU is needed)")
		}
		if command != u.Command {
			return jobs.Job{}, &StaleError{"the update changed since it was shown — look at it again"}
		}
		return s.Jobs.Run(mc, u.Title, u.Command, func(say func(string)) error { return s.deploySelf(mc, say) }), nil
	}
	u, err := Plan(what, report)
	if err != nil {
		return jobs.Job{}, err
	}
	if command != u.Command {
		return jobs.Job{}, &StaleError{"the update changed since it was shown — look at it again"}
	}
	first, then := s.restartHooks(mc, what)
	if what == "herdr" {
		then = s.herdrThen(mc)
	}
	return s.Jobs.StartThen(mc, u.Title, u.Command, first, then), nil
}

// serverRestartPlan offers a restart of herdr's server when it still runs an older version than the one installed.
func serverRestartPlan(mc *machines.Machine, rep *health.Report, snap *herdr.Snapshot) (Update, bool) {
	if rep == nil || rep.HerdrServer == "" || rep.Herdr == "" || rep.HerdrServer == rep.Herdr {
		return Update{}, false
	}
	return serverRestartUpdate(mc, rep.HerdrServer, rep.Herdr, snap), true
}

func serverRestartUpdate(mc *machines.Machine, running, installed string, snap *herdr.Snapshot) Update {
	ready, busy := restart.Count(snap, restart.Kinds("agents"))
	n := ready + busy
	note := fmt.Sprintf("The server runs %s while %s is installed. A restart ends every pane process; herdr restores the workspaces, tabs and panes, and remuda starts each of the %d chat(s) again with its session resumed. Shells and other processes in panes come back as fresh shells.", running, installed, n)
	if busy > 0 {
		note += fmt.Sprintf(" %d chat(s) are busy right now: whatever they are doing is cut short.", busy)
	}
	return Update{What: "restart-herdr", Title: "Restart herdr's server", Command: "# on " + mc.Label() + ", through remuda:\nherdr server stop; herdr server   # " + running + " → " + installed + "; then every chat started again on its session", Note: note}
}

// herdrThen is the step after an install of herdr: the running server onto the new version, unless the installer's handoff already did that.
func (s *Service) herdrThen(mc *machines.Machine) func(say func(string)) error {
	return func(say func(string)) error {
		installed, running, err := mc.ServerVersions()
		if err != nil {
			return fmt.Errorf("read herdr's versions: %v", err)
		}
		say("")
		switch {
		case installed == "":
			return fmt.Errorf("cannot read the installed herdr's version")
		case running == "":
			say("no server runs; remuda starts " + installed)
			return mc.StartServer()
		case running == installed:
			say("the server runs " + installed + " already")
			return nil
		}
		say(fmt.Sprintf("== the server still runs %s; onto %s", running, installed))
		return s.Restart.Server(mc, say)
	}
}

// restartNote says what an agent update does to the chats afterwards.
func restartNote(snap *herdr.Snapshot, kinds []string) string {
	if len(kinds) == 0 {
		return ""
	}
	ready, busy := restart.Count(snap, kinds)
	if ready+busy == 0 {
		return ""
	}
	msg := fmt.Sprintf("Afterwards %d chat(s) here are restarted with their sessions resumed", ready+busy)
	if busy > 0 {
		msg += fmt.Sprintf(" (%d busy: once they have settled for a while)", busy)
	}
	return msg + "; a message half-typed in a chat's box is lost."
}

// restartPlans offers a restart on its own for each agent with chats on the machine, for an agent that updated itself.
func restartPlans(snap *herdr.Snapshot) []Update {
	if snap == nil {
		return nil
	}
	var out []Update
	for _, kind := range restart.Kinds("agents") {
		if u, n := restartPlan(kind, snap); n > 0 {
			out = append(out, u)
		}
	}
	return out
}

// restartPlan is the offer to restart one agent's chats; the panes it means are in the command text, so a Run on a stale offer is refused.
func restartPlan(kind string, snap *herdr.Snapshot) (Update, int) {
	if restart.Kinds(kind) == nil {
		return Update{}, 0
	}
	ready, busy := restart.Chats(snap, []string{kind})
	var ids []string
	for _, p := range append(append([]herdr.PaneInfo{}, ready...), busy...) {
		ids = append(ids, p.PaneID)
	}
	n := len(ids)
	title := restart.Title(kind)
	note := fmt.Sprintf("%d chat(s)", n)
	if len(busy) > 0 {
		note += fmt.Sprintf(", %d of them busy (those restart once they have settled for a while)", len(busy))
	}
	note += ". " + restartWords(kind) + fmt.Sprintf(" — for a %s that updated itself, or after `%s update` run by hand. A message half-typed in a chat's box is lost.", title, kind)
	cmd := "# in every " + title + " pane here (" + strings.Join(ids, ", ") + "), through herdr:\n" + restartCommand(kind, "<session>")
	return Update{What: "restart-" + kind, Title: "Restart " + title + " chats", Command: cmd, Note: note}, n
}

// restartCommand is what a restart types into a pane of the kind, for showing: the agent's own in-place restart
// when it has one, with the exit and resume it falls back to, else the exit and resume alone.
func restartCommand(kind, session string) string {
	leave := restart.Resume(kind, session) + "   (after its own " + restart.Exit(kind) + ")"
	if in := restart.InPlace(kind); in != "" {
		return in + "   (restarts in place, the session kept; without the command: " + leave + ")"
	}
	return leave
}

// restartWords says how a chat of the kind is restarted.
func restartWords(kind string) string {
	title := restart.Title(kind)
	leave := fmt.Sprintf("asked to leave and %s is started again in the same pane with the conversation resumed", title)
	if in := restart.InPlace(kind); in != "" {
		return fmt.Sprintf("Each is told `%s`, so %s restarts in place with the conversation kept; one without the command is %s", in, title, leave)
	}
	return "Each is " + leave
}

// restartHooks reads the agents' versions before and after an update's script and restarts the chats of every agent whose version changed.
func (s *Service) restartHooks(mc *machines.Machine, what string) (first func(say func(string)), then func(say func(string)) error) {
	kinds := restart.Kinds(what)
	if len(kinds) == 0 {
		return nil, nil
	}
	before := map[string]string{}
	versions := func() map[string]string {
		s.Health.Refresh(mc)
		out := map[string]string{}
		if rep := s.Health.Report(mc.ID()); rep != nil {
			for _, k := range kinds {
				out[k] = agentVersion(rep, k)
			}
		}
		return out
	}
	first = func(say func(string)) {
		before = versions()
		var have []string
		for _, k := range kinds {
			if before[k] != "" {
				have = append(have, restart.Title(k)+" "+before[k])
			}
		}
		if len(have) > 0 {
			say("before: " + strings.Join(have, ", "))
		}
	}
	then = func(say func(string)) error {
		after := versions()
		var changed []string
		for _, k := range kinds {
			now := after[k]
			if now == "" {
				if before[k] != "" {
					say(fmt.Sprintf("could not read %s's version there now — use \"Restart %s chats\" if it did update", restart.Title(k), restart.Title(k)))
				}
				continue
			}
			if before[k] == now {
				say(fmt.Sprintf("%s is still %s — its chats stay as they are", restart.Title(k), now))
				continue
			}
			say(fmt.Sprintf("%s: %s → %s", restart.Title(k), firstNonEmpty(before[k], "?"), now))
			changed = append(changed, k)
		}
		if len(changed) == 0 {
			return nil
		}
		say("")
		say("== chats onto the new version")
		return s.Restart.Now(mc, changed, say)
	}
	return first, then
}

func agentVersion(r *health.Report, kind string) string {
	switch kind {
	case "claude":
		return r.Claude
	case "codex":
		return r.Codex
	case "gemini":
		return r.Gemini
	case "kimi":
		return r.Kimi
	}
	return ""
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// deployPlan offers this very binary to a remote machine of the same OS and CPU whose remuda is behind or missing.
func (s *Service) deployPlan(mc *machines.Machine, rep *health.Report) (Update, bool) {
	if mc.Local() || rep == nil || rep.Arch == "" || rep.Arch != runtime.GOARCH {
		return Update{}, false
	}
	if (runtime.GOOS == "darwin") != strings.HasPrefix(rep.OS, "macOS") {
		return Update{}, false
	}
	me := s.Health.Version
	if me == "" || me == "dev" || rep.Remuda == me {
		return Update{}, false
	}
	have := rep.Remuda
	if have == "" {
		have = "not deployed"
	}
	return Update{What: "remuda", Title: "Deploy this remuda there", Command: "# this remuda binary → " + mc.Cfg.Host + ":~/.local/bin/remuda.new over the ssh connection, then there:\n" + deployScript,
		Note: have + " → " + me + ". The binary this page is served by is copied over your SSH and its service restarted; the peer relay reconnects by itself."}, true
}

// deployScript installs the copied binary and restarts the service: the two steps a release download ends with too.
const deployScript = InstallScript + " && " + RestartScript

// releasePlan offers a newer GitHub release of remuda than the machine runs, the one serving this page included.
func (s *Service) releasePlan(mc *machines.Machine, rep *health.Report) (Update, bool) {
	rel := s.Health.Release()
	if mc.Local() {
		return ReleaseUpdate(s.Health.Version, runtime.GOOS, runtime.GOARCH, rel, true)
	}
	if rep == nil {
		return Update{}, false
	}
	return ReleaseUpdate(rep.Remuda, rep.OS, rep.Arch, rel, false)
}

// deploySelf copies this binary to the machine and restarts the service.
func (s *Service) deploySelf(mc *machines.Machine, say func(string)) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	say(fmt.Sprintf("copying %s (%.1f MB) to %s:~/.local/bin/remuda.new…", exe, float64(len(data))/1048576, mc.Cfg.Host))
	conn, err := mc.RemoteConn()
	if err != nil {
		return err
	}
	if err := conn.Put(mc.Home()+"/.local/bin/remuda.new", data, 5*time.Minute); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	say("installing and restarting the service…")
	out, err := shell.Text(mc, deployScript, 2*time.Minute)
	if out != "" {
		say(strings.TrimSpace(out))
	}
	if err != nil {
		return err
	}
	// the job ends only once the new remuda answers, so the vitals read that follows finds the peer again
	say("waiting for remuda there…")
	for i := 0; i < 30; i++ {
		time.Sleep(time.Second)
		if out, err := shell.Text(mc, `curl -s -m 2 http://127.0.0.1:7317/api/health 2>/dev/null`, 10*time.Second); err == nil && strings.Contains(out, `"ok":true`) {
			say("remuda there answers: " + strings.TrimSpace(out))
			return nil
		}
	}
	say("remuda there has not answered yet; the next vitals read will tell")
	return nil
}
