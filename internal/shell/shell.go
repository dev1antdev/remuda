// Package shell is the port every machine-side read and command goes through:
// a POSIX script run on a machine, here through sh -c or over its ssh master,
// its output returned whole or streamed as it comes. The packages that only
// run scripts take a Host and know nothing of ssh or herdr; a test hands them
// a Fake.
package shell

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Shell runs POSIX scripts on one machine.
type Shell interface {
	// Run returns the script's stdout. A script that ended non-zero is an *ExitError; one that outran the timeout is an error too.
	Run(script string, timeout time.Duration) ([]byte, error)
	// Stream hands sink the output, stdout and stderr together, as it comes; the exit status is the error.
	Stream(script string, timeout time.Duration, sink func([]byte)) error
}

// Host is a machine seen through its shell, for the packages that key their work by machine and need nothing of herdr.
type Host interface {
	Shell
	ID() string
	Label() string
	Home() string
	Local() bool
}

// ExitError is a script that ran and ended non-zero.
type ExitError struct {
	Code   int
	Stderr string
}

func (e *ExitError) Error() string {
	if e.Stderr != "" {
		return e.Stderr
	}
	return fmt.Sprintf("exit status %d", e.Code)
}

// Text is Run with the output as text.
func Text(sh Shell, script string, timeout time.Duration) (string, error) {
	out, err := sh.Run(script, timeout)
	return string(out), err
}

// Local runs scripts on this machine.
type Local struct {
	Name string // the machine's id
}

func (l Local) ID() string { return l.Name }

func (l Local) Label() string { return l.Name }

func (Local) Local() bool { return true }

func (Local) Home() string {
	home, _ := os.UserHomeDir()
	return home
}

func (Local) Run(script string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	cmd.WaitDelay = 2 * time.Second // a killed sh may leave children on the pipes; do not wait for them
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if err == nil {
		return out.Bytes(), nil
	}
	if ctx.Err() != nil {
		return out.Bytes(), fmt.Errorf("timed out after %s", timeout)
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return out.Bytes(), &ExitError{Code: exit.ExitCode(), Stderr: strings.TrimSpace(errb.String())}
	}
	return out.Bytes(), err
}

func (Local) Stream(script string, timeout time.Duration, sink func([]byte)) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // its own group: a timeout ends the children too
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = 2 * time.Second
	pipeReader, pipeWriter := io.Pipe()
	cmd.Stdout, cmd.Stderr = pipeWriter, pipeWriter
	if err := cmd.Start(); err != nil {
		pipeWriter.Close()
		return err
	}
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 8192)
		for {
			n, err := pipeReader.Read(buf)
			if n > 0 {
				sink(append([]byte(nil), buf[:n]...))
			}
			if err != nil {
				return
			}
		}
	}()
	err := cmd.Wait()
	pipeWriter.Close()
	<-readDone
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("timed out after %s", timeout)
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return &ExitError{Code: exit.ExitCode()}
	}
	return err
}
