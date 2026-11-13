//go:build !windows

package proc

import (
	"os/exec"
	"syscall"
)

// Group puts the command in a process group of its own and has its cancel end the whole group with sig.
func Group(cmd *exec.Cmd, sig syscall.Signal) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, sig) }
}

// Detach starts the command in a session of its own: it stays when remuda exits, and a signal to remuda's group never reaches it.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
