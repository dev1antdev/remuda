package awake

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

// standIns is a temp HOME whose ~/.local/bin holds a caffeinate and a systemd-inhibit that just sleep until killed,
// and a uname that answers AWAKE_TEST_OS, so the scripts run the same on any machine and start nothing real.
func standIns(t *testing.T, osName string) (home, stateDir string) {
	t.Helper()
	home = t.TempDir()
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	inhibitor := "#!/bin/sh\nsleep 300 &\np=$!\ntrap 'kill $p 2>/dev/null; exit 0' TERM INT HUP\nwait $p\n"
	for _, name := range []string{"caffeinate", "systemd-inhibit"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(inhibitor), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(bin, "uname"), []byte("#!/bin/sh\nprintf '%s\\n' \"$AWAKE_TEST_OS\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("AWAKE_TEST_OS", osName)
	out, err := exec.Command("sh", "-c", `command -v uname`).Output()
	if err != nil || strings.TrimSpace(string(out)) != filepath.Join(bin, "uname") {
		t.Fatalf("the stand-ins are not what the scripts find: uname is %q (%v)", strings.TrimSpace(string(out)), err)
	}
	stateDir = filepath.Join(home, "state")
	return home, stateDir
}

func pidOf(t *testing.T, file string) int {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("no pidfile: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatalf("pidfile holds %q", b)
	}
	return pid
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func waitGone(t *testing.T, pid int) {
	t.Helper()
	for i := 0; i < 40; i++ {
		if !alive(pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("pid %d still runs", pid)
}

func TestHoldStartsAnInhibitorKeepsItAndLetsItGo(t *testing.T) {
	_, stateDir := standIns(t, "Darwin")
	host := shell.Local{Name: "local"}
	holder := ShellHolder{Dir: stateDir}
	dirLine, file := holder.dirLine(host), holder.file(host)
	out, err := shell.Text(host, HoldScript(dirLine, file), 10*time.Second)
	if err != nil || !strings.HasPrefix(out, "started ") {
		t.Fatalf("hold: %q %v", out, err)
	}
	pid := pidOf(t, filepath.Join(stateDir, "awake.pid"))
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGTERM) })
	if !alive(pid) {
		t.Fatalf("pid %d is not running", pid)
	}
	if strconv.Itoa(pid) != strings.TrimSpace(strings.TrimPrefix(out, "started ")) {
		t.Fatalf("the script said %q, the pidfile %d", out, pid)
	}
	args, _ := exec.Command("ps", "-o", "args=", "-p", strconv.Itoa(pid)).Output()
	if !strings.Contains(string(args), "caffeinate -i") {
		t.Fatalf("the process is not caffeinate -i: %q", args)
	}
	out, err = shell.Text(host, HoldScript(dirLine, file), 10*time.Second)
	if err != nil || strings.TrimSpace(out) != "held "+strconv.Itoa(pid) {
		t.Fatalf("a second hold should keep the running one: %q %v", out, err)
	}
	if err := holder.Hold(host); err != nil {
		t.Fatalf("Hold: %v", err)
	}
	if err := holder.Release(host); err != nil {
		t.Fatalf("Release: %v", err)
	}
	waitGone(t, pid)
	if _, err := os.Stat(filepath.Join(stateDir, "awake.pid")); !os.IsNotExist(err) {
		t.Fatalf("pidfile still there: %v", err)
	}
	if err := holder.Release(host); err != nil {
		t.Fatalf("a release with nothing held: %v", err)
	}
}

func TestHoldOnLinuxUsesSystemdInhibit(t *testing.T) {
	_, stateDir := standIns(t, "Linux")
	host := shell.Local{Name: "local"}
	holder := ShellHolder{Dir: stateDir}
	if err := holder.Hold(host); err != nil {
		t.Fatalf("Hold: %v", err)
	}
	pid := pidOf(t, filepath.Join(stateDir, "awake.pid"))
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGTERM) })
	args, _ := exec.Command("ps", "-o", "args=", "-p", strconv.Itoa(pid)).Output()
	if !strings.Contains(string(args), "systemd-inhibit --what=idle:sleep --who=remuda") {
		t.Fatalf("the process is not systemd-inhibit: %q", args)
	}
	if err := holder.Release(host); err != nil {
		t.Fatal(err)
	}
	waitGone(t, pid)
}

