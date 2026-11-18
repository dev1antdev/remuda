package sshx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/script"
)

// TestForwardBackAsksTheMasterForAReverseForward: the door on a remote is `-O forward -R <its loopback port>:<ours>` on the master.
func TestForwardBackAsksTheMasterForAReverseForward(t *testing.T) {
	c := &Conn{Host: "gog.remote", ctl: "/tmp/remuda-ssh-test.ctl"}
	args := c.forwardBackCmd(7318, 7317).Args
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-S /tmp/remuda-ssh-test.ctl") || !strings.Contains(joined, "BatchMode=yes") {
		t.Fatalf("not on the master, or interactive: %s", joined)
	}
	if !strings.HasSuffix(joined, "-O forward -R 127.0.0.1:7318:127.0.0.1:7317 gog.remote") {
		t.Fatalf("the reverse forward reads wrong: %s", joined)
	}
	if forwardBackSpec(7318, 7441) != "127.0.0.1:7318:127.0.0.1:7441" {
		t.Fatal("spec")
	}
}

// answeringSSH puts an `ssh` first on PATH that prints the answer, says the complaint and exits with the code.
func answeringSSH(t *testing.T, answer, complaint string, code string) {
	t.Helper()
	dir := t.TempDir()
	stub := "#!/bin/sh\nprintf '%s' " + script.Quote(answer) + "\nprintf '%s' " + script.Quote(complaint) + " >&2\nexit " + code + "\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestForwardBackAnyNamesThePortTheHostPicked: asked for port 0, the master prints the port it got.
func TestForwardBackAnyNamesThePortTheHostPicked(t *testing.T) {
	c := &Conn{Host: "gog.remote", ctl: "/tmp/remuda-ssh-test.ctl"}
	if joined := strings.Join(c.forwardBackCmd(0, 7317).Args, " "); !strings.HasSuffix(joined, "-O forward -R 127.0.0.1:0:127.0.0.1:7317 gog.remote") {
		t.Fatalf("the reverse forward reads wrong: %s", joined)
	}
	answeringSSH(t, "50236\n", "", "0")
	if port, err := c.ForwardBackAny(7317); err != nil || port != 50236 {
		t.Fatalf("port %d, %v", port, err)
	}
	for _, answer := range []string{"", "Allocated port\n", "0\n", "70000\n"} {
		answeringSSH(t, answer, "", "0")
		if port, err := c.ForwardBackAny(7317); err == nil {
			t.Errorf("the answer %q gave port %d", answer, port)
		}
	}
	answeringSSH(t, "", "remote port forwarding failed for listen port 0", "255")
	if _, err := c.ForwardBackAny(7317); err == nil || !strings.Contains(err.Error(), "forwarding failed") {
		t.Fatalf("a refusal says why: %v", err)
	}
}
