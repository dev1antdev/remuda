package transcript

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/text"
)

// Running is an agent process whose session is looked up, with what tells it from a later process given its pid.
type Running struct {
	PID     int
	Agent   string
	CWD     string
	Seen    time.Time
	Started int64
}

// SessionsOf answers, pid by pid, the session each running Claude Code or Codex is in, read off its process.
func (r *Reader) SessionsOf(procs []Running) (map[int]string, error) {
	lookup := sessionsByPIDScript(procs)
	if lookup == "" {
		return map[int]string{}, nil
	}
	var clock machineClock
	if slices.ContainsFunc(procs, func(p Running) bool { return p.Started == 0 && !p.Seen.IsZero() }) {
		read, err := r.machineClock()
		if err != nil {
			return nil, err
		}
		clock = read
	}
	out, err := shell.Text(r.host, lookup, 15*time.Second)
	if err != nil {
		return nil, err
	}
	return parseSessionsByPID(out, procs, rpath.For(r.host.Home()), clock), nil
}

// maxClockSpan is the longest a reading of the machine's clock may take (an assumption, unmeasured on a Windows PC).
var maxClockSpan = 2 * time.Second

// machineClock is the machine's clock read in a call of its own: asked here, no later than latest there; zero, none.
type machineClock struct {
	asked, latest time.Time
}

// machineClock reads the machine's clock, refusing a reading whose call took over maxClockSpan.
func (r *Reader) machineClock() (machineClock, error) {
	asked := time.Now()
	out, err := shell.Text(r.host, "date +%s", 10*time.Second)
	if err != nil {
		return machineClock{}, err
	}
	if took := time.Since(asked); took > maxClockSpan {
		return machineClock{}, fmt.Errorf("the machine took %s to tell its time, too long to tell its processes apart", took.Round(time.Millisecond))
	}
	secs, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil {
		return machineClock{}, fmt.Errorf("the machine tells its time as %q", strings.TrimSpace(out))
	}
	return machineClock{asked: asked, latest: time.Unix(secs+1, 0)}, nil
}

// begunBy says whether a process with that start can be the one seen running here at seen: it began by then. Placed by
// monotonic time, a step of this clock moves nothing; a step of the machine's moves ps's lstart with it on Linux
// alone, so on macOS and Windows a forward step between the sighting and the read can let a later process through.
func (c machineClock) begunBy(start string, seen time.Time) bool {
	if c.asked.IsZero() || seen.IsZero() {
		return true
	}
	began, known := startedMS(start)
	return known && !time.UnixMilli(began).After(c.latest.Add(-c.asked.Sub(seen)))
}

