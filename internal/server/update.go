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

	"github.com/dev1antdev/herdr-ui/internal/health"
	"github.com/dev1antdev/herdr-ui/internal/jobs"
	"github.com/dev1antdev/herdr-ui/internal/machines"
)

// Updates from the page: herdr, the agents, and herdr-ui itself on a
// remote machine — each a job the person first sees as a command, then
// watches run. Everything runs as the user through the package manager
// that installed the thing; nothing asks for sudo.

// GET /api/machines/{id}/updates — what this machine can update now.
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
	if u, ok := s.deployPlan(mc, rep); ok {
		ups = append(ups, u)
	}
	writeJSON(w, map[string]any{"updates": ups})
}

// POST /api/machines/{id}/update {what} — start it; the answer is the job.
func (s *Server) handleMachineUpdate(w http.ResponseWriter, r *http.Request) {
	mc, err := s.Machines.Get(r.PathValue("id"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	var req struct {
		What    string `json:"what"`
		Command string `json:"command"` // the one the page showed; refused when the plan changed since
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpErr(w, 400, err)
		return
	}
	if s.Jobs == nil || s.Health == nil {
		httpErr(w, 503, fmt.Errorf("updates are off"))
		return
	}
	rep := s.Health.Report(mc.Cfg.ID)
	var u health.Update
	if req.What == "herdr-ui" {
		var ok bool
		if u, ok = s.deployPlan(mc, rep); !ok {
			httpErr(w, 400, fmt.Errorf("this herdr-ui cannot be deployed there (a remote machine of the same OS and CPU is needed)"))
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
	if req.What == "herdr-ui" {
		j = s.Jobs.Run(mc, u.Title, u.Command, func(say func(string)) error { return s.deploySelf(mc, say) })
	} else {
		j = s.Jobs.Start(mc, u.Title, u.Command)
	}
	writeJSON(w, map[string]any{"job": j.ID})
}

// GET /api/jobs/{id}
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

// deployPlan: this very binary can go to a remote machine of the same OS
// and CPU when its herdr-ui is behind (or missing).
func (s *Server) deployPlan(mc *machines.Machine, rep *health.Report) (health.Update, bool) {
	if mc.Local || rep == nil || rep.Arch == "" || rep.Arch != runtime.GOARCH {
		return health.Update{}, false
	}
	if (runtime.GOOS == "darwin") != strings.HasPrefix(rep.OS, "macOS") {
		return health.Update{}, false
	}
	me := s.version()
	if me == "" || me == "dev" || rep.HerdrUI == me {
		return health.Update{}, false
	}
	have := rep.HerdrUI
	if have == "" {
		have = "not deployed"
	}
	return health.Update{What: "herdr-ui", Title: "Deploy this herdr-ui there", Command: "# this herdr-ui binary → " + mc.Cfg.Host + ":~/.local/bin/herdr-ui.new over the ssh connection, then there:\n" + deployScript,
		Note: have + " → " + me + ". The binary this page is served by is copied over your SSH and its service restarted; the peer relay reconnects by itself."}, true
}

// deployScript installs the copied binary and restarts the service — the
// part of a deploy that runs as a shell script on the machine.
const deployScript = `chmod +x "$HOME/.local/bin/herdr-ui.new" && mv "$HOME/.local/bin/herdr-ui.new" "$HOME/.local/bin/herdr-ui" && "$HOME/.local/bin/herdr-ui" -version && ` +
	`if command -v launchctl >/dev/null 2>&1; then launchctl kickstart -k "gui/$(id -u)/dev.mardeus.herdr-ui" 2>&1 || echo "(no launchd service; start herdr-ui there yourself)"; ` +
	`elif command -v systemctl >/dev/null 2>&1; then systemctl --user restart herdr-ui.service 2>&1 || echo "(no systemd service; start herdr-ui there yourself)"; fi`

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
	say(fmt.Sprintf("copying %s (%.1f MB) to %s:~/.local/bin/herdr-ui.new…", exe, float64(len(data))/1048576, mc.Cfg.Host))
	conn, err := mc.RemoteConn()
	if err != nil {
		return err
	}
	if err := conn.Put(mc.Home()+"/.local/bin/herdr-ui.new", data, 5*time.Minute); err != nil {
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
	// the job ends when the new herdr-ui answers there, so the vitals read
	// that follows finds the peer again
	say("waiting for herdr-ui there…")
	for i := 0; i < 30; i++ {
		time.Sleep(time.Second)
		if out, err := mc.Shell(`curl -s -m 2 http://127.0.0.1:7317/api/health 2>/dev/null`, 10*time.Second); err == nil && strings.Contains(out, `"ok":true`) {
			say("herdr-ui there answers: " + strings.TrimSpace(out))
			return nil
		}
	}
	say("herdr-ui there has not answered yet; the next vitals read will tell")
	return nil
}

func (s *Server) version() string {
	if s.Health != nil {
		return s.Health.Version
	}
	return ""
}
