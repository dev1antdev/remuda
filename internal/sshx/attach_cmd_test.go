package sshx

import (
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/script"
)

// TestAttachCmdCarriesTheMachinesSocket: a herdr listening off its default path is reached by the attach too, the
// socket in the environment as the local attach's is, and through sh whatever the login shell there.
func TestAttachCmdCarriesTheMachinesSocket(t *testing.T) {
	c := &Conn{Host: "gog.remote", ctl: "/tmp/remuda-ssh-test.ctl"}
	args := c.AttachCmd("/home/me/.local/bin/herdr", "/run/herdr/herdr.sock", "term_1").Args
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, " -tt gog.remote ") {
		t.Fatalf("not a tty session on the host: %s", joined)
	}
	want := "sh -c " + script.Quote("HERDR_SOCKET='/run/herdr/herdr.sock' HERDR_SOCKET_PATH='/run/herdr/herdr.sock' exec '/home/me/.local/bin/herdr' terminal attach 'term_1'")
	if got := args[len(args)-1]; got != want {
		t.Fatalf("remote command\n got %s\nwant %s", got, want)
	}
	args = c.AttachCmd("herdr", "", "term_1").Args
	if got, want := args[len(args)-1], "sh -c "+script.Quote("exec 'herdr' terminal attach 'term_1'"); got != want {
		t.Fatalf("without a socket\n got %s\nwant %s", got, want)
	}
}
