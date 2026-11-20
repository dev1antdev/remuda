package transcript

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/text"
)

// Running is an agent process whose session is looked up: its pid, its kind (claude, codex) and its folder.
type Running struct {
	PID   int
	Agent string
	CWD   string
}

// SessionsOf answers, pid by pid, the session each running Claude Code or Codex is in, read off its process.
func (r *Reader) SessionsOf(procs []Running) (map[int]string, error) {
	lookup := sessionsByPIDScript(procs)
	if lookup == "" {
		return map[int]string{}, nil
	}
	out, err := shell.Text(r.host, lookup, 15*time.Second)
	if err != nil {
		return nil, err
	}
	return parseSessionsByPID(out, procs, rpath.For(r.host.Home())), nil
}

// sessionsByPIDScript prints a record per process, split by US:
//   - its pid and agent;
//   - its start as Claude Code records it: `ps -o lstart` in UTC, "" for a live process ps does not tell of,
//     "ft:<FILETIME>" from PowerShell's Get-Process for a native Windows process (asked on Git Bash alone), "-" for
//     one gone;
//   - for Claude Code, every candidate sessions/<pid>.json, each ended by GS: under the CLAUDE_CONFIG_DIR its
//     environment names (macOS's `ps -E` runs the entries together, so each reading is a candidate; a relative one
//     from the process's folder), then the default ~/.claude;
//   - for Codex, a line per rollout the process or a child holds open: the pid holding it, its path and its first
//     line whole, split by GS.
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
	return script.PATH + `
case "$(uname -s 2>/dev/null)" in MINGW* | MSYS* | CYGWIN*) windows=1 ;; *) windows= ;; esac
s() {
  start=$(LC_ALL=C TZ=UTC ps -o lstart= -p "$1" 2>/dev/null)
  if [ -z "$start" ] && ! kill -0 "$1" 2>/dev/null; then
    start=-
    if [ -n "$windows" ]; then
      ft=$(powershell.exe -NoProfile -NonInteractive -Command "(Get-Process -Id $1 -ErrorAction Stop).StartTime.ToFileTimeUtc()" 2>/dev/null | tr -d '\r')
      case "$ft" in [0-9]*) start="ft:$ft" ;; esac
    fi
  fi
  printf '%s\037%s\037%s\037' "$1" "$2" "$start"
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
        for fd in /proc/$p/fd/*; do readlink "$fd" 2>/dev/null; done
      else
        lsof -nP -p "$p" -Fn 2>/dev/null | sed -n 's/^n//p'
      fi | grep '/rollout-[^/]*\.jsonl$' | sort -u | while IFS= read -r f; do
        [ -f "$f" ] || continue
        printf '%s\035%s\035' "$p" "$f"
        head -n 1 "$f" 2>/dev/null | tr -d '\n'
        printf '\n'
      done
    done
  fi
  printf '\036\n'
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
func parseSessionsByPID(out string, procs []Running, style rpath.Style) map[int]string {
	cwds := map[int]string{}
	for _, p := range procs {
		cwds[p.PID] = p.CWD
	}
	found := map[int]string{}
	for _, record := range script.Sections(out) {
		fields := strings.SplitN(record, script.US, 4)
		if len(fields) < 4 {
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
		start, payload := strings.TrimSpace(fields[2]), strings.TrimSpace(fields[3])
		if start == "-" {
			continue
		}
		var id string
		switch fields[1] {
		case "claude":
			id = claudeSession(payload, pid, start, cwd, style)
		case "codex":
			id = heldThread(payload, pid, cwd, style)
		}
		if text.SessionID(id) {
			found[pid] = id
		}
	}
	return found
}

// claudeSession is the session of a candidate file that is this very process's: of its folder when both name one
// (an agent elsewhere holds the pid now), and one whose procStart is the process's start when both sides know it;
// for a native Windows process one whose procStartFt is its creation, to the millisecond; else, while the process's
// start is known, one that began no earlier; else the first. A file left by an ended process whose pid was given to
// another is not.
func claudeSession(payload string, pid int, start, cwd string, style rpath.Style) string {
	began, known := startedMS(start)
	creation, native := strings.CutPrefix(start, "ft:")
	created, _ := filetimeOf(creation)
	var byFolder string
	for _, doc := range strings.Split(payload, script.GS) {
		var file claudeSessionFile
		if json.Unmarshal([]byte(doc), &file) != nil || file.PID != pid {
			continue
		}
		if cwd != "" && file.CWD != "" && !style.Equal(file.CWD, cwd) {
			continue
		}
		recorded, ft := filetimeOf(file.ProcStartFt)
		switch {
		case file.ProcStart != "" && start != "" && !native:
			if strings.TrimSpace(file.ProcStart) == start {
				return file.SessionID
			}
		case native && ft:
			if max(created-recorded, recorded-created) < filetimeMS {
				return file.SessionID
			}
		case known && file.StartedAt > 0:
			if file.StartedAt >= began {
				return file.SessionID
			}
		case !native && byFolder == "":
			byFolder = file.SessionID
		}
	}
	return byFolder
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
// source, a thread_source of "user" or none), the thread id it names and the folder it was started in.
func personsThread(head string) (id, cwd string, persons bool) {
	var record codexRecord
	var meta struct {
		ID           string          `json:"id"`
		Cwd          string          `json:"cwd"`
		Source       json.RawMessage `json:"source"`
		ThreadSource string          `json:"thread_source"`
	}
	if json.Unmarshal([]byte(head), &record) != nil || record.Type != "session_meta" || json.Unmarshal(record.Payload, &meta) != nil {
		return "", "", false
	}
	var source struct {
		Subagent json.RawMessage `json:"subagent"`
	}
	subagent := json.Unmarshal(meta.Source, &source) == nil && source.Subagent != nil
	return meta.ID, meta.Cwd, !subagent && (meta.ThreadSource == "" || meta.ThreadSource == "user")
}

// heldThread is the thread of the person's conversation among the rollouts a Codex process holds open, one started in
// the folder asked about when both name one: the foreground process's own first, a child's only when it holds none;
// "" when none or more than one is there, since enumeration order says nothing about which one is on screen.
func heldThread(payload string, pid int, cwd string, style rpath.Style) string {
	own, children := map[string]bool{}, map[string]bool{}
	for _, line := range script.Lines(payload) {
		fields := strings.SplitN(line, script.GS, 3)
		if len(fields) < 3 {
			continue
		}
		id, started, persons := personsThread(fields[2])
		if !persons || cwd != "" && started != "" && !style.Equal(started, cwd) {
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
