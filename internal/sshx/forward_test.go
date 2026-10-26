package sshx

import (
	"strings"
	"testing"
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