// sessionsByPIDScript prints the system, then per process its pid, agent, start, folder, session files and start again.
func sessionsByPIDScript(procs []Running) string {
	var b strings.Builder
	for _, p := range procs {
		if p.PID <= 1 || (p.Agent != "claude" && p.Agent != "codex") {
			continue
		}
		b.WriteString("s " + strconv.Itoa(p.PID) + " " + p.Agent + " " + script.Quote(p.CWD) + "\n")
	}
	if b.Len() == 0 {
		return ""
	}
	return script.PATH + "\n" + script.CwdOf + `system=$(uname -s 2>/dev/null)
printf 'system\037%s\036\n' "$system"
case "$system" in MINGW* | MSYS* | CYGWIN*) windows=1 ;; *) windows= ;; esac
lstart() { LC_ALL=C TZ=UTC ps -o lstart= -p "$1" 2>/dev/null; }
s() {
  start=$(lstart "$1")
  if [ -z "$start" ]; then
    start=-
    if [ -n "$windows" ]; then
      ft=$(powershell.exe -NoProfile -NonInteractive -Command "(Get-Process -Id $1 -ErrorAction Stop).StartTime.ToFileTimeUtc()" 2>/dev/null | tr -d '\r')
      case "$ft" in [0-9]*) start="ft:$ft" ;; esac
    fi
  fi
  cwd=
  case "$start" in -|ft:*) ;; *) cwd_of "$1" ;; esac
  printf '%s\037%s\037%s\037%s\037' "$1" "$2" "$start" "$cwd"
  if [ "$2" = claude ]; then
    {
      if [ -r "/proc/$1/environ" ]; then
        tr '\0' '\n' < "/proc/$1/environ" | sed -n 's/^CLAUDE_CONFIG_DIR=//p' | head -n 1
      else
        ps -E -ww -o command= -p "$1" 2>/dev/null | awk '
          { key = " CLAUDE_CONFIG_DIR="; p = 0; t = $0
            while ((j = index(t, key)) > 0) { p += j; t = substr(t, j + 1) }
            if (!p) next
            v = substr($0, p + length(key)); seen = ""
            while (match(v, / [A-Za-z_][A-Za-z0-9_]*=/)) {
              print seen substr(v, 1, RSTART - 1)
              seen = seen substr(v, 1, RSTART + RLENGTH - 1); v = substr(v, RSTART + RLENGTH)
            }
            print seen v }'
      fi
      printf '%s\n' "$HOME/.claude"
    } | while IFS= read -r c; do
      case "$c" in /* | [A-Za-z]:*) ;; ?*) c="$3/$c" ;; esac
      f="$c/sessions/$1.json"
      [ -n "$c" ] && [ -f "$f" ] && { head -c 65536 "$f"; printf '\035'; }
    done
  else
    for p in $1 $(pgrep -P "$1" 2>/dev/null); do
      if [ -d "/proc/$p/fd" ]; then
        for fd in /proc/$p/fd/*; do
          f=$(readlink "$fd" 2>/dev/null) && case "${f##*/}" in rollout-*.jsonl) rollout "$p" "$f" ;; esac
        done
      else
        LC_ALL=en_US.UTF-8 lsof -nP -p "$p" -Fn 2>/dev/null | sed -n 's/^n//p' | grep '/rollout-[^/]*\.jsonl$' | sort -u |
          while IFS= read -r n; do rollout "$p" "$(printf '%b' "$n")"; done
      fi
    done
  fi
  printf '\037%s\036\n' "$(lstart "$1")"
}
rollout() {
  [ -f "$2" ] || return 0
  printf '%s\035%s\035' "$1" "$2"
  head -n 1 "$2" 2>/dev/null | tr -d '\n'
  printf '\034'
}
` + b.String()
}

// claudeSessionFile is what Claude Code keeps per running process in <config dir>/sessions/<pid>.json.
type claudeSessionFile struct {
	PID         int    `json:"pid"`
	SessionID   string `json:"sessionId"`
	CWD         string `json:"cwd"`
	ProcStart   string `json:"procStart"`
	ProcStartFt string `json:"procStartFt"` // a native Windows process's creation, a FILETIME, in place of procStart
	StartedAt   int64  `json:"startedAt"`   // unix ms, when Claude Code began: never before its process was created
}

