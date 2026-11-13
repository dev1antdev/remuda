package proc

import (
	"os/exec"
	"syscall"
)

// Group is the process alone on Windows, which has no process groups to signal: its cancel kills it, and the
// command's WaitDelay keeps a child left holding the output from being waited for.
func Group(cmd *exec.Cmd, sig syscall.Signal) {}

// Detach starts the command in a process group of its own with no console, so remuda's end does not take it along.
func Detach(cmd *exec.Cmd) {
	const detachedProcess = 0x00000008
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess}
}
