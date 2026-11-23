package transcript

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

const rolloutID = "01a1f692-08d7-70f0-b846-42b3f2f3c1da"

// TestHelperHoldsFile is not a test: run as a child process with REMUDA_TEST_READY set, it holds every file of
// REMUDA_TEST_HOLD open until it is killed, as Codex holds its rollouts, and keeps its environment where `ps` and
// /proc show it, as Claude Code's is read. It writes REMUDA_TEST_READY once they are open.
func TestHelperHoldsFile(t *testing.T) {
	ready := os.Getenv("REMUDA_TEST_READY")
	if ready == "" {
		return
	}
	for _, path := range filepath.SplitList(os.Getenv("REMUDA_TEST_HOLD")) {
		f, err := os.OpenFile(path, os.O_RDWR, 0) // read-write: a FIFO opens without waiting for a writer
		if err != nil {
			os.Exit(2)
		}
		defer f.Close()
	}
	if err := os.WriteFile(ready, nil, 0o600); err != nil {
		os.Exit(2)
	}
	time.Sleep(time.Minute)
	os.Exit(0)
}

// holder starts the helper in dir holding holds open, with env; under a shell (a wrapper such as npm's) holding
// shellHolds open itself when asked. It answers the pid the lookup is asked about once every file is open.
func holder(t *testing.T, dir string, holds []string, env []string, underShell bool, shellHolds string) int {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the lookup reads ps, /proc and lsof")
	}
	ready := filepath.Join(t.TempDir(), "ready")
	run := "-test.run=^TestHelperHoldsFile$"
	cmd := exec.Command(os.Args[0], run)
	if underShell {
		line := `"$0" "$1" & wait`
		if shellHolds != "" {
			line = `exec 3<"$2"; ` + line
		}
		cmd = exec.Command("sh", "-c", line, os.Args[0], run, shellHolds)
	}
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), "REMUDA_TEST_HOLD="+strings.Join(holds, string(os.PathListSeparator)), "REMUDA_TEST_READY="+ready), env...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = exec.Command("pkill", "-P", strconv.Itoa(cmd.Process.Pid)).Run()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(ready); err == nil {
			return cmd.Process.Pid
		}
		if time.Now().After(deadline) {
			t.Fatal("the holder never opened its files")
		}
	}
}

// creationOf is a process's creation as remuda-core tells it: on Linux its birth to the millisecond, read the way the
// lookup reads it; elsewhere ps's second.
func creationOf(t *testing.T, pid int, began time.Time) int64 {
	t.Helper()
	if runtime.GOOS != "linux" {
		return began.UnixMilli()
	}
	read := `before=$(date +%s%3N); read -r up _ < /proc/uptime; after=$(date +%s%3N); ` +
		`printf 'Linux\037%s\037%s\037%s\037%s\037%s\036' "$(sed -n 's/^btime //p' /proc/stat)" "$up" "$before" "$after" "$(getconf CLK_TCK)"; ` +
		`sed 's/.*) //' /proc/` + strconv.Itoa(pid) + `/stat | cut -d' ' -f20`
	out, err := exec.Command("sh", "-c", read).Output()
	system, ticks, _ := strings.Cut(string(out), script.RS)
	boot := bootOf(system)
	since, errTicks := strconv.ParseInt(strings.TrimSpace(ticks), 10, 64)
	if err != nil || errTicks != nil || boot.hz == 0 {
		t.Fatalf("this process's birth: %q, %v", out, err)
	}
	return (boot.earliest+boot.latest)/2 + since*1000/boot.hz
}

// named is the processes an answer names a session for, leaving out the ones it says have none.
func named(got map[int]string) map[int]string {
	out := map[int]string{}
	for pid, id := range got {
		if id != "" {
			out[pid] = id
		}
	}
	return out
}

