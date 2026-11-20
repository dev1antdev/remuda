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

// TestHelperHoldsFile is not a test: run as a child process it holds REMUDA_TEST_HOLD open until it is killed, as
// Codex holds its rollout, and keeps its environment where `ps` and /proc show it, as Claude Code's is read.
func TestHelperHoldsFile(t *testing.T) {
	path := os.Getenv("REMUDA_TEST_HOLD")
	if path == "" {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		os.Exit(2)
	}
	defer f.Close()
	time.Sleep(time.Minute)
	os.Exit(0)
}

// running starts the holder, directly or as the child of a shell, and answers the pid the lookup is asked about.
func running(t *testing.T, hold string, env []string, asChild bool) int {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the lookup reads ps, /proc and lsof")
	}
	args := []string{"-test.run=^TestHelperHoldsFile$"}
	cmd := exec.Command(os.Args[0], args...)
	if asChild {
		cmd = exec.Command("sh", "-c", `"$0" "$1" & wait`, os.Args[0], args[0])
	}
	cmd.Env = append(append(os.Environ(), "REMUDA_TEST_HOLD="+hold), env...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = exec.Command("pkill", "-P", strconv.Itoa(cmd.Process.Pid)).Run()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	return cmd.Process.Pid
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

// remoteReader reads through a machine whose scripts run in this machine's sh, as a remote's would over ssh.
func remoteReader(t *testing.T) (*Reader, *shell.Fake) {
	host := &shell.Fake{Name: "gog", Remote: true, Dir: os.Getenv("HOME"), Answer: func(line string) (string, error) {
		out, err := exec.Command("sh", "-c", line).Output()
		return string(out), err
	}}
	return NewReader(host), host
}

func TestClaudeSessionComesFromItsConfigDirAndItsStart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	config := filepath.Join(home, "elsewhere")
	hold := filepath.Join(home, "held")
	if err := os.WriteFile(hold, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	pid := running(t, hold, []string{"CLAUDE_CONFIG_DIR=" + config}, false)
	start := procStart(t, pid)
	writeSessionFile(t, filepath.Join(config, "sessions"), pid, "4c9d6f0e-1111-4222-8333-944455556666", "/work/api", start)
	writeSessionFile(t, filepath.Join(home, ".claude", "sessions"), pid, "0000aaaa-default-dir-not-this-one", "/work/api", start)

	reader, _ := remoteReader(t)
	got, err := reader.SessionsOf([]Running{{PID: pid, Agent: "claude", CWD: "/work/api"}})
	if err != nil {
		t.Fatal(err)
	}
	if got[pid] != "4c9d6f0e-1111-4222-8333-944455556666" {
		t.Fatalf("session = %q, want the one in CLAUDE_CONFIG_DIR", got[pid])
	}
}

func TestClaudeSessionLeftByAnEndedProcessIsNotTaken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	hold := filepath.Join(home, "held")
	if err := os.WriteFile(hold, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	pid := running(t, hold, nil, false)
	writeSessionFile(t, filepath.Join(home, ".claude", "sessions"), pid, "4c9d6f0e-1111-4222-8333-944455556666", "/work/api", "Mon Jan  1 00:00:00 2024")

	reader, _ := remoteReader(t)
	got, err := reader.SessionsOf([]Running{{PID: pid, Agent: "claude", CWD: "/work/api"}})
	if err != nil {
		t.Fatal(err)
	}
	if id, ok := got[pid]; ok {
		t.Fatalf("a file whose start is not the process's gave %q", id)
	}
}

func TestCodexSessionIsTheRolloutItsProcessHoldsOpen(t *testing.T) {
	for _, asChild := range []bool{false, true} {
		t.Run(fmt.Sprintf("child=%v", asChild), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			dir := filepath.Join(home, ".codex", "sessions", "2026", "11", "20")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			rollout := filepath.Join(dir, "rollout-2026-11-20T02-28-48-"+rolloutID+".jsonl")
			if err := os.WriteFile(rollout, []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			pid := running(t, rollout, nil, asChild)

			reader, _ := remoteReader(t)
			var got map[int]string
			for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
				var err error
				if got, err = reader.SessionsOf([]Running{{PID: pid, Agent: "codex", CWD: home}}); err != nil {
					t.Fatal(err)
				}
				if got[pid] != "" {
					break
				}
			}
			if got[pid] != rolloutID {
				t.Fatalf("session = %q, want %s", got[pid], rolloutID)
			}
		})
	}
}

func TestSessionsOfAsksTheMachineOnceForEveryAgent(t *testing.T) {
	answer := "41\037claude\037Thu Nov 19 00:36:10 2026\037" +
		`{"pid":41,"sessionId":"aaaa-1","cwd":"/w","procStart":"Thu Nov 19 00:36:10 2026"}` + "\036\n" +
		"52\037codex\037Thu Nov 19 00:40:00 2026\037/home/me/.codex/sessions/2026/11/19/rollout-2026-11-19T00-40-01-" + rolloutID + ".jsonl\036\n"
	host := &shell.Fake{Name: "gog", Remote: true, Dir: "/home/me", Answer: func(string) (string, error) { return answer, nil }}
	got, err := NewReader(host).SessionsOf([]Running{{PID: 41, Agent: "claude", CWD: "/w"}, {PID: 52, Agent: "codex", CWD: "/w"}, {PID: 60, Agent: "gemini"}})
	if err != nil {
		t.Fatal(err)
	}
	if got[41] != "aaaa-1" || got[52] != rolloutID || len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	scripts := host.Scripts()
	if len(scripts) != 1 || !strings.Contains(scripts[0], "\ns 41 claude\n") || !strings.Contains(scripts[0], "\ns 52 codex\n") || strings.Contains(scripts[0], "gemini") {
		t.Fatalf("scripts = %q", scripts)
	}

	host = &shell.Fake{Name: "gog", Remote: true}
	if got, err := NewReader(host).SessionsOf([]Running{{PID: 60, Agent: "gemini"}, {PID: 1, Agent: "claude"}}); err != nil || len(got) != 0 || len(host.Scripts()) != 0 {
		t.Fatalf("nothing to look up: got %v, %v, scripts %q", got, err, host.Scripts())
	}
}

func TestParseSessionsByPID(t *testing.T) {
	record := func(pid int, agent, start, payload string) string {
		return strconv.Itoa(pid) + script.US + agent + script.US + start + script.US + payload + script.RS + "\n"
	}
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
		{"start matches", record(7, "claude", "S", claude(7, "id-1", "/a", "S")), []Running{{7, "claude", "/b"}}, rpath.Style{}, "id-1"},
		{"start differs", record(7, "claude", "S", claude(7, "id-1", "/a", "T")), []Running{{7, "claude", "/a"}}, rpath.Style{}, ""},
		{"another pid's file", record(7, "claude", "S", claude(8, "id-1", "/a", "S")), []Running{{7, "claude", "/a"}}, rpath.Style{}, ""},
		{"no start here, the folder decides", record(7, "claude", "", claude(7, "id-1", `C:\Work\Api`, "S")), []Running{{7, "claude", `c:/work/api`}}, windows, "id-1"},
		{"no start here, another folder", record(7, "claude", "", claude(7, "id-1", `C:\Work\Api`, "S")), []Running{{7, "claude", `C:\Work\Web`}}, windows, ""},
		{"no session file", record(7, "claude", "S", ""), []Running{{7, "claude", "/a"}}, rpath.Style{}, ""},
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
