// Package herdr is the dictionary of a machine: the snapshot, a pane, a workspace, a tab, the layout, the statuses, the
// events — and the few things remuda still asks about the herdr a machine came from (its socket, its binary, whether its
// server answers), for the move at the person's press. remuda speaks to remuda-core (internal/core), never to herdr.
package herdr

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DefaultSocket is herdr's socket under a home directory.
func DefaultSocket(home string) string {
	return filepath.Join(home, ".config", "herdr", "herdr.sock")
}

// FindBinary resolves the herdr binary: the name on PATH, else where installers put it
// (a service or an IDE starts remuda with a bare PATH that lacks ~/.local/bin).
func FindBinary(name string) (string, error) {
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	if strings.Contains(name, "/") {
		return "", fmt.Errorf("%s: not an executable", name)
	}
	home, _ := os.UserHomeDir()
	dirs := []string{filepath.Join(home, ".local", "bin"), "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin"}
	for _, dir := range dirs {
		p := filepath.Join(dir, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s not found in PATH or %s", name, strings.Join(dirs, ", "))
}

// SocketPath returns $HERDR_SOCKET or herdr's default socket location.
func SocketPath() string {
	if p := os.Getenv("HERDR_SOCKET"); p != "" {
		return p
	}
	if p := os.Getenv("HERDR_SOCKET_PATH"); p != "" {
		return p
	}
	return DefaultSocket(mustHome())
}

// mustHome is this user's home, "" when it cannot be read.
func mustHome() string {
	home, _ := os.UserHomeDir()
	return home
}
