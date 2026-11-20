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
	// the pane was seen in another folder: the pid went to this process since
	if got, err := reader.SessionsOf([]Running{{PID: pid, Agent: "claude", CWD: filepath.Join(work, "elsewhere")}}); err != nil || len(got) != 0 {
		t.Fatalf("a process standing in another folder than the pane's: got %v, %v", got, err)
	}
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
	if id, ok := got[pid]; ok {
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
	if id, ok := got[gone]; ok {
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
			got, err := NewReader(remoteHere(home)).SessionsOf([]Running{{PID: pid, Agent: "codex", CWD: home}})
			if err != nil {
				t.Fatal(err)
			}
			if got[pid] != c.want {
				t.Fatalf("session = %q, want %q", got[pid], c.want)
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

func TestSessionsOfAsksTheMachineOnceForEveryAgent(t *testing.T) {
	answer := "41\037claude\037Thu Nov 19 00:36:10 2026\037/w\037" +
		`{"pid":41,"sessionId":"aaaa-1","cwd":"/w","procStart":"Thu Nov 19 00:36:10 2026"}` + "\036\n" +
		"52\037codex\037Thu Nov 19 00:40:00 2026\037\03752\035/home/me/.codex/sessions/2026/11/19/rollout-2026-11-19T00-40-01-" + rolloutID + ".jsonl\035" + sessionMeta(rolloutID, false) + "\n\036\n"
	host := &shell.Fake{Name: "gog", Remote: true, Dir: "/home/me", Answer: func(string) (string, error) { return answer, nil }}
	got, err := NewReader(host).SessionsOf([]Running{{PID: 41, Agent: "claude", CWD: "/w"}, {PID: 52, Agent: "codex", CWD: "/w"}, {PID: 60, Agent: "gemini"}})
	if err != nil {
		t.Fatal(err)
	}
	if got[41] != "aaaa-1" || got[52] != rolloutID || len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	scripts := host.Scripts()
	if len(scripts) != 1 || !strings.Contains(scripts[0], "\ns 41 claude '/w'\n") || !strings.Contains(scripts[0], "\ns 52 codex '/w'\n") || strings.Contains(scripts[0], "gemini") {
		t.Fatalf("scripts = %q", scripts)
	}

	host = &shell.Fake{Name: "gog", Remote: true}
	if got, err := NewReader(host).SessionsOf([]Running{{PID: 60, Agent: "gemini"}, {PID: 1, Agent: "claude"}}); err != nil || len(got) != 0 || len(host.Scripts()) != 0 {
		t.Fatalf("nothing to look up: got %v, %v, scripts %q", got, err, host.Scripts())
	}
}

func TestParseSessionsByPID(t *testing.T) {
	standing := func(pid int, agent, start, folder, payload string) string {
		return strconv.Itoa(pid) + script.US + agent + script.US + start + script.US + folder + script.US + payload + script.RS + "\n"
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
		{"start matches", record(7, "claude", "S", claude(7, "id-1", "/a", "S")), []Running{{7, "claude", "/a"}}, rpath.Style{}, "id-1"},
		{"start matches, the file of another folder", record(7, "claude", "S", claude(7, "id-1", "/a", "S")), []Running{{7, "claude", "/b"}}, rpath.Style{}, "id-1"},
		{"the process stands in the pane's folder", standing(7, "claude", "S", "/b", claude(7, "id-1", "/a", "S")), []Running{{7, "claude", "/b"}}, rpath.Style{}, "id-1"},
		{"the process stands elsewhere: the pid went to another", standing(7, "claude", "S", "/c", claude(7, "id-1", "/c", "S")), []Running{{7, "claude", "/b"}}, rpath.Style{}, ""},
		{"a Windows process in its folder, spelt as Git Bash does", standing(7, "claude", "", "/c/Work/Api", claude(7, "id-1", `C:\Work\Api`, "S")), []Running{{7, "claude", `C:\Work\Api`}}, windows, "id-1"},
		{"start differs", record(7, "claude", "S", claude(7, "id-1", "/a", "T")), []Running{{7, "claude", "/a"}}, rpath.Style{}, ""},
		{"another pid's file", record(7, "claude", "S", claude(8, "id-1", "/a", "S")), []Running{{7, "claude", "/a"}}, rpath.Style{}, ""},
		{"no start here, the folder decides", record(7, "claude", "", claude(7, "id-1", `C:\Work\Api`, "S")), []Running{{7, "claude", `c:/work/api`}}, windows, "id-1"},
		{"no start here, another folder", record(7, "claude", "", claude(7, "id-1", `C:\Work\Api`, "S")), []Running{{7, "claude", `C:\Work\Web`}}, windows, ""},
		{"no session file", record(7, "claude", "S", ""), []Running{{7, "claude", "/a"}}, rpath.Style{}, ""},
		{"a native process, started since", record(7, "claude", "ft:116444736050000000", `{"pid":7,"sessionId":"id-1","cwd":"C:\\a","startedAt":5200}`), []Running{{7, "claude", `C:\a`}}, windows, "id-1"},
		{"a native process, a file from before it", record(7, "claude", "ft:116444736050000000", `{"pid":7,"sessionId":"id-1","cwd":"C:\\a","startedAt":1000}`), []Running{{7, "claude", `C:\a`}}, windows, ""},
		{"a native process, a file half a second before it", record(7, "claude", "ft:116444736100000000", `{"pid":7,"sessionId":"id-1","cwd":"C:\\a","startedAt":9500}`), []Running{{7, "claude", `C:\a`}}, windows, ""},
		{"a native process, its creation recorded", record(7, "claude", "ft:116444736050000000", `{"pid":7,"sessionId":"id-1","cwd":"C:\\a","startedAt":1000,"procStartFt":"116444736050000016"}`), []Running{{7, "claude", `C:\a`}}, windows, "id-1"},
		{"a native process, another's creation though begun since", record(7, "claude", "ft:116444736050000000", `{"pid":7,"sessionId":"id-1","cwd":"C:\\a","startedAt":9000,"procStartFt":"116444736040000000"}`), []Running{{7, "claude", `C:\a`}}, windows, ""},
		{"a native process, a creation in another unit: when it began decides", record(7, "claude", "ft:116444736050000000", `{"pid":7,"sessionId":"id-1","cwd":"C:\\a","startedAt":5200,"procStartFt":"1795048570000000000"}`), []Running{{7, "claude", `C:\a`}}, windows, "id-1"},
		{"no procStart, begun before the process", record(7, "claude", "Thu Nov 19 00:36:10 2026", `{"pid":7,"sessionId":"id-1","cwd":"/a","startedAt":1795048569000}`), []Running{{7, "claude", "/a"}}, rpath.Style{}, ""},
		{"no procStart, begun since", record(7, "claude", "Thu Nov 19 00:36:10 2026", `{"pid":7,"sessionId":"id-1","cwd":"/a","startedAt":1795048570500}`), []Running{{7, "claude", "/a"}}, rpath.Style{}, "id-1"},
		{"a process gone", record(7, "claude", "-", claude(7, "id-1", "/a", "")), []Running{{7, "claude", "/a"}}, rpath.Style{}, ""},
		{"a verified file over a folder-only one before it", record(7, "claude", "S", claude(7, "stale", "/a", "")+script.GS+claude(7, "live", "/a", "S")+script.GS), []Running{{7, "claude", "/a"}}, rpath.Style{}, "live"},
		{"the session_meta's id over the file name", record(7, "codex", "S", "7"+script.GS+"/x/rollout-2026-11-20T03-10-00-"+rolloutID+"_01a1f6a0-1111-7222-8333-944455556666.jsonl"+script.GS+sessionMeta(rolloutID, false)+"\n"), []Running{{7, "codex", "/a"}}, rpath.Style{}, rolloutID},
		{"the file name when the session_meta names no id", record(7, "codex", "S", "7"+script.GS+"/x/rollout-2026-11-20T02-28-48-"+rolloutID+".jsonl"+script.GS+`{"type":"session_meta","payload":{"source":"cli"}}`+"\n"), []Running{{7, "codex", "/a"}}, rpath.Style{}, rolloutID},
		{"a thread resumed in another folder than it began in", standing(7, "codex", "S", "/a", "7"+script.GS+"/x/rollout-2026-11-20T02-28-48-"+rolloutID+".jsonl"+script.GS+`{"type":"session_meta","payload":{"id":"`+rolloutID+`","cwd":"/b","thread_source":"user"}}`+"\n"), []Running{{7, "codex", "/a"}}, rpath.Style{}, rolloutID},
		{"a thread held by a process standing elsewhere", standing(7, "codex", "S", "/b", "7"+script.GS+"/x/rollout-2026-11-20T02-28-48-"+rolloutID+".jsonl"+script.GS+sessionMeta(rolloutID, false)+"\n"), []Running{{7, "codex", "/a"}}, rpath.Style{}, ""},
		{"a subagent's thread alone", record(7, "codex", "S", "7"+script.GS+"/x/rollout-2026-11-20T02-28-48-"+rolloutID+".jsonl"+script.GS+sessionMeta(rolloutID, true)+"\n"), []Running{{7, "codex", "/a"}}, rpath.Style{}, ""},
		{"a rollout whose first line is no session_meta", record(7, "codex", "S", "7"+script.GS+"/x/rollout-2026-11-20T02-28-48-"+rolloutID+".jsonl"+script.GS+`{"type":"response_item","payload":{}}`+"\n"), []Running{{7, "codex", "/a"}}, rpath.Style{}, ""},
		{"a rollout still empty", record(7, "codex", "S", "7"+script.GS+"/x/rollout-2026-11-20T02-28-48-"+rolloutID+".jsonl"+script.GS+"\n"), []Running{{7, "codex", "/a"}}, rpath.Style{}, ""},
		{"an id that is not one", record(7, "claude", "S", claude(7, "--resume x", "/a", "S")), []Running{{7, "claude", "/a"}}, rpath.Style{}, ""},
		{"no rollout open", record(7, "codex", "S", ""), []Running{{7, "codex", "/a"}}, rpath.Style{}, ""},
		{"a pid not asked about", record(9, "codex", "S", "/x/rollout-2026-11-20T02-28-48-"+rolloutID+".jsonl"), []Running{{7, "codex", "/a"}}, rpath.Style{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseSessionsByPID(c.out, c.procs, c.style)
			if got[c.procs[0].PID] != c.want {
				t.Fatalf("got %v, want %q", got, c.want)
			}
		})
	}
}
