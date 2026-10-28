package sshx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSocketPathLeavesOpenSSHItsSuffix keeps OpenSSH's 17-byte suffix within a 104-byte sockaddr:
// a Mac's state dir with a ten-letter user name went two bytes over, and every remote stayed "error".
func TestSocketPathLeavesOpenSSHItsSuffix(t *testing.T) {
	defer func(d string) { SocketDir = d }(SocketDir)

	SocketDir = "/Users/dev1antdev/Library/Application Support/remuda/ssh"
	p := tmpPath("remuda-ssh-", "gog.remote", ".ctl")
	if strings.HasPrefix(p, SocketDir) {
		t.Fatalf("a %d-byte state dir was used: %s is %d bytes", len(SocketDir), p, len(p))
	}
	if filepath.Dir(p) != filepath.Clean(os.TempDir()) {
		t.Fatalf("fell back to %s, not the temp dir", p)
	}

	SocketDir = "/home/sergii/.config/remuda/ssh"
	p = tmpPath("remuda-ssh-", "gog.remote", ".ctl")
	if !strings.HasPrefix(p, SocketDir+"/") || !strings.HasSuffix(p, ".ctl") || len(p) > socketPathMax {
		t.Fatalf("a short state dir was not used as is: %s (%d bytes)", p, len(p))
	}

	// the longest state dir that still fits, and the first that does not
	fits := strings.Repeat("d", socketPathMax-1-len("remuda-ssh-0123456789abcdef.ctl"))
	SocketDir = fits
	if p := tmpPath("remuda-ssh-", "x", ".ctl"); !strings.HasPrefix(p, fits+"/") || len(p) != socketPathMax {
		t.Fatalf("the last fitting dir was not used: %s (%d bytes)", p, len(p))
	}
	SocketDir = fits + "d"
	if p := tmpPath("remuda-ssh-", "x", ".ctl"); strings.HasPrefix(p, fits) {
		t.Fatalf("one byte over was still used: %s (%d bytes)", p, len(p))
	}
}

// TestTempDirSocketsAreTheInstancesOwn: two instances whose state dirs are too long for a socket both fall back to
// the temp dir, where the name must still tell them apart, or a scratch instance reuses the real one's master.
func TestTempDirSocketsAreTheInstancesOwn(t *testing.T) {
	defer func(d string) { SocketDir = d }(SocketDir)
	SocketDir = "/Users/dev1antdev/Library/Application Support/remuda/ssh"
	real := tmpPath("remuda-ssh-", "gog.remote", ".ctl")
	again := tmpPath("remuda-ssh-", "gog.remote", ".ctl")
	SocketDir = "/private/tmp/claude-501/-Users-dev1antdev-Projects-mardeus-remuda/scratchpad/state/ssh"
	scratch := tmpPath("remuda-ssh-", "gog.remote", ".ctl")
	if real != again {
		t.Fatalf("the same instance got two names: %s, %s", real, again)
	}
	if real == scratch {
		t.Fatalf("two instances share %s", real)
	}
	if filepath.Dir(real) != filepath.Clean(os.TempDir()) || filepath.Dir(scratch) != filepath.Clean(os.TempDir()) {
		t.Fatalf("not in the temp dir: %s, %s", real, scratch)
	}
}
