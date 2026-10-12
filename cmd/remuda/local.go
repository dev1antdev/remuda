package main

import (
	"os"
	"path/filepath"

	"github.com/dev1antdev/remuda/internal/sshx"
)

// socketsUnder keeps the ssh control and forward sockets in the state dir
// (0700, the person's own) rather than the shared temp dir, where another
// user of the machine could leave a socket at the name.
func socketsUnder(dir string) {
	d := filepath.Join(dir, "ssh")
	if err := os.MkdirAll(d, 0o700); err == nil {
		sshx.SocketDir = d
	}
}

// interactive says a person is at the terminal: stderr is one.
func interactive() bool {
	fi, err := os.Stderr.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
