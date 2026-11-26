// Package coreapi is the dictionary of a machine as remuda-core tells it: the snapshot, a pane, a workspace, a tab, the
// layout, the statuses and the events — the port every package that asks a machine speaks. Its one implementation is
// the adapter over the machine's remuda-core (internal/core). The herdr a machine came from: its socket and binary
// here, and whether its server answers (and its stop) in internal/herdrollback, for the move at the person's press.
package coreapi

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
