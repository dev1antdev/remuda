package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/jobs"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/restart"
)

// Updates from the page: herdr, the agents and remuda itself, each a job the person first sees as a command, then watches run; nothing asks for sudo.

// handleMachineUpdates is GET /api/machines/{id}/updates: what this machine can update now.
func (s *Server) handleMachineUpdates(w http.ResponseWriter, r *http.Request) {
	mc, err := s.Machines.Get(r.PathValue("id"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	if s.Health == nil {
		writeJSON(w, map[string]any{"updates": []health.Update{}})
		return
	}
	rep := s.Health.Report(mc.Cfg.ID)
	if rep == nil {
		rep = &health.Report{}
	}
	ups := health.Updates(rep, s.Health.Latest())
	snap, _ := s.Watch.Snapshot(mc.Cfg.ID)
	for i := range ups {
		if note := s.restartNote(snap, restart.Kinds(ups[i].What)); note != "" {
			ups[i].Note += " " + note
		}
	}
	if u, ok := s.deployPlan(mc, rep); ok {
		ups = append(ups, u)
	}
	if u, ok := s.releasePlan(mc, rep); ok {
		ups = append(ups, u)
	}
	ups = append(ups, s.restartPlans(mc, snap)...)
	if u, ok := s.serverRestartPlan(mc, rep, snap); ok {
		ups = append(ups, u)
	}
	writeJSON(w, map[string]any{"updates": ups})
}

// serverRestartPlan offers a restart of herdr's server when it still runs an older version than the one installed.
func (s *Server) serverRestartPlan(mc *machines.Machine, rep *health.Report, snap *herdr.Snapshot) (health.Update, bool) {
	if s.Restart == nil || rep == nil || rep.HerdrServer == "" || rep.Herdr == "" || rep.HerdrServer == rep.Herdr {
		return health.Update{}, false
	}
	return serverRestartUpdate(mc, rep.HerdrServer, rep.Herdr, snap), true
}

func serverRestartUpdate(mc *machines.Machine, running, installed string, snap *herdr.Snapshot) health.Update {
	ready, busy := restart.Count(snap, []string{"claude", "codex", "gemini"})
	n := ready + busy
	note := fmt.Sprintf("The server runs %s while %s is installed. A restart ends every pane process; herdr restores the workspaces, tabs and panes, and remuda starts each of the %d chat(s) again with its session resumed. Shells and other processes in panes come back as fresh shells.", running, installed, n)
	if busy > 0 {
		note += fmt.Sprintf(" %d chat(s) are busy right now: whatever they are doing is cut short.", busy)
	}
	return health.Update{What: "restart-herdr", Title: "Restart herdr's server", Command: "# on " + mc.Label() + ", through remuda:\nherdr server stop; herdr server   # " + running + " → " + installed + "; then every chat started again on its session", Note: note}
}

// herdrThen is the step after an install of herdr: the running server onto the new version, unless the installer's handoff already did that.
func (s *Server) herdrThen(mc *machines.Machine) func(say func(string)) error {
	if s.Restart == nil {
		return nil
	}
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
func (s *Server) restartNote(snap *herdr.Snapshot, kinds []string) string {
	if s.Restart == nil || len(kinds) == 0 {
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
func (s *Server) restartPlans(mc *machines.Machine, snap *herdr.Snapshot) []health.Update {
	if s.Restart == nil || snap == nil {
		return nil
	}
	var out []health.Update
	for _, kind := range []string{"claude", "codex", "gemini"} {
		if u, n := restartPlan(kind, snap); n > 0 {
			out = append(out, u)
		}
	}
	return out
}

// restartPlan is the offer to restart one agent's chats; the panes it means are in the command text, so a Run on a stale offer is refused.
func restartPlan(kind string, snap *herdr.Snapshot) (health.Update, int) {
	if restart.Kinds(kind) == nil {
		return health.Update{}, 0
	}
	ready, busy := restart.Chats(snap, []string{kind})
	var ids []string
	for _, p := range append(append([]herdr.PaneInfo{}, ready...), busy...) {
		ids = append(ids, p.PaneID)
	}
	n := len(ids)
	t := restart.Title(kind)
	note := fmt.Sprintf("%d chat(s)", n)
	if len(busy) > 0 {
		note += fmt.Sprintf(", %d of them busy (those restart once they have settled for a while)", len(busy))
	}
	note += fmt.Sprintf(". Each is asked to leave and %s is started again in the same pane with the conversation resumed — for a %s that updated itself, or after `%s update` run by hand. A message half-typed in a chat's box is lost.", t, t, kind)
	cmd := "# in every " + t + " pane here (" + strings.Join(ids, ", ") + "), through herdr:\n" + restart.Resume(kind, "<session>") + "   (after its own " + exitOf(kind) + ")"
	return health.Update{What: "restart-" + kind, Title: "Restart " + t + " chats", Command: cmd, Note: note}, n
}

func exitOf(kind string) string {
	if kind == "claude" {
		return "/exit"
	}
	return "/quit"
}

// restartHooks reads the agents' versions before and after an update's script and restarts the chats of every agent whose version changed.
func (s *Server) restartHooks(mc *machines.Machine, what string) (first func(say func(string)), then func(say func(string)) error) {
	kinds := restart.Kinds(what)
	if s.Restart == nil || len(kinds) == 0 {
		return nil, nil
	}
	before := map[string]string{}
	versions := func() map[string]string {
		s.Health.Refresh(mc)
		out := map[string]string{}
		if rep := s.Health.Report(mc.Cfg.ID); rep != nil {
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

// handleMachineJobs is GET /api/machines/{id}/jobs: recent jobs there, newest first.
func (s *Server) handleMachineJobs(w http.ResponseWriter, r *http.Request) {
	mc, err := s.Machines.Get(r.PathValue("id"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	if s.Jobs == nil {
		writeJSON(w, map[string]any{"jobs": []jobs.Job{}})
		return
	}
	list := s.Jobs.List(mc.Cfg.ID, 8)
	if list == nil {
		list = []jobs.Job{}
	}
	writeJSON(w, map[string]any{"jobs": list, "pending": s.pendingRestarts(mc.Cfg.ID)})
}

func (s *Server) pendingRestarts(machineID string) int {
	if s.Restart == nil {
		return 0
	}
	return s.Restart.Pending(machineID)
}

// handleMachineUpdate is POST /api/machines/{id}/update {what, command}: starts it; the answer is the job.
func (s *Server) handleMachineUpdate(w http.ResponseWriter, r *http.Request) {
	mc, err := s.Machines.Get(r.PathValue("id"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	var req struct {
		What    string `json:"what"`
		Command string `json:"command"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpErr(w, 400, err)
		return
	}
	if s.Jobs == nil || s.Health == nil {
		httpErr(w, 503, fmt.Errorf("updates are off"))
		return
	}
	if req.Command == "" { // the promise is "what was shown is what runs": a run without the shown text is refused
		httpErr(w, 400, fmt.Errorf("the command shown must come back with the request"))
		return
	}
	rep := s.Health.Report(mc.Cfg.ID)
	var u health.Update
	if req.What == "restart-herdr" {
		if s.Restart == nil {
			httpErr(w, 503, fmt.Errorf("restarts are off"))
			return
		}
		installed, running, err := mc.ServerVersions()
		if err != nil || running == "" {
			httpErr(w, 400, fmt.Errorf("no herdr server runs there (%v)", err))
			return
		}
		snap, _ := s.Watch.Snapshot(mc.Cfg.ID)
		u := serverRestartUpdate(mc, running, installed, snap)
		if req.Command != "" && req.Command != u.Command {
			httpErr(w, http.StatusConflict, fmt.Errorf("the versions changed since this was shown — look again"))
			return
		}
		j := s.Jobs.Run(mc, u.Title, u.Command, func(say func(string)) error { return s.Restart.Server(mc, say) })
		writeJSON(w, map[string]any{"job": j.ID})
		return
	}
	if kind, isRestart := strings.CutPrefix(req.What, "restart-"); isRestart {
		snap, _ := s.Watch.Snapshot(mc.Cfg.ID)
		u, n := restartPlan(kind, snap)
		if s.Restart == nil || restart.Kinds(kind) == nil {
			httpErr(w, 400, fmt.Errorf("nothing to restart called %q", req.What))
			return
		}
		if n == 0 {
			httpErr(w, 400, fmt.Errorf("no %s chat with a session to resume there now", restart.Title(kind)))
			return
		}
		if req.Command != "" && req.Command != u.Command {
			httpErr(w, http.StatusConflict, fmt.Errorf("the chats changed since this was shown — look again"))
			return
		}
		j := s.Jobs.Run(mc, u.Title, u.Command, func(say func(string)) error { return s.Restart.Now(mc, []string{kind}, say) })
		writeJSON(w, map[string]any{"job": j.ID})
		return
	}
	if req.What == "remuda-release" {
		u, ok := s.releasePlan(mc, rep)
		if !ok {
			httpErr(w, 400, fmt.Errorf("no newer release of remuda is known for this machine"))
			return
		}
		if req.Command != "" && req.Command != u.Command {
			httpErr(w, http.StatusConflict, fmt.Errorf("the release changed since this was shown — look again"))
			return
		}
		j := s.Jobs.Start(mc, u.Title, u.Command)
		writeJSON(w, map[string]any{"job": j.ID})
		return
	}
	if req.What == "remuda" {
		var ok bool
		if u, ok = s.deployPlan(mc, rep); !ok {
			httpErr(w, 400, fmt.Errorf("this remuda cannot be deployed there (a remote machine of the same OS and CPU is needed)"))
			return
		}
	} else {
		var err error
		if u, err = health.Plan(req.What, rep); err != nil {
			httpErr(w, 400, err)
			return
		}
	}
	if req.Command != "" && req.Command != u.Command {
		httpErr(w, http.StatusConflict, fmt.Errorf("the update changed since it was shown — look at it again"))
		return
	}
	var j jobs.Job
	if req.What == "remuda" {
		j = s.Jobs.Run(mc, u.Title, u.Command, func(say func(string)) error { return s.deploySelf(mc, say) })
	} else {
		first, then := s.restartHooks(mc, req.What)
		if req.What == "herdr" {
			then = s.herdrThen(mc)
		}
		j = s.Jobs.StartThen(mc, u.Title, u.Command, first, then)
	}
	writeJSON(w, map[string]any{"job": j.ID})
}

// handleJob is GET /api/jobs/{id}.
func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	if s.Jobs == nil {
		httpErr(w, 404, fmt.Errorf("no such job"))
		return
	}
	j, ok := s.Jobs.Get(r.PathValue("id"))
	if !ok {
		httpErr(w, 404, fmt.Errorf("no such job"))
		return
	}
	writeJSON(w, j)
}

// deployPlan offers this very binary to a remote machine of the same OS and CPU whose remuda is behind or missing.
func (s *Server) deployPlan(mc *machines.Machine, rep *health.Report) (health.Update, bool) {
	if mc.Local || rep == nil || rep.Arch == "" || rep.Arch != runtime.GOARCH {
		return health.Update{}, false
	}
	if (runtime.GOOS == "darwin") != strings.HasPrefix(rep.OS, "macOS") {
		return health.Update{}, false
	}
	me := s.version()
	if me == "" || me == "dev" || rep.Remuda == me {
		return health.Update{}, false
	}
	have := rep.Remuda
	if have == "" {
		have = "not deployed"
	}
	return health.Update{What: "remuda", Title: "Deploy this remuda there", Command: "# this remuda binary → " + mc.Cfg.Host + ":~/.local/bin/remuda.new over the ssh connection, then there:\n" + deployScript,
		Note: have + " → " + me + ". The binary this page is served by is copied over your SSH and its service restarted; the peer relay reconnects by itself."}, true
}

// deployScript installs the copied binary and restarts the service: the two steps a release download ends with too.
const deployScript = health.InstallScript + " && " + health.RestartScript

// releasePlan offers a newer GitHub release of remuda than the machine runs, the one serving this page included.
func (s *Server) releasePlan(mc *machines.Machine, rep *health.Report) (health.Update, bool) {
	if s.Health == nil {
		return health.Update{}, false
	}
	rel := s.Health.Release()
	if mc.Local {
		return health.ReleaseUpdate(s.version(), runtime.GOOS, runtime.GOARCH, rel, true)
	}
	if rep == nil {
		return health.Update{}, false
	}
	return health.ReleaseUpdate(rep.Remuda, rep.OS, rep.Arch, rel, false)
}

// deploySelf copies this binary to the machine and restarts the service.
func (s *Server) deploySelf(mc *machines.Machine, say func(string)) error {
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
	out, err := mc.Shell(deployScript, 2*time.Minute)
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
		if out, err := mc.Shell(`curl -s -m 2 http://127.0.0.1:7317/api/health 2>/dev/null`, 10*time.Second); err == nil && strings.Contains(out, `"ok":true`) {
			say("remuda there answers: " + strings.TrimSpace(out))
			return nil
		}
	}
	say("remuda there has not answered yet; the next vitals read will tell")
	return nil
}

func (s *Server) version() string {
	if s.Health != nil {
		return s.Health.Version
	}
	return ""
}
