package sshx

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestASessionRidesTheMasterOrFails: with the master gone, ssh connects to the host by itself for a session — a whole
// handshake for every script and every nc RPC, which keep answering that way, so the machine never learns its master
// died. A session fails instead; nothing listens on the loopback's port 1, where a connection of its own would knock.
func TestASessionRidesTheMasterOrFails(t *testing.T) {
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("no ssh on PATH")
	}
	dir, err := os.MkdirTemp("", "sx") // short: a control path holds 104 bytes
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	c := &Conn{Host: "ssh://127.0.0.1:1", ctl: filepath.Join(dir, "gone.ctl")}
	_, err = c.Run("true", 10*time.Second)
	if err == nil {
		t.Fatal("a session with no master ran")
	}
	if strings.Contains(err.Error(), "connect to host") {
		t.Fatalf("the session went around the master: %v", err)
	}
	conn, err := c.DialExec("true", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	buf := make([]byte, 64)
	if _, err := conn.Read(buf); err == nil || strings.Contains(err.Error(), "connect to host") {
		t.Fatalf("the nc bridge went around the master: %v", err)
	}
}

// TestOnlyTheMasterConnectsByItself: the master is the one connection to the host, made as the person's ssh config
// says (a jump host too); everything else asks it, the scripts, the bridge and the attach alike.
func TestOnlyTheMasterConnectsByItself(t *testing.T) {
	calls := fakeSSH(t)
	c := &Conn{Host: "gog.remote", ctl: "/tmp/remuda-ssh-test.ctl"}
	if _, err := c.Run("true", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := c.Stream("true", 5*time.Second, func([]byte) {}); err != nil {
		t.Fatal(err)
	}
	conn, err := c.DialExec("nc -U /x.sock", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(conn) // the stand-in has said its line once the bridge ends
	conn.Close()
	sessions := recorded(t, calls)
	if len(sessions) != 3 {
		t.Fatalf("%d sessions recorded, want the script, the stream and the bridge:\n%s", len(sessions), strings.Join(sessions, "\n"))
	}
	sessions = append(sessions, strings.Join(c.AttachCmd("herdr", "", "term_1").Args[1:], " "))
	for _, line := range sessions {
		if !strings.Contains(line, "-o ProxyCommand=false") {
			t.Errorf("a session may connect by itself: %s", line)
		}
	}
	if master := strings.Join(c.cmd("-M", "-N", c.Host).Args, " "); strings.Contains(master, "ProxyCommand") {
		t.Errorf("the master must connect as configured: %s", master)
	}
}
