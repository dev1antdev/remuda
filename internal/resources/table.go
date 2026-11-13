// Package resources is what a machine's herdr panes weigh on it: every process of every pane's tree summed — CPU as
// a share of all its cores, memory as resident bytes — by workspace, with the herdr server's own beside them, for the
// status bar's "CPU 12 % · 3.8 GB". One process table a read (ps on a Mac, /proc on Linux), the panes' shells as herdr
// names them, nothing on a Windows machine yet.
package resources

import (
	"errors"
	"path"
	"strconv"
	"strings"

	"github.com/dev1antdev/remuda/internal/script"
)

// NotOnWindows is why a Windows machine's load is not read: Git Bash's ps names no process's CPU or memory.
const NotOnWindows = "the load of a Windows machine is not read yet: Git Bash's ps says nothing of a process's CPU or memory"

// Table is one reading of a machine's processes: its cores, its memory, and every process.
type Table struct {
	Linux bool  // CPU comes as cumulative ticks, Ticks a second, to be told against the reading before
	Cores int   // the logical cores
	MemKB int64 // the memory installed
	Ticks int   // clock ticks a second, on Linux
	Procs map[int]Proc
	Kids  map[int][]int
}

// Proc is one process: its parent, its CPU — percent of one core on a Mac, cumulative ticks on Linux — its resident
// memory and its program's name.
type Proc struct {
	PID, PPID int
	CPU       float64
	RSSKB     int64
	Name      string
}

// Script prints the machine's cores and memory as key and value lines, a section break, then a line per process:
// pid, parent, CPU, resident KB and its name. /proc on Linux, whose ps tells a process's CPU over its whole life
// rather than now; ps elsewhere, a Mac's being a recent average already.
func Script() string {
	return script.PATH + `
if [ "$(uname)" = Linux ] && [ -r /proc/self/stat ]; then
  printf 'kind\037linux\n'
  printf 'cores\037%s\n' "$(getconf _NPROCESSORS_ONLN 2>/dev/null || nproc 2>/dev/null)"
  printf 'ticks\037%s\n' "$(getconf CLK_TCK 2>/dev/null)"
  page=$(getconf PAGESIZE 2>/dev/null || echo 4096)
  awk '/^MemTotal:/ { printf "mem\037%s\n", $2 }' /proc/meminfo 2>/dev/null
  printf '\036\n'
  cat /proc/[0-9]*/stat 2>/dev/null | awk -v page="$page" '{
    if (!match($0, /\(.*\)/)) next
    name = substr($0, RSTART + 1, RLENGTH - 2)
    n = split(substr($0, RSTART + RLENGTH + 1), f, " ")
    if (n < 22) next
    printf "%s\037%s\037%s\037%d\037%s\n", $1, f[2], f[12] + f[13], f[22] * page / 1024, name
  }'
else
  printf 'kind\037ps\n'
  printf 'cores\037%s\n' "$(sysctl -n hw.ncpu 2>/dev/null)"
  printf 'mem\037%s\n' "$(sysctl -n hw.memsize 2>/dev/null | awk '{ printf "%d", $1 / 1024 }')"
  printf '\036\n'
  ps -A -o pid= -o ppid= -o pcpu= -o rss= -o comm= 2>/dev/null | awk '{
    pid = $1; ppid = $2; cpu = $3; rss = $4
    sub(/^[ \t]*[^ \t]+[ \t]+[^ \t]+[ \t]+[^ \t]+[ \t]+[^ \t]+[ \t]+/, "")
    printf "%s\037%s\037%s\037%s\037%s\n", pid, ppid, cpu, rss, $0
  }'
fi
`
}

// Parse reads the script's answer into a table; an answer with no process in it is an error.
func Parse(out string) (Table, error) {
	sections := script.Sections(out)
	if len(sections) < 2 {
		return Table{}, errors.New("the machine's process table did not come")
	}
	t := Table{Procs: map[int]Proc{}, Kids: map[int][]int{}}
	for _, line := range script.Lines(sections[0]) {
		f := script.Fields(line)
		if len(f) != 2 {
			continue
		}
		value := strings.TrimSpace(f[1])
		switch f[0] {
		case "kind":
			t.Linux = value == "linux"
		case "cores":
			t.Cores, _ = strconv.Atoi(value)
		case "mem":
			t.MemKB, _ = strconv.ParseInt(value, 10, 64)
		case "ticks":
			t.Ticks, _ = strconv.Atoi(value)
		}
	}
	if t.Cores < 1 {
		t.Cores = 1
	}
	if t.Linux && t.Ticks < 1 {
		t.Ticks = 100
	}
	for _, line := range script.Lines(sections[1]) {
		f := script.Fields(line)
		if len(f) != 5 {
			continue
		}
		pid, err1 := strconv.Atoi(strings.TrimSpace(f[0]))
		ppid, err2 := strconv.Atoi(strings.TrimSpace(f[1]))
		cpu, err3 := strconv.ParseFloat(strings.TrimSpace(f[2]), 64)
		rss, err4 := strconv.ParseInt(strings.TrimSpace(f[3]), 10, 64)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			continue
		}
		t.Procs[pid] = Proc{PID: pid, PPID: ppid, CPU: cpu, RSSKB: rss, Name: programName(f[4])}
		t.Kids[ppid] = append(t.Kids[ppid], pid)
	}
	if len(t.Procs) == 0 {
		return Table{}, errors.New("the machine's process table is empty")
	}
	return t, nil
}

// programName is a process's program as a person knows it: a Mac's ps names the whole path, Linux the name alone
func programName(name string) string {
	name = strings.TrimSpace(name)
	if strings.Contains(name, "/") {
		name = path.Base(name)
	}
	return name
}
