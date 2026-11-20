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

// SessionsOf finds the session each running agent is in, by its process, in one script on the machine:
// Claude Code's id from <config dir>/sessions/<pid>.json (CLAUDE_CONFIG_DIR read off the process's environment;
// macOS's `ps -E` runs the entries together, so its text gives candidates and the one holding the file wins),
// Codex's thread id from the rollout of the person's thread the process (else a child of it) holds open, none
// when two are open. An agent whose session is not found is left out; a session changes under the same pid
// (/clear, /resume), so the caller asks again as it sees fit.
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

// sessionsByPIDScript prints a record per process: pid, agent, the process's start as Claude Code records it
// (`ps -o lstart` in UTC), then the session file's text for Claude Code or, for Codex, a line per open rollout:
// the pid holding it, whether its session_meta is the person's thread (1) or a subagent's (0), its path.
func sessionsByPIDScript(procs []Running) string {
	var b strings.Builder
	for _, p := range procs {
		if p.PID <= 1 || (p.Agent != "claude" && p.Agent != "codex") {
			continue
		}
		b.WriteString("s " + strconv.Itoa(p.PID) + " " + p.Agent + "\n")
	}
	if b.Len() == 0 {
		return ""
	}
	return `PATH="$PATH:/usr/sbin:/sbin"
s() {
  start=$(LC_ALL=C TZ=UTC ps -o lstart= -p "$1" 2>/dev/null)
  printf '%s\037%s\037%s\037' "$1" "$2" "$start"
  if [ "$2" = claude ]; then
    if [ -r "/proc/$1/environ" ]; then
      cfg=$(tr '\0' '\n' < "/proc/$1/environ" | sed -n 's/^CLAUDE_CONFIG_DIR=//p' | head -n 1)
    else
      cfg=$(ps -E -ww -o command= -p "$1" 2>/dev/null | awk '
        { key = " CLAUDE_CONFIG_DIR="; p = 0; t = $0
          while ((j = index(t, key)) > 0) { p += j; t = substr(t, j + 1) }
          if (!p) next
          v = substr($0, p + length(key)); seen = ""
          while (match(v, / [A-Za-z_][A-Za-z0-9_]*=/)) {
            print seen substr(v, 1, RSTART - 1)
            seen = seen substr(v, 1, RSTART + RLENGTH - 1); v = substr(v, RSTART + RLENGTH)
          }
          print seen v }' | while IFS= read -r c; do [ -f "$c/sessions/$1.json" ] && { printf '%s' "$c"; break; }; done)
    fi
    f="${cfg:-$HOME/.claude}/sessions/$1.json"
    [ -f "$f" ] && head -c 65536 "$f"
  else
    for p in $1 $(pgrep -P "$1" 2>/dev/null); do
      if [ -d "/proc/$p/fd" ]; then
        for fd in /proc/$p/fd/*; do readlink "$fd" 2>/dev/null; done
      else
        lsof -p "$p" -Fn 2>/dev/null | sed -n 's/^n//p'
      fi | grep '/rollout-[^/]*\.jsonl$' | sort -u | while IFS= read -r f; do
        [ -f "$f" ] || continue
        head=$(head -n 1 "$f" 2>/dev/null | head -c 65536)
        case "$head" in
          *'"source":{"subagent"'*) own=0 ;;
          *'"thread_source":"user"'*) own=1 ;;
          *'"thread_source":"'*) own=0 ;;
          *) own=1 ;;
        esac
        printf '%s\035%s\035%s\n' "$p" "$own" "$f"
      done
    done
  fi
  printf '\036\n'
}
` + b.String()
}

// claudeSessionFile is what Claude Code keeps per running process in <config dir>/sessions/<pid>.json.
type claudeSessionFile struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd"`
	ProcStart string `json:"procStart"`
}

// parseSessionsByPID reads sessionsByPIDScript's answer. A Claude Code file counts only when it is this very
// process's: its pid, and its start time when both sides know it, else its folder; a leftover of an ended
// process whose pid was given to another is not.
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
		var id string
		switch fields[1] {
		case "claude":
			var file claudeSessionFile
			if json.Unmarshal([]byte(payload), &file) != nil || file.PID != pid {
				continue
			}
			if file.ProcStart != "" && start != "" {
				if strings.TrimSpace(file.ProcStart) != start {
					continue
				}
			} else if cwd != "" && file.CWD != "" && !style.Equal(file.CWD, cwd) {
				continue
			}
			id = file.SessionID
		case "codex":
			id = heldThread(payload, pid)
		}
		if text.SessionID(id) {
			found[pid] = id
		}
	}
	return found
}

// heldThread is the thread of the person's conversation among the rollouts a Codex process holds open: the
// foreground process's own first, a child's only when it holds none; "" when none or more than one is there,
// since enumeration order says nothing about which one is on screen.
func heldThread(payload string, pid int) string {
	own, children := map[string]bool{}, map[string]bool{}
	for _, line := range strings.Split(payload, "\n") {
		fields := strings.SplitN(line, script.GS, 3)
		if len(fields) < 3 || fields[1] != "1" {
			continue
		}
		id := CodexSessionID(fields[2])
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