func TestHoldOnAnUnknownSystemIsAnError(t *testing.T) {
	_, stateDir := standIns(t, "FreeBSD")
	host := shell.Local{Name: "local"}
	err := ShellHolder{Dir: stateDir}.Hold(host)
	if err == nil || !strings.Contains(err.Error(), "no sleep inhibitor on local") || !strings.Contains(err.Error(), "FreeBSD") {
		t.Fatalf("err %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(stateDir, "awake.pid")); !os.IsNotExist(statErr) {
		t.Fatal("a pidfile was written with nothing started")
	}
}

func TestAStalePidfileNamingAnotherProcessIsNotTrusted(t *testing.T) {
	_, stateDir := standIns(t, "Darwin")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	me := os.Getpid()
	if err := os.WriteFile(filepath.Join(stateDir, "awake.pid"), []byte(strconv.Itoa(me)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	host := shell.Local{Name: "local"}
	holder := ShellHolder{Dir: stateDir}
	out, err := shell.Text(host, HoldScript(holder.dirLine(host), holder.file(host)), 10*time.Second)
	if err != nil || !strings.HasPrefix(out, "started ") {
		t.Fatalf("hold over a stale pidfile: %q %v", out, err)
	}
	pid := pidOf(t, filepath.Join(stateDir, "awake.pid"))
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGTERM) })
	if pid == me {
		t.Fatal("the test's own pid was taken for the inhibitor")
	}
	if err := os.WriteFile(filepath.Join(stateDir, "awake.pid"), []byte(strconv.Itoa(me)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := holder.Release(host); err != nil {
		t.Fatal(err)
	}
	if !alive(me) {
		t.Fatal("unreachable: the test was killed")
	}
}

func TestScriptsNameThePidfileByMachine(t *testing.T) {
	holder := ShellHolder{Dir: "/tmp/state dir", Name: "Sergii's Mac"}
	var scripts []string
	fake := &shell.Fake{Name: "gog", Remote: true, Answer: func(script string) (string, error) {
		scripts = append(scripts, script)
		return "started 4242\n", nil
	}}
	if err := holder.Hold(fake); err != nil {
		t.Fatal(err)
	}
	if err := holder.Release(fake); err != nil {
		t.Fatal(err)
	}
	for _, s := range scripts {
		if !strings.Contains(s, `uname -s`) || !strings.Contains(s, `Library/Application Support/remuda`) || !strings.Contains(s, `XDG_CONFIG_HOME`) {
			t.Fatalf("a remote's script does not compute remuda's state dir there:\n%s", s)
		}
		if !strings.Contains(s, `f="$d/awake-Sergii-s-Mac.pid"`) {
			t.Fatalf("a remote's pidfile is not named by the holder:\n%s", s)
		}
	}
	local := &shell.Fake{Name: "local", Answer: func(string) (string, error) { return "held 7\n", nil }}
	if err := holder.Hold(local); err != nil {
		t.Fatal(err)
	}
	s := local.Scripts()[0]
	if !strings.Contains(s, `d='/tmp/state dir'`) || !strings.Contains(s, `f="$d/awake.pid"`) {
		t.Fatalf("the local script does not use this instance's state dir:\n%s", s)
	}
}

func TestHoldResultReadsTheAnswer(t *testing.T) {
	for _, c := range []struct{ out, want string }{
		{"started 12\n", ""},
		{"held 12\n", ""},
		{"none Linux\n", "no sleep inhibitor on box: neither caffeinate nor systemd-inhibit for Linux"},
		{"none\n", "no sleep inhibitor on box: neither caffeinate nor systemd-inhibit for this system"},
		{"sh: something\n", `the hold on box answered "sh: something"`},
	} {
		err := holdResult(c.out, "box")
		got := ""
		if err != nil {
			got = err.Error()
		}
		if got != c.want {
			t.Errorf("%q: got %q, want %q", c.out, got, c.want)
		}
	}
}