// realDir is a folder of the test's own as a process standing in it is told: a Mac's temp folder is a link.
func realDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// procStart is the process's start the way Claude Code records it in its session file.
func procStart(t *testing.T, pid int) string {
	t.Helper()
	cmd := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid))
	cmd.Env = append(os.Environ(), "LC_ALL=C", "TZ=UTC")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ps: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func writeSessionFile(t *testing.T, dir string, pid int, id, cwd, start string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"pid":%d,"sessionId":%q,"cwd":%q,"startedAt":1794994322985,"procStart":%q,"version":"2.1.328","kind":"interactive","entrypoint":"cli"}`, pid, id, cwd, start)
	if err := os.WriteFile(filepath.Join(dir, strconv.Itoa(pid)+".json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeSessionComesFromItsConfigDirAndItsStart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	config := filepath.Join(home, "Application Support", "Claude PROFILE=work")
	work := realDir(t)
	pid := holder(t, work, nil, []string{"CLAUDE_CONFIG_DIR=" + config, "AFTER_IT=1"}, false, "")
	start := procStart(t, pid)
	writeSessionFile(t, filepath.Join(config, "sessions"), pid, "4c9d6f0e-1111-4222-8333-944455556666", "/work/api", start)
	writeSessionFile(t, filepath.Join(home, ".claude", "sessions"), pid, "0000aaaa-default-dir-not-this-one", "/work/api", start)
	// a file an ended process left in the folder the value's first words name, under the same pid
	writeSessionFile(t, filepath.Join(home, "Application Support", "Claude", "sessions"), pid, "1111bbbb-stale-not-this-one", "/work/api", "")

	reader := NewReader(remoteHere(home))
	got, err := reader.SessionsOf([]Running{{PID: pid, Agent: "claude", CWD: work}})
	if err != nil {
		t.Fatal(err)
	}
	if got[pid] != "4c9d6f0e-1111-4222-8333-944455556666" {
		t.Fatalf("session = %q, want the one in CLAUDE_CONFIG_DIR", got[pid])
	}
	// the transcript behind that id lives under the config dir's own projects: the file is found there, not only the id
	projects := filepath.Join(config, "projects", EncodePath("/work/api"))
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptPath := filepath.Join(projects, "4c9d6f0e-1111-4222-8333-944455556666.jsonl")
	if err := os.WriteFile(transcriptPath, []byte("{\"type\":\"user\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	defaultProjects := filepath.Join(home, ".claude", "projects", EncodePath("/work/api"))
	if err := os.MkdirAll(defaultProjects, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defaultProjects, "4c9d6f0e-1111-4222-8333-944455556666.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if path, err := reader.ResolveSession("claude", "/work/api", "id", "4c9d6f0e-1111-4222-8333-944455556666"); err != nil || path != transcriptPath {
		t.Fatalf("session file = %q, %v; want %s", path, err, transcriptPath)
	}
	// the pane was seen in another folder: the pid went to this process since
	if got, err := reader.SessionsOf([]Running{{PID: pid, Agent: "claude", CWD: filepath.Join(work, "elsewhere")}}); err != nil || len(named(got)) != 0 {
		t.Fatalf("a process standing in another folder than the pane's: got %v, %v", got, err)
	}
}

// A folder named past ASCII, or with a backslash in its name, is the same folder to the lookup as to the core,
// whatever the machine's locale — lsof writes é as \xc3\xa9 in C's, and a backslash doubled in any — for Claude Code's
// folder and for the rollout Codex holds open under a home so named alike.
func TestAnAgentInAFolderNamedPastASCIIIsFound(t *testing.T) {
	for _, name := range []string{"café", `back\slash`} {
		t.Run(name, func(t *testing.T) {
			home := filepath.Join(realDir(t), name)
			if err := os.MkdirAll(home, 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HOME", home)
			t.Setenv("LC_ALL", "C")
			claude := holder(t, home, nil, nil, false, "")
			writeSessionFile(t, filepath.Join(home, ".claude", "sessions"), claude, "4c9d6f0e-1111-4222-8333-944455556666", home, procStart(t, claude))
			codex := holder(t, home, []string{writeRollout(t, home, rolloutID, false)}, nil, false, "")
			got, err := NewReader(remoteHere(home)).SessionsOf([]Running{{PID: claude, Agent: "claude", CWD: home}, {PID: codex, Agent: "codex", CWD: home}})
			if err != nil {
				t.Fatal(err)
			}
			if got[claude] != "4c9d6f0e-1111-4222-8333-944455556666" || got[codex] != rolloutID {
				t.Fatalf("got %v in a folder named %s", got, name)
			}
		})
	}
}

// A rollout under a folder whose name holds a line break is found whole: lsof writes the break as \n, /proc as it is.
func TestACodexRolloutUnderAFolderWithALineBreakIsFound(t *testing.T) {
	home := filepath.Join(realDir(t), "line\nbreak")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	codex := holder(t, home, []string{writeRollout(t, home, rolloutID, false)}, nil, false, "")
	got, err := NewReader(remoteHere(home)).SessionsOf([]Running{{PID: codex, Agent: "codex", CWD: home}})
	if err != nil {
		t.Fatal(err)
	}
	if got[codex] != rolloutID {
		t.Fatalf("got %v under a folder with a line break", got)
	}
}

// A folder whose name ends in a line break is told whole: a shell's command substitution would drop it.
func TestAnAgentInAFolderEndingInALineBreakIsFound(t *testing.T) {
	home := realDir(t)
	t.Setenv("HOME", home)
	work := filepath.Join(home, "ends\n")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	pid := holder(t, work, nil, nil, false, "")
	writeSessionFile(t, filepath.Join(home, ".claude", "sessions"), pid, "4c9d6f0e-1111-4222-8333-944455556666", work, procStart(t, pid))
	got, err := NewReader(remoteHere(home)).SessionsOf([]Running{{PID: pid, Agent: "claude", CWD: work}})
	if err != nil {
		t.Fatal(err)
	}
	if got[pid] != "4c9d6f0e-1111-4222-8333-944455556666" {
		t.Fatalf("session = %q in a folder ending in a line break", got[pid])
	}
}

// On Linux the process's birth is read to the millisecond, as remuda-core reads it: a later process given the pid within
// the same second is another, whatever its own records say, and a reading of the boot too slow to tell them apart
// tells nothing; no machine clock is read for it.
func TestALinuxProcessIsKnownByItsBirthToTheMillisecond(t *testing.T) {
	const start = "Thu Nov 19 00:36:10 2026"
	boot := time.Date(2026, 11, 19, 0, 0, 0, 708_000_000, time.UTC) // btime counts whole seconds: :00
	read := time.Date(2026, 11, 19, 0, 40, 0, 0, time.UTC)          // when /proc/uptime was read
	system := func(before, after time.Duration) string {
		up := float64(read.Sub(boot).Truncate(10*time.Millisecond)) / float64(time.Second)
		return "system" + script.US + "Linux" + script.US + strconv.FormatInt(boot.Unix(), 10) + script.US +
			strconv.FormatFloat(up, 'f', 2, 64) + script.US + strconv.FormatInt(read.Add(-before).UnixMilli(), 10) + script.US +
			strconv.FormatInt(read.Add(after).UnixMilli(), 10) + script.US + "100" + script.RS + "\n"
	}
	record := func(ticks int64) string {
		start := boot.Truncate(time.Second).Add(time.Duration(ticks) * 10 * time.Millisecond).Format(time.ANSIC) // as ps tells it
		return "7" + script.US + "claude" + script.US + start + script.US + "/a" + script.US +
			`{"pid":7,"sessionId":"id-1","cwd":"/a","procStart":"` + start + `","startedAt":1795048571000}` + script.US +
			start + script.US + strconv.FormatInt(ticks, 10) + script.RS + "\n"
	}
	seen := boot.Add(2169400 * time.Millisecond) // the core saw it born at 00:36:10.108
	for _, c := range []struct {
		name          string
		before, after time.Duration // how long before and after the uptime read the real clock was read
		ticks         int64         // the start since boot, hundredths
		want          string
		unsure        bool
	}{
		{"the very process", time.Millisecond, time.Millisecond, 216940, "id-1", false},
		{"the very process, the real clock read 15 ms late", time.Millisecond, 15 * time.Millisecond, 216940, "id-1", false},
		{"the very process, the real clock read 15 ms early", 15 * time.Millisecond, time.Millisecond, 216940, "id-1", false},
		{"another born 400 ms later in the same second", time.Millisecond, time.Millisecond, 216980, "", false},
		{"another born 60 ms later", time.Millisecond, time.Millisecond, 216946, "", false},
		{"another born 400 ms later, the boot read over 500 ms", 500 * time.Millisecond, time.Millisecond, 216980, "", true},
		{"the very process, the boot read over 100 ms", time.Millisecond, 100 * time.Millisecond, 216940, "", true},
		{"another born two seconds later, the boot read over 100 ms", time.Millisecond, 100 * time.Millisecond, 217140, "", false},
		{"another born 400 ms later, the clock stepped back between its readings", -50 * time.Millisecond, -50 * time.Millisecond, 216980, "", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			procs := []Running{{PID: 7, Agent: "claude", CWD: "/a", Seen: seen, Started: seen.UnixMilli()}}
			never := machineClock{asked: seen.Add(time.Hour), latest: seen.Add(-time.Hour)}
			got, _, unsure := parseSessionsByPID(system(c.before, c.after)+record(c.ticks), procs, rpath.Style{}, never)
			if got[7] != c.want || unsure[7] != c.unsure {
				t.Fatalf("got %v, unsure %v, want %q", got, unsure, c.want)
			}
		})
	}
	slow := &shell.Fake{Name: "gog", Remote: true, Answer: func(string) (string, error) {
		return system(time.Millisecond, 100*time.Millisecond) + record(216940), nil
	}}
	got, err := NewReader(slow).SessionsOf([]Running{{PID: 7, Agent: "claude", CWD: "/a", Seen: seen, Started: seen.UnixMilli()}})
	if _, told := got[7]; err == nil || told {
		t.Fatalf("a boot read too slowly to tell: got %v, %v", got, err)
	}
	for _, date := range []string{"%3N", "1795048800"} {
		noMillis := "Linux" + script.US + "1795046400" + script.US + "12.34" + script.US + date + script.US + date + script.US + "100"
		if b := bootOf(noMillis); b.hz != 0 || !b.linux {
			t.Fatalf("a date telling no milliseconds (%s) read as a boot: %+v", date, b)
		} else if fits, sure := b.mayBe("216940", seen.UnixMilli()); fits || sure {
			t.Fatalf("a Linux with no boot told settled a process: fits %v, sure %v", fits, sure)
		}
	}
}

// A runtime that tells a Linux process's start since boot decides outright: the same boot, time namespace and ticks is
// that process whatever the clocks say, other ticks or another boot another process; only a lookup in another time
// namespace, which reads the ticks shifted by its boot-time offset, or a machine that told no boot, goes by the clocks.
func TestTheStartSinceBootDecides(t *testing.T) {
	const start = "Thu Nov 19 00:36:09 2026" // ps's, a second behind the runtime's on Linux
	const boot, ns = "6f1c0d2e-4b7a-4c1e-9a1b-2f3e4d5c6b7a", "time:[4026531834]"
	began := time.Date(2026, 11, 19, 0, 36, 10, 108_000_000, time.UTC)
	// the real clock read after it was read before, so no boot to place a creation by: the clocks cannot settle it
	stepped := "system" + script.US + "Linux" + script.US + "1795046400" + script.US + "2170.40" + script.US +
		strconv.FormatInt(began.UnixMilli()+50, 10) + script.US + strconv.FormatInt(began.UnixMilli(), 10) + script.US + "100" + script.RS + "\n"
	since := func(bootID, timeNS string) string {
		return "since" + script.US + bootID + script.US + timeNS + script.RS + "\n"
	}
	record := func(ticks string) string {
		return "7" + script.US + "claude" + script.US + start + script.US + "/a" + script.US +
			`{"pid":7,"sessionId":"id-1","cwd":"/a","procStart":"` + start + `","startedAt":` + strconv.FormatInt(began.Add(time.Second).UnixMilli(), 10) + `}` +
			script.US + start + script.US + ticks + script.RS + "\n"
	}
	told := SinceBoot{BootID: boot, TimeNS: ns, Ticks: 216940}
	if _, known := (SinceBoot{BootID: boot, Ticks: 216940}).same(boot, "", "216940"); known {
		t.Fatal("two namespaces neither side could name taken for one")
	}
	for _, c := range []struct {
		name   string
		out    string
		want   string
		unsure bool
	}{
		{"the very process, the clocks no help", since(boot, ns) + record("216940"), "id-1", false},
		{"another born in the same second", since(boot, ns) + record("216980"), "", false},
		{"a process of another boot", since("0e2d8c1f-0000-4000-8000-000000000000", ns) + record("216940"), "", false},
		{"a lookup in another time namespace: the clocks decide, and cannot", since(boot, "time:[4026532001]") + record("216940"), "", true},
		{"a lookup that could not name its time namespace: the clocks decide", since(boot, "") + record("216940"), "", true},
		{"a machine that told no boot: the clocks decide, and cannot", record("216940"), "", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			procs := []Running{{PID: 7, Agent: "claude", CWD: "/a", Seen: began, Started: began.UnixMilli(), SinceBoot: told}}
			never := machineClock{asked: began.Add(time.Hour), latest: began.Add(-time.Hour)}
			got, _, unsure := parseSessionsByPID(stepped+c.out, procs, rpath.Style{}, never)
			if got[7] != c.want || unsure[7] != c.unsure {
				t.Fatalf("got %v, unsure %v, want %q", got, unsure, c.want)
			}
		})
	}
}

// End to end on a real process, as remuda-core tells its creation: the process the runtime names is found, and one a
// tick of its boot or a second of its creation away is another process's.
func TestARealProcessIsKnownByTheCreationTheRuntimeTold(t *testing.T) {
	home := realDir(t)
	t.Setenv("HOME", home)
	pid := holder(t, home, nil, nil, false, "")
	start := procStart(t, pid)
	body := fmt.Sprintf(`{"pid":%d,"sessionId":"4c9d6f0e-1111-4222-8333-944455556666","cwd":%q,"startedAt":%d,"procStart":%q}`, pid, home, time.Now().UnixMilli(), start)
	if err := os.MkdirAll(filepath.Join(home, ".claude", "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "sessions", strconv.Itoa(pid)+".json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	began, err := time.Parse(time.ANSIC, start)
	if err != nil {
		t.Fatal(err)
	}
	known := Running{PID: pid, Agent: "claude", CWD: home, Seen: time.Now().Add(-time.Hour), Started: creationOf(t, pid, began)}
	if runtime.GOOS == "linux" {
		known.SinceBoot = sinceBootOf(t, pid) // a core that tells it binds the process without reading a clock
	}
	host := remoteHere(home)
	reader := NewReader(host)
	got, err := reader.SessionsOf([]Running{known})
	if err != nil || got[pid] != "4c9d6f0e-1111-4222-8333-944455556666" {
		t.Fatalf("the creation told: got %v, %v", got, err)
	}
	for _, script := range host.Scripts() {
		if script == "date +%s" {
			t.Fatal("the machine's clock was read though the runtime told the creation")
		}
	}

	other := known
	if runtime.GOOS == "linux" {
		other.SinceBoot.Ticks++
	} else {
		other.Started -= 1000
	}
	got, err = reader.SessionsOf([]Running{other})
	if err != nil || len(named(got)) != 0 {
		t.Fatalf("another process than the runtime named: got %v, %v", got, err)
	}

	if runtime.GOOS != "linux" {
		return
	}
	// On Linux the start since boot decides alone: a creation the clocks place a second off is no matter beside it.
	clocksOff := known
	clocksOff.Started -= 1000
	got, err = reader.SessionsOf([]Running{clocksOff})
	if err != nil || got[pid] != "4c9d6f0e-1111-4222-8333-944455556666" {
		t.Fatalf("the start since boot told: got %v, %v", got, err)
	}
}

// sinceBootOf is a Linux process's start since boot as remuda-core tells it: this boot, this process's time namespace
// (the lookup's, its child) and the process's starttime ticks.
func sinceBootOf(t *testing.T, pid int) SinceBoot {
	t.Helper()
	boot, errBoot := os.ReadFile("/proc/sys/kernel/random/boot_id")
	ns, _ := os.Readlink("/proc/self/ns/time")
	stat, errStat := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if errBoot != nil || errStat != nil {
		t.Fatalf("this process's start since boot: %v, %v", errBoot, errStat)
	}
	after := string(stat[strings.LastIndex(string(stat), ")")+2:])
	ticks, err := strconv.ParseUint(strings.Fields(after)[19], 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return SinceBoot{BootID: strings.TrimSpace(string(boot)), TimeNS: ns, Ticks: ticks}
}

func TestClaudeSessionLeftByAnEndedProcessIsNotTaken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := realDir(t)
	pid := holder(t, work, nil, nil, false, "")
	writeSessionFile(t, filepath.Join(home, ".claude", "sessions"), pid, "4c9d6f0e-1111-4222-8333-944455556666", work, "Mon Jan  1 00:00:00 2024")

	reader := NewReader(remoteHere(home))
	got, err := reader.SessionsOf([]Running{{PID: pid, Agent: "claude", CWD: work}})
	if err != nil {
		t.Fatal(err)
	}
	if id := got[pid]; id != "" {
		t.Fatalf("a file whose start is not the process's gave %q", id)
	}
}

// writeRollout writes a rollout of thread id whose session_meta says whose thread it is.
func writeRollout(t *testing.T, home, id string, subagent bool) string {
	return writeRolloutNamed(t, home, "rollout-2026-11-20T02-28-48-"+id+".jsonl", id, subagent)
}

// sessionMeta is a rollout's first line as Codex writes it, the person's thread's or a subagent's.
func sessionMeta(id string, subagent bool) string {
	if subagent {
		return `{"timestamp":"2026-11-20T02:28:48.000Z","type":"session_meta","payload":{"session_id":"` + id + `","id":"` + id + `","source":{"subagent":{"thread_spawn":{}}},"thread_source":"subagent"}}`
	}
	return `{"timestamp":"2026-11-20T02:28:48.000Z","type":"session_meta","payload":{"session_id":"` + id + `","id":"` + id + `","source":"cli","thread_source":"user"}}`
}

// writeRolloutNamed is writeRollout under a name of its own, such as a revert's replacement file.
func writeRolloutNamed(t *testing.T, home, name, id string, subagent bool) string {
	t.Helper()
	dir := filepath.Join(home, ".codex", "sessions", "2026", "11", "20")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(sessionMeta(id, subagent)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestClaudeSessionUnderARelativeConfigDirIsReadFromTheAgentsFolder(t *testing.T) {
	home := realDir(t)
	t.Setenv("HOME", home)
	work := filepath.Join(home, "work", "app")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	pid := holder(t, work, nil, []string{"CLAUDE_CONFIG_DIR=./.claude-profile"}, false, "")
	writeSessionFile(t, filepath.Join(work, ".claude-profile", "sessions"), pid, "4c9d6f0e-1111-4222-8333-944455556666", work, procStart(t, pid))

	got, err := NewReader(remoteHere(home)).SessionsOf([]Running{{PID: pid, Agent: "claude", CWD: work}})
	if err != nil {
		t.Fatal(err)
	}
	if got[pid] != "4c9d6f0e-1111-4222-8333-944455556666" {
		t.Fatalf("session = %q, want the one under the agent's own folder", got[pid])
	}
}

func TestAClaudeThatEndedGivesNoSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cmd := exec.Command("sh", "-c", "exit 0")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	gone := cmd.Process.Pid
	writeSessionFile(t, filepath.Join(home, ".claude", "sessions"), gone, "4c9d6f0e-1111-4222-8333-944455556666", "/work/api", "")
	// a Linux under WSL reaches Windows' powershell.exe, which knows other processes by the same numbers
	stand := t.TempDir()
	if err := os.WriteFile(filepath.Join(stand, "powershell.exe"), []byte("#!/bin/sh\necho 1000\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stand+string(os.PathListSeparator)+os.Getenv("PATH"))

	got, err := NewReader(remoteHere(home)).SessionsOf([]Running{{PID: gone, Agent: "claude", CWD: "/work/api"}})
	if err != nil {
		t.Fatal(err)
	}
	if id := got[gone]; id != "" {
		t.Fatalf("an ended process gave %q", id)
	}
}

func TestCodexSessionIsThePersonsRolloutItsProcessHoldsOpen(t *testing.T) {
	const other = "01a1f692-0000-70f0-b846-42b3f2f3c1db"
	cases := []struct {
		name  string
		holds func(home string) (holds []string, underShell bool, shellHolds string)
		want  string
	}{
		{"its own", func(home string) ([]string, bool, string) {
			return []string{writeRollout(t, home, rolloutID, false)}, false, ""
		}, rolloutID},
		{"a child's, under a wrapper", func(home string) ([]string, bool, string) {
			return []string{writeRollout(t, home, rolloutID, false)}, true, ""
		}, rolloutID},
		{"beside a subagent's", func(home string) ([]string, bool, string) {
			return []string{writeRollout(t, home, other, true), writeRollout(t, home, rolloutID, false)}, false, ""
		}, rolloutID},
		{"its own over a child's", func(home string) ([]string, bool, string) {
			return []string{writeRollout(t, home, other, false)}, true, writeRollout(t, home, rolloutID, false)
		}, rolloutID},
		{"beside a rollout still empty", func(home string) ([]string, bool, string) {
			empty := filepath.Join(filepath.Dir(writeRollout(t, home, rolloutID, false)), "rollout-2026-11-20T02-29-00-01a1f692-0000-70f0-b846-42b3f2f3c1db.jsonl")
			if err := os.WriteFile(empty, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			return []string{filepath.Join(filepath.Dir(empty), "rollout-2026-11-20T02-28-48-"+rolloutID+".jsonl"), empty}, false, ""
		}, rolloutID},
		{"a revert's replacement file", func(home string) ([]string, bool, string) {
			return []string{writeRolloutNamed(t, home, "rollout-2026-11-20T03-10-00-"+rolloutID+"_01a1f6a0-1111-7222-8333-944455556666.jsonl", rolloutID, false)}, false, ""
		}, rolloutID},
		{"two of the person's", func(home string) ([]string, bool, string) {
			return []string{writeRollout(t, home, rolloutID, false), writeRollout(t, home, other, false)}, false, ""
		}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := realDir(t)
			t.Setenv("HOME", home)
			holds, underShell, shellHolds := c.holds(home)
			pid := holder(t, home, holds, nil, underShell, shellHolds)
			reader := NewReader(remoteHere(home))
			got, err := reader.SessionsOf([]Running{{PID: pid, Agent: "codex", CWD: home}})
			if err != nil {
				t.Fatal(err)
			}
			if got[pid] != c.want {
				t.Fatalf("session = %q, want %q", got[pid], c.want)
			}
			if c.name == "a revert's replacement file" {
				if found, err := reader.ResolveSession("codex", home, "id", rolloutID); err != nil || found != holds[0] {
					t.Fatalf("the live replacement rollout = %q, %v; want %s", found, err, holds[0])
				}
			}
		})
	}
}

func TestAPipeNamedLikeARolloutHoldsNothingUp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no FIFOs")
	}
	home := realDir(t)
	t.Setenv("HOME", home)
	rollout := writeRollout(t, home, rolloutID, false)
	pipe := filepath.Join(filepath.Dir(rollout), "rollout-2026-11-20T02-28-49-01a1f692-0000-70f0-b846-42b3f2f3c1db.jsonl")
	if err := exec.Command("mkfifo", pipe).Run(); err != nil {
		t.Fatal(err)
	}
	codex := holder(t, home, []string{pipe, rollout}, nil, false, "")
	claude := holder(t, home, nil, nil, false, "")
	writeSessionFile(t, filepath.Join(home, ".claude", "sessions"), claude, "4c9d6f0e-1111-4222-8333-944455556666", "/work/api", procStart(t, claude))

	began := time.Now()
	got, err := NewReader(remoteHere(home)).SessionsOf([]Running{{PID: codex, Agent: "codex", CWD: home}, {PID: claude, Agent: "claude", CWD: home}})
	if err != nil {
		t.Fatalf("after %s: %v", time.Since(began), err)
	}
	if got[codex] != rolloutID || got[claude] != "4c9d6f0e-1111-4222-8333-944455556666" {
		t.Fatalf("got %v", got)
	}
}

// The machine is asked its time in a call of its own, only while an agent's creation is not told by the runtime, then
// once for every agent.
func TestSessionsOfAsksTheMachineOnceForEveryAgent(t *testing.T) {
	answer := "41\037claude\037Thu Nov 19 00:36:10 2026\037/w\037" +
		`{"pid":41,"sessionId":"aaaa-1","cwd":"/w","procStart":"Thu Nov 19 00:36:10 2026","startedAt":1795048571200}` + "\037Thu Nov 19 00:36:10 2026\036\n" +
		"52\037codex\037Thu Nov 19 00:40:00 2026\037\03752\035/home/me/.codex/sessions/2026/11/19/rollout-2026-11-19T00-40-01-" + rolloutID + ".jsonl\035" + sessionMeta(rolloutID, false) + "\034\037Thu Nov 19 00:40:00 2026\036\n"
	host := &shell.Fake{Name: "gog", Remote: true, Dir: "/home/me", Answer: func(script string) (string, error) {
		if script == "date +%s" {
			return "1795048800\n", nil
		}
		return answer, nil
	}}
	codexBegan := time.Date(2026, 11, 19, 0, 40, 0, 0, time.UTC).UnixMilli() + 250
	procs := []Running{{PID: 41, Agent: "claude", CWD: "/w", Seen: time.Now()}, {PID: 52, Agent: "codex", CWD: "/w", Started: codexBegan}, {PID: 60, Agent: "gemini"}}
	got, err := NewReader(host).SessionsOf(procs)
	if err != nil {
		t.Fatal(err)
	}
	if got[41] != "aaaa-1" || got[52] != rolloutID || len(named(got)) != 2 {
		t.Fatalf("got %v", got)
	}
	scripts := host.Scripts()
	if len(scripts) != 2 || scripts[0] != "date +%s" || !strings.Contains(scripts[1], "\ns 41 claude '/w'\n") || !strings.Contains(scripts[1], "\ns 52 codex '/w'\n") || strings.Contains(scripts[1], "gemini") {
		t.Fatalf("scripts = %q", scripts)
	}

	told := &shell.Fake{Name: "gog", Remote: true, Dir: "/home/me", Answer: func(string) (string, error) { return answer, nil }}
	procs[0].Started = time.Date(2026, 11, 19, 0, 36, 10, 900_000_000, time.UTC).UnixMilli()
	if got, err := NewReader(told).SessionsOf(procs); err != nil || len(named(got)) != 2 || len(told.Scripts()) != 1 {
		t.Fatalf("every creation told by the runtime: got %v, %v, scripts %d, want no clock asked", got, err, len(told.Scripts()))
	}

	host = &shell.Fake{Name: "gog", Remote: true}
	if got, err := NewReader(host).SessionsOf([]Running{{PID: 60, Agent: "gemini"}, {PID: 1, Agent: "claude"}}); err != nil || len(named(got)) != 0 || len(host.Scripts()) != 0 {
		t.Fatalf("nothing to look up: got %v, %v, scripts %q", got, err, host.Scripts())
	}
}

// The process holding a pid when it is read must have begun by the time the agent was seen running: a later one took
// the pid after it. The sighting goes onto the machine's clock by the time passed since it, so a machine whose clock
// runs elsewhere is no matter; a machine that takes too long to tell its time is no reading at all.
func TestAProcessBegunAfterTheAgentWasSeenHasNoSession(t *testing.T) {
	const start = "Thu Nov 19 00:36:10 2026"
	began := time.Date(2026, 11, 19, 0, 36, 10, 0, time.UTC)
	record := "7" + script.US + "claude" + script.US + start + script.US + "/a" + script.US +
		`{"pid":7,"sessionId":"id-1","cwd":"/a","procStart":"` + start + `","startedAt":1795048571000}` + script.US + start + script.RS + "\n"
	for _, c := range []struct {
		name  string
		ahead time.Duration // the machine's clock against this one's
		after time.Duration // the sighting, on the machine's clock, after the process began
		want  string
	}{
		{"seen running after it began", 0, 10 * time.Minute, "id-1"},
		{"seen running the moment it began", 0, 0, "id-1"},
		{"begun three seconds after the agent was seen", 0, -3 * time.Second, ""},
		{"begun an hour after", 0, -time.Hour, ""},
		{"a machine an hour ahead, seen after it began", time.Hour, 10 * time.Minute, "id-1"},
		{"a machine an hour ahead, begun after the agent was seen", time.Hour, -3 * time.Second, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			seen := began.Add(c.after - c.ahead)
			asked := seen.Add(20 * time.Minute)
			machineNow := asked.Add(c.ahead + 300*time.Millisecond)
			clock := machineClock{asked: asked, latest: time.Unix(machineNow.Unix()+1, 0)}
			got, _, _ := parseSessionsByPID(record, []Running{{PID: 7, Agent: "claude", CWD: "/a", Seen: seen}}, rpath.Style{}, clock)
			if got[7] != c.want {
				t.Fatalf("got %v, want %q", got, c.want)
			}
		})
	}
	if got, _, _ := parseSessionsByPID(record, []Running{{PID: 7, Agent: "claude", CWD: "/a"}}, rpath.Style{}, machineClock{asked: began, latest: began}); got[7] != "id-1" {
		t.Fatalf("no sighting known: got %v", got)
	}

	maxClockSpan = 50 * time.Millisecond
	t.Cleanup(func() { maxClockSpan = 2 * time.Second })
	// two agents on a machine slow to tell its time: the one the runtime told of is still found, the other has none
	other := "8" + script.US + "codex" + script.US + start + script.US + "/b" + script.US + "8" + script.GS +
		"/x/rollout-2026-11-20T02-28-48-" + rolloutID + ".jsonl" + script.GS + sessionMeta(rolloutID, false) + script.FS +
		script.US + start + script.RS + "\n"
	slow := &shell.Fake{Name: "gog", Remote: true, Answer: func(script string) (string, error) {
		if script == "date +%s" {
			time.Sleep(100 * time.Millisecond)
			return "1795048800\n", nil
		}
		return record + other, nil
	}}
	procs := []Running{
		{PID: 7, Agent: "claude", CWD: "/a", Seen: time.Now(), Started: began.UnixMilli() + 400},
		{PID: 8, Agent: "codex", CWD: "/b", Seen: time.Now()},
		{PID: 9, Agent: "gemini", CWD: "/c", Seen: time.Now()},
	}
	got, err := NewReader(slow).SessionsOf(procs)
	_, unverified := got[8]
	if none, told := got[9]; err == nil || got[7] != "id-1" || unverified || !told || none != "" {
		t.Fatalf("a machine slow to tell its time: got %v, %v", got, err)
	}
	gemini := &shell.Fake{Name: "gog", Remote: true, Answer: func(string) (string, error) { return record, nil }}
	if got, err := NewReader(gemini).SessionsOf(procs[:1:1]); err != nil || got[7] != "id-1" || len(gemini.Scripts()) != 1 {
		t.Fatalf("no agent the clock is read for: got %v, %v after %q", got, err, gemini.Scripts())
	}
	procs = append(procs[:1:1], procs[2])
	if _, err := NewReader(gemini).SessionsOf(procs); err != nil || len(gemini.Scripts()) != 2 {
		t.Fatalf("a Gemini CLI, whose session is not looked up, had the clock read: %q", gemini.Scripts())
	}
}

// A runtime that tells the process's creation tells it outright: the process holding the pid when it is read must be
// that very one, to ps's second or a FILETIME's millisecond, and no clock is read for it, so a machine whose clock
// stepped back passes neither a later process nor loses the one seen.
func TestTheCreationTheRuntimeToldDecides(t *testing.T) {
	const start = "Thu Nov 19 00:36:10 2026"
	began := time.Date(2026, 11, 19, 0, 36, 10, 0, time.UTC)
	claudeBegan := began.Add(time.Second).UnixMilli() // Claude Code's own startedAt, after its process was created
	recordAt := func(start string, startedAt int64) string {
		return "7" + script.US + "claude" + script.US + start + script.US + "/a" + script.US +
			`{"pid":7,"sessionId":"id-1","cwd":"/a","procStart":"` + start + `","startedAt":` + strconv.FormatInt(startedAt, 10) + `}` + script.US + start + script.RS + "\n"
	}
	record := func(start string) string { return recordAt(start, claudeBegan) }
	native := func(ft int64) string {
		return "7" + script.US + "claude" + script.US + "ft:" + strconv.FormatInt(ft, 10) + script.US + script.US +
			`{"pid":7,"sessionId":"id-1","cwd":"C:\\a","startedAt":` + strconv.FormatInt(claudeBegan, 10) + `,"procStartFt":"` + strconv.FormatInt(ft, 10) + `"}` + script.US + script.RS + "\n"
	}
	ft := func(at time.Time) int64 { return at.UnixMilli()*10_000 + filetimeEpoch }
	// a clock reading that would refuse anything, and one that would take anything: neither counts once the runtime told
	never := machineClock{asked: began.Add(time.Hour), latest: began.Add(-time.Hour)}
	always := machineClock{asked: began, latest: began.Add(time.Hour)}
	for _, c := range []struct {
		name    string
		out     string
		started time.Time
		clock   machineClock
		style   rpath.Style
		cwd     string
		want    string
	}{
		{"the very process, its clock stepped back since", record(start), began.Add(420 * time.Millisecond), never, rpath.Style{}, "/a", "id-1"},
		{"another process under the pid, begun a second later", record("Thu Nov 19 00:36:11 2026"), began.Add(420 * time.Millisecond), always, rpath.Style{}, "/a", ""},
		{"another process that reads as begun earlier: the clock stepped back", record("Thu Nov 19 00:30:00 2026"), began.Add(420 * time.Millisecond), always, rpath.Style{}, "/a", ""},
		{"a Linux that told no boot: its second settles nothing", "system" + script.US + "Linux" + script.RS + "\n" + record("Thu Nov 19 00:36:09 2026"), began.Add(420 * time.Millisecond), always, rpath.Style{}, "/a", ""},
		{"a Mac's ps a second behind: another process", "system" + script.US + "Darwin" + script.RS + "\n" + record("Thu Nov 19 00:36:09 2026"), began.Add(420 * time.Millisecond), always, rpath.Style{}, "/a", ""},
		{"ps two seconds behind", record("Thu Nov 19 00:36:08 2026"), began.Add(420 * time.Millisecond), always, rpath.Style{}, "/a", ""},
		{"a stale file of a process before it, the same second", recordAt(start, began.Add(300*time.Millisecond).UnixMilli()), began.Add(420 * time.Millisecond), always, rpath.Style{}, "/a", ""},
		{"a file saying nothing of when its Claude Code began", recordAt(start, 0), began.Add(420 * time.Millisecond), always, rpath.Style{}, "/a", ""},
		{"a native process, the very one", native(ft(began.Add(420 * time.Millisecond))), began.Add(420 * time.Millisecond), never, rpath.For(`C:\Users\me`), `C:\a`, "id-1"},
		{"a native process, another a millisecond apart", native(ft(began.Add(421 * time.Millisecond))), began.Add(420 * time.Millisecond), always, rpath.For(`C:\Users\me`), `C:\a`, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			procs := []Running{{PID: 7, Agent: "claude", CWD: c.cwd, Seen: began.Add(time.Minute), Started: c.started.UnixMilli()}}
			got, _, unsure := parseSessionsByPID(c.out, procs, c.style, c.clock)
			if got[7] != c.want || unsure[7] != strings.HasPrefix(c.name, "a Linux that told no boot") {
				t.Fatalf("got %v, unsure %v, want %q", got, unsure, c.want)
			}
		})
	}
}

// End to end on a real process: seen running now, it is found; seen an hour before it began, the process holding the
// pid is a later one.
func TestTheProcessHoldingAPidIsTheOneSeen(t *testing.T) {
	home := realDir(t)
	t.Setenv("HOME", home)
	pid := holder(t, home, nil, nil, false, "")
	writeSessionFile(t, filepath.Join(home, ".claude", "sessions"), pid, "4c9d6f0e-1111-4222-8333-944455556666", home, procStart(t, pid))
	reader := NewReader(remoteHere(home))
	got, err := reader.SessionsOf([]Running{{PID: pid, Agent: "claude", CWD: home, Seen: time.Now()}})
	if err != nil || got[pid] != "4c9d6f0e-1111-4222-8333-944455556666" {
		t.Fatalf("seen running now: got %v, %v", got, err)
	}
	got, err = reader.SessionsOf([]Running{{PID: pid, Agent: "claude", CWD: home, Seen: time.Now().Add(-time.Hour)}})
	if err != nil || len(named(got)) != 0 {
		t.Fatalf("seen an hour before it began: got %v, %v", got, err)
	}
}

func TestParseSessionsByPID(t *testing.T) {
	read := func(pid int, agent, start, folder, payload, after string) string {
		return strconv.Itoa(pid) + script.US + agent + script.US + start + script.US + folder + script.US + payload + script.US + after + script.RS + "\n"
	}
	standing := func(pid int, agent, start, folder, payload string) string {
		after := start
		if strings.HasPrefix(start, "ft:") {
			after = "" // ps sees no native Windows process
		}
		return read(pid, agent, start, folder, payload, after)
	}
	record := func(pid int, agent, start, payload string) string { return standing(pid, agent, start, "", payload) }
	claude := func(pid int, id, cwd, start string) string {
		return fmt.Sprintf(`{"pid":%d,"sessionId":%q,"cwd":%q,"procStart":%q}`, pid, id, cwd, start)
	}
	windows := rpath.For(`C:\Users\me`)
	cases := []struct {
		name  string
		out   string
		procs []Running
		style rpath.Style
		want  string
	}{
		{"start matches", record(7, "claude", "S", claude(7, "id-1", "/a", "S")), []Running{{PID: 7, Agent: "claude", CWD: "/a"}}, rpath.Style{}, "id-1"},
		{"start matches, the file of another folder", record(7, "claude", "S", claude(7, "id-1", "/a", "S")), []Running{{PID: 7, Agent: "claude", CWD: "/b"}}, rpath.Style{}, "id-1"},
		{"the process stands in the pane's folder", standing(7, "claude", "S", "/b", claude(7, "id-1", "/a", "S")), []Running{{PID: 7, Agent: "claude", CWD: "/b"}}, rpath.Style{}, "id-1"},
		{"the process stands elsewhere: the pid went to another", standing(7, "claude", "S", "/c", claude(7, "id-1", "/c", "S")), []Running{{PID: 7, Agent: "claude", CWD: "/b"}}, rpath.Style{}, ""},
		{"a Windows process in its folder, spelt as Git Bash does", standing(7, "claude", "S", "/c/Work/Api", claude(7, "id-1", `C:\Work\Api`, "S")), []Running{{PID: 7, Agent: "claude", CWD: `C:\Work\Api`}}, windows, "id-1"},
		{"start differs", record(7, "claude", "S", claude(7, "id-1", "/a", "T")), []Running{{PID: 7, Agent: "claude", CWD: "/a"}}, rpath.Style{}, ""},
		{"another pid's file", record(7, "claude", "S", claude(8, "id-1", "/a", "S")), []Running{{PID: 7, Agent: "claude", CWD: "/a"}}, rpath.Style{}, ""},
		{"no start here: nothing tells it from another process", record(7, "claude", "", claude(7, "id-1", `C:\Work\Api`, "S")), []Running{{PID: 7, Agent: "claude", CWD: `c:/work/api`}}, windows, ""},
		{"the first start unknown, the second told", read(7, "claude", "-", "/a", claude(7, "id-1", "/a", "S"), "S"), []Running{{PID: 7, Agent: "claude", CWD: "/a"}}, rpath.Style{}, ""},
		{"no start here, another folder", record(7, "claude", "", claude(7, "id-1", `C:\Work\Api`, "S")), []Running{{PID: 7, Agent: "claude", CWD: `C:\Work\Web`}}, windows, ""},
		{"no session file", record(7, "claude", "S", ""), []Running{{PID: 7, Agent: "claude", CWD: "/a"}}, rpath.Style{}, ""},
		{"a native process, started since", record(7, "claude", "ft:116444736050000000", `{"pid":7,"sessionId":"id-1","cwd":"C:\\a","startedAt":5200}`), []Running{{PID: 7, Agent: "claude", CWD: `C:\a`}}, windows, "id-1"},
		{"a native process, a file from before it", record(7, "claude", "ft:116444736050000000", `{"pid":7,"sessionId":"id-1","cwd":"C:\\a","startedAt":1000}`), []Running{{PID: 7, Agent: "claude", CWD: `C:\a`}}, windows, ""},
		{"a native process, a file half a second before it", record(7, "claude", "ft:116444736100000000", `{"pid":7,"sessionId":"id-1","cwd":"C:\\a","startedAt":9500}`), []Running{{PID: 7, Agent: "claude", CWD: `C:\a`}}, windows, ""},
		{"a native process, its creation recorded", record(7, "claude", "ft:116444736050000000", `{"pid":7,"sessionId":"id-1","cwd":"C:\\a","startedAt":1000,"procStartFt":"116444736050000016"}`), []Running{{PID: 7, Agent: "claude", CWD: `C:\a`}}, windows, "id-1"},
		{"a native process, its creation recorded by an agent of another folder", record(7, "claude", "ft:116444736050000000", `{"pid":7,"sessionId":"id-1","cwd":"C:\\b","startedAt":1000,"procStartFt":"116444736050000016"}`), []Running{{PID: 7, Agent: "claude", CWD: `C:\a`}}, windows, ""},
		{"a native process, another's creation though begun since", record(7, "claude", "ft:116444736050000000", `{"pid":7,"sessionId":"id-1","cwd":"C:\\a","startedAt":9000,"procStartFt":"116444736040000000"}`), []Running{{PID: 7, Agent: "claude", CWD: `C:\a`}}, windows, ""},
		{"a native process, a creation in another unit: when it began decides", record(7, "claude", "ft:116444736050000000", `{"pid":7,"sessionId":"id-1","cwd":"C:\\a","startedAt":5200,"procStartFt":"1795048570000000000"}`), []Running{{PID: 7, Agent: "claude", CWD: `C:\a`}}, windows, "id-1"},
		{"no procStart, begun before the process", record(7, "claude", "Thu Nov 19 00:36:10 2026", `{"pid":7,"sessionId":"id-1","cwd":"/a","startedAt":1795048569000}`), []Running{{PID: 7, Agent: "claude", CWD: "/a"}}, rpath.Style{}, ""},
		{"no procStart, begun since", record(7, "claude", "Thu Nov 19 00:36:10 2026", `{"pid":7,"sessionId":"id-1","cwd":"/a","startedAt":1795048570500}`), []Running{{PID: 7, Agent: "claude", CWD: "/a"}}, rpath.Style{}, "id-1"},
		{"a process gone", record(7, "claude", "-", claude(7, "id-1", "/a", "")), []Running{{PID: 7, Agent: "claude", CWD: "/a"}}, rpath.Style{}, ""},
		{"a verified file over a folder-only one before it", record(7, "claude", "S", claude(7, "stale", "/a", "")+script.GS+claude(7, "live", "/a", "S")+script.GS), []Running{{PID: 7, Agent: "claude", CWD: "/a"}}, rpath.Style{}, "live"},
		{"the session_meta's id over the file name", record(7, "codex", "S", "7"+script.GS+"/x/rollout-2026-11-20T03-10-00-"+rolloutID+"_01a1f6a0-1111-7222-8333-944455556666.jsonl"+script.GS+sessionMeta(rolloutID, false)+script.FS), []Running{{PID: 7, Agent: "codex", CWD: "/a"}}, rpath.Style{}, rolloutID},
		{"the file name when the session_meta names no id", record(7, "codex", "S", "7"+script.GS+"/x/rollout-2026-11-20T02-28-48-"+rolloutID+".jsonl"+script.GS+`{"type":"session_meta","payload":{"source":"cli"}}`+script.FS), []Running{{PID: 7, Agent: "codex", CWD: "/a"}}, rpath.Style{}, rolloutID},
		{"a thread resumed in another folder than it began in", standing(7, "codex", "S", "/a", "7"+script.GS+"/x/rollout-2026-11-20T02-28-48-"+rolloutID+".jsonl"+script.GS+`{"type":"session_meta","payload":{"id":"`+rolloutID+`","cwd":"/b","thread_source":"user"}}`+script.FS), []Running{{PID: 7, Agent: "codex", CWD: "/a"}}, rpath.Style{}, rolloutID},
		{"a thread held by a process standing elsewhere", standing(7, "codex", "S", "/b", "7"+script.GS+"/x/rollout-2026-11-20T02-28-48-"+rolloutID+".jsonl"+script.GS+sessionMeta(rolloutID, false)+script.FS), []Running{{PID: 7, Agent: "codex", CWD: "/a"}}, rpath.Style{}, ""},
		{"a subagent's thread alone", record(7, "codex", "S", "7"+script.GS+"/x/rollout-2026-11-20T02-28-48-"+rolloutID+".jsonl"+script.GS+sessionMeta(rolloutID, true)+script.FS), []Running{{PID: 7, Agent: "codex", CWD: "/a"}}, rpath.Style{}, ""},
		{"a rollout whose first line is no session_meta", record(7, "codex", "S", "7"+script.GS+"/x/rollout-2026-11-20T02-28-48-"+rolloutID+".jsonl"+script.GS+`{"type":"response_item","payload":{}}`+script.FS), []Running{{PID: 7, Agent: "codex", CWD: "/a"}}, rpath.Style{}, ""},
		{"a rollout still empty", record(7, "codex", "S", "7"+script.GS+"/x/rollout-2026-11-20T02-28-48-"+rolloutID+".jsonl"+script.GS+script.FS), []Running{{PID: 7, Agent: "codex", CWD: "/a"}}, rpath.Style{}, ""},
		{"another process took the pid while it was read", read(7, "codex", "S", "/a", "7"+script.GS+"/x/rollout-2026-11-20T02-28-48-"+rolloutID+".jsonl"+script.GS+sessionMeta(rolloutID, false)+script.FS, "T"), []Running{{PID: 7, Agent: "codex", CWD: "/a"}}, rpath.Style{}, ""},
		{"the process ended while it was read", read(7, "claude", "S", "/a", claude(7, "id-1", "/a", "S"), ""), []Running{{PID: 7, Agent: "claude", CWD: "/a"}}, rpath.Style{}, ""},
		{"a rollout under a folder with a line break", standing(7, "codex", "S", "/a", "7"+script.GS+"/x\ny/rollout-2026-11-20T02-28-48-"+rolloutID+".jsonl"+script.GS+`{"type":"session_meta","payload":{"thread_source":"user"}}`+script.FS), []Running{{PID: 7, Agent: "codex", CWD: "/a"}}, rpath.Style{}, rolloutID},
		{"an id that is not one", record(7, "claude", "S", claude(7, "--resume x", "/a", "S")), []Running{{PID: 7, Agent: "claude", CWD: "/a"}}, rpath.Style{}, ""},
		{"no rollout open", record(7, "codex", "S", ""), []Running{{PID: 7, Agent: "codex", CWD: "/a"}}, rpath.Style{}, ""},
		{"a pid not asked about", record(9, "codex", "S", "/x/rollout-2026-11-20T02-28-48-"+rolloutID+".jsonl"), []Running{{PID: 7, Agent: "codex", CWD: "/a"}}, rpath.Style{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _, _ := parseSessionsByPID(c.out, c.procs, c.style, machineClock{})
			if got[c.procs[0].PID] != c.want {
				t.Fatalf("got %v, want %q", got, c.want)
			}
		})
	}
}

// TestASessionUnderItsOwnCODEXHOMEIsOpenedToo: Codex with CODEX_HOME of its own writes its rollout there; the session
// the process holds is found by id, and the file behind it is found under that folder, not only under ~/.codex.
func TestASessionUnderItsOwnCODEXHOMEIsOpenedToo(t *testing.T) {
	home := realDir(t)
	t.Setenv("HOME", home)
	own := filepath.Join(home, "codex-profile")
	dir := filepath.Join(own, "sessions", "2026", "11", "20")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rollout-2026-11-20T02-28-48-"+rolloutID+".jsonl")
	if err := os.WriteFile(path, []byte(sessionMeta(rolloutID, false)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	work := realDir(t)
	pid := holder(t, work, []string{path}, []string{"CODEX_HOME=" + own}, false, "")
	reader := NewReader(remoteHere(home))
	got, err := reader.SessionsOf([]Running{{PID: pid, Agent: "codex", CWD: work}})
	if err != nil {
		t.Fatal(err)
	}
	if got[pid] != rolloutID {
		t.Fatalf("session = %q, want %q", got[pid], rolloutID)
	}
	if found, err := reader.ResolveSession("codex", work, "id", rolloutID); err != nil || found != path {
		t.Fatalf("rollout = %q, %v; want %s", found, err, path)
	}
	sessions, err := reader.Sessions(Codex, "")
	if err != nil || len(sessions) != 1 || sessions[0].Path != path {
		t.Fatalf("the folder's sessions = %+v, %v; want %s", sessions, err, path)
	}
}
