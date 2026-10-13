package herdr

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestEnsureServerStartsOne starts a throwaway herdr server on a socket of its own, never the person's.
func TestEnsureServerStartsOne(t *testing.T) {
	bin, err := exec.LookPath("herdr")
	if err != nil {
		t.Skip("no herdr on PATH")
	}
	home, err := os.MkdirTemp("", "hs") // short: a unix socket path has ~100 bytes
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(home)
	t.Setenv("HOME", home)
	sock := filepath.Join(home, "herdr.sock")
	defer func() {
		cmd := exec.Command(bin, "server", "stop")
		cmd.Env = append(os.Environ(), "HERDR_SOCKET="+sock, "HERDR_SOCKET_PATH="+sock)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Logf("stop: %v %s", err, out)
		}
		for i := 0; i < 20; i++ {
			if _, _, err := NewClient(sock).Ping(); err != nil {
				return
			}
			time.Sleep(250 * time.Millisecond)
		}
		t.Errorf("the throwaway server still answers at %s", sock)
	}()
	if err := EnsureServer(bin, sock); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewClient(sock).Ping(); err != nil {
		t.Fatalf("ping after start: %v", err)
	}
	if err := EnsureServer(bin, sock); err != nil { // a ping only now
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "herdr-server.out")); err != nil {
		t.Errorf("no server log: %v", err)
	}
}