// parseSessionsByPID reads sessionsByPIDScript's answer.
func parseSessionsByPID(out string, procs []Running, style rpath.Style, clock machineClock) map[int]string {
	cwds, seen, started := map[int]string{}, map[int]time.Time{}, map[int]int64{}
	for _, p := range procs {
		cwds[p.PID], seen[p.PID], started[p.PID] = p.CWD, p.Seen, p.Started
	}
	found, linux := map[int]string{}, false
	for _, record := range script.Sections(out) {
		if system, ok := strings.CutPrefix(strings.TrimSpace(record), "system"+script.US); ok {
			linux = system == "Linux"
			continue
		}
		fields := strings.SplitN(record, script.US, 6)
		if len(fields) < 6 {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil {
			continue
		}
		cwd, asked := cwds[pid]
		if !asked {
			continue
		}
		start, standing, payload, after := strings.TrimSpace(fields[2]), fields[3], strings.TrimSpace(fields[4]), strings.TrimSpace(fields[5])
		switch {
		case start == "-" || start == "":
			continue // gone, or nothing tells it from another process given its pid
		case standing != "" && cwd != "" && !style.Equal(standing, cwd):
			continue // the pid went to a process elsewhere since the pane was seen
		case !strings.HasPrefix(start, "ft:") && after != start:
			continue // another process took the pid while it was read, or it ended
		}
		if told := started[pid]; told != 0 && !sameCreation(start, told, linux) || told == 0 && !clock.begunBy(start, seen[pid]) {
			continue // another process than the one seen: the pid changed hands before it was read
		}
		var id string
		switch fields[1] {
		case "claude":
			id = claudeSession(payload, pid, start, cwd, style, started[pid])
		case "codex":
			id = heldThread(payload, pid)
		}
		if text.SessionID(id) {
			found[pid] = id
		}
	}
	return found
}

// claudeSession is the session of the candidate file that is this very process's, none for one nothing ties to it.
func claudeSession(payload string, pid int, start, cwd string, style rpath.Style, told int64) string {
	began, known := startedMS(start)
	creation, native := strings.CutPrefix(start, "ft:")
	created, _ := filetimeOf(creation)
	for _, doc := range strings.Split(payload, script.GS) {
		var file claudeSessionFile
		if json.Unmarshal([]byte(doc), &file) != nil || file.PID != pid || told != 0 && file.StartedAt < told {
			continue
		}
		inFolder := cwd == "" || file.CWD == "" || style.Equal(file.CWD, cwd)
		recorded, ft := filetimeOf(file.ProcStartFt)
		switch {
		case file.ProcStart != "" && !native:
			if strings.TrimSpace(file.ProcStart) == start {
				return file.SessionID
			}
		case native && ft:
			if max(created-recorded, recorded-created) < filetimeMS && inFolder {
				return file.SessionID
			}
		case known && file.StartedAt > 0:
			if file.StartedAt >= began && inFolder {
				return file.SessionID
			}
		}
	}
	return ""
}

// FILETIME counts 100 ns from 1601: a millisecond of it, and the Unix epoch in it.
const (
	filetimeMS    = 10_000
	filetimeEpoch = 116_444_736_000_000_000
)

// filetimeOf reads a FILETIME as PowerShell and Claude Code write it; false for anything else, a value from 3e17 up
// among it, which Claude Code itself reads as another unit.
func filetimeOf(s string) (int64, bool) {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return v, err == nil && v > filetimeEpoch && v < 3e17
}

// sameCreation says whether the script's start is the creation the runtime told (unix ms), reading no clock.
func sameCreation(start string, told int64, linux bool) bool {
	read, known := startedMS(start)
	if strings.HasPrefix(start, "ft:") {
		return known && read == told
	}
	second := told - told%1000 // Linux's ps counts the boot time in whole seconds, remuda-core does not: one behind
	return known && (read == second || linux && read == second-1000)
}

// startedMS is the process's creation the script told, in unix ms: PowerShell's FILETIME for a native Windows
// process, else ps's lstart read in UTC, a second's floor that the true creation is never before.
func startedMS(start string) (int64, bool) {
	if created, native := strings.CutPrefix(start, "ft:"); native {
		ft, ok := filetimeOf(created)
		return (ft - filetimeEpoch) / filetimeMS, ok
	}
	at, err := time.Parse(time.ANSIC, start)
	return at.UnixMilli(), err == nil
}

// personsThread reads a rollout's first line: whether it is the session_meta of the person's thread (no subagent's
// source, a thread_source of "user" or none), and the thread id it names. Its folder is where the thread began, which
// a resume may leave: the process holding the rollout open is what tells.
func personsThread(head string) (string, bool) {
	var record codexRecord
	var meta struct {
		ID           string          `json:"id"`
		Source       json.RawMessage `json:"source"`
		ThreadSource string          `json:"thread_source"`
	}
	if json.Unmarshal([]byte(head), &record) != nil || record.Type != "session_meta" || json.Unmarshal(record.Payload, &meta) != nil {
		return "", false
	}
	var source struct {
		Subagent json.RawMessage `json:"subagent"`
	}
	subagent := json.Unmarshal(meta.Source, &source) == nil && source.Subagent != nil
	return meta.ID, !subagent && (meta.ThreadSource == "" || meta.ThreadSource == "user")
}

// heldThread is the thread of the person's conversation among the rollouts a Codex process holds open: the
// foreground process's own first, a child's only when it holds none; "" when none or more than one is there,
// since enumeration order says nothing about which one is on screen.
func heldThread(payload string, pid int) string {
	own, children := map[string]bool{}, map[string]bool{}
	for _, entry := range strings.Split(payload, script.FS) {
		fields := strings.SplitN(entry, script.GS, 3)
		if len(fields) < 3 {
			continue
		}
		id, persons := personsThread(fields[2])
		if !persons {
			continue
		}
		if !text.SessionID(id) { // session_meta's own goes first: a revert's replacement file, <id>_<rollout id>.jsonl, keeps it
			id = CodexSessionID(fields[1])
		}
		if id == "" {
			continue
		}
		if fields[0] == strconv.Itoa(pid) {
			own[id] = true
		} else {
			children[id] = true
		}
	}
	for _, ids := range []map[string]bool{own, children} {
		if len(ids) == 1 {
			for id := range ids {
				return id
			}
		}
		if len(ids) > 1 {
			return ""
		}
	}
	return ""
}
