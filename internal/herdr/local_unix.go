//go:build !windows

package herdr

import (
	"net"
	"os"
	"path/filepath"
	"time"
)

// DialLocal opens a connection to the herdr server on this machine: its unix socket.
func DialLocal(socket string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("unix", socket, timeout)
}

// localSocket is herdr's default socket on this machine: under ~/.config/herdr.
func localSocket() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "herdr", "herdr.sock")
}
