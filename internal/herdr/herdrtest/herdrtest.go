// Package herdrtest starts a herdr server of a test's own, for the tests that speak herdr's protocol for real.
package herdrtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// Server starts a throwaway herdr server and answers its socket, skipping the test without herdr. Its socket and its
// HOME are both temporary: herdr restores the saved session of the HOME it runs under — the person's workspaces, their
// agents resumed — and saves its own over it. It is stopped through its socket when the test ends.
func Server(t testing.TB) string {
	t.Helper()
	bin, err := exec.LookPath("herdr")
	if err != nil {
		t.Skip("no herdr on PATH")
	}
	home, err := os.MkdirTemp("", "hs") // short: a unix socket path has ~100 bytes
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	sock := filepath.Join(home, "herdr.sock")
	t.Setenv("HERDR_SOCKET", sock)
	t.Setenv("HERDR_SOCKET_PATH", sock)
	t.Cleanup(func() {
		defer os.RemoveAll(home)
		cmd := exec.Command(bin, "server", "stop")
		cmd.Env = append(os.Environ(), "HOME="+home, "HERDR_SOCKET="+sock, "HERDR_SOCKET_PATH="+sock)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Logf("stop: %v %s", err, out)
		}
		for i := 0; i < 20; i++ {
			if _, _, err := herdr.NewClient(sock).Ping(); err != nil {
				return
			}
			time.Sleep(250 * time.Millisecond)
		}
		t.Errorf("the throwaway server still answers at %s", sock)
	})
	if err := herdr.StartServerNow(bin, sock); err != nil {
		t.Fatal(err)
	}
	return sock
}
