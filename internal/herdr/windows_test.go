//go:build windows

package herdr_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/wintest"
)

// herdrExe is herdr's stable alias, where its Windows installer keeps the current release.
func herdrExe(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("LOCALAPPDATA") + `\Programs\Herdr\bin\herdr.exe`
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("herdr for Windows is not installed: %v", err)
	}
	return bin
}

// TestOnWindowsTheServerStartsOverSSHAndOutlivesTheSession is the cockpit's start of herdr on a Windows machine: through the
// runner's OpenSSH, Git Bash its shell, the session ends at once and the server stays after it.
func TestOnWindowsTheServerStartsOverSSHAndOutlivesTheSession(t *testing.T) {
	wintest.Live(t)
	sock := herdr.OwnSocket()
	if _, _, err := herdr.NewClient(sock).Ping(); err == nil {
		t.Log("a server runs already; stopping it to start one the cockpit's way")
		if out, err := exec.Command(herdrExe(t), "server", "stop").CombinedOutput(); err != nil {
			t.Fatalf("stop: %v %s", err, out)
		}
		time.Sleep(2 * time.Second)
	}
	bin := strings.ReplaceAll(herdrExe(t), `\`, "/")
	started := time.Now()
	out, err := wintest.Remote(t).Run(herdr.StartServerScriptOn("windows", bin, sock), 60*time.Second)
	if err != nil {
		t.Fatalf("the start over ssh: %v\n%s", err, out)
	}
	t.Logf("the session ended %s after it began", time.Since(started).Round(time.Millisecond))
	time.Sleep(3 * time.Second)
	version, protocol, err := herdr.NewClient(sock).Ping()
	if err != nil {
		t.Fatalf("the server did not outlive the ssh session: %v", err)
	}
	t.Logf("herdr %s (protocol %d) answers at %s, the pipe %s", version, protocol, sock, herdr.PipeName(sock))
}

// TestOnWindowsTheNamedPipeTakesARequestWhileAReadWaits is DialLocal's own reason for overlapped I/O: one connection, the
// answer read on one goroutine while the request is written on another, as the bridge does.
func TestOnWindowsTheNamedPipeTakesARequestWhileAReadWaits(t *testing.T) {
	wintest.Live(t)
	conn, err := herdr.DialLocal(herdr.OwnSocket(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	answer := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			answer <- "error " + err.Error()
			return
		}
		answer <- string(buf[:n])
	}()
	time.Sleep(300 * time.Millisecond)
	if _, err := conn.Write([]byte(`{"id":"w1","method":"ping","params":{}}` + "\n")); err != nil {
		t.Fatalf("the write while a read waits: %v", err)
	}
	select {
	case got := <-answer:
		if !strings.Contains(got, `"pong"`) {
			t.Fatalf("herdr answered %q", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no answer: the write never reached herdr while the read waited")
	}
}
