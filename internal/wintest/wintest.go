//go:build windows

package wintest

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/sshx"
)

// Live skips the test unless it runs on the windows job's prepared Windows.
func Live(t *testing.T) {
	t.Helper()
	if os.Getenv("REMUDA_WINDOWS_HERDR") != "1" {
		t.Skip("REMUDA_WINDOWS_HERDR=1 runs this on a prepared Windows (the CI windows job)")
	}
}

// RemudaExe is the remuda.exe the job installed where `remuda setup` puts it.
func RemudaExe(t *testing.T) string {
	t.Helper()
	exe := os.Getenv("REMUDA_EXE")
	if exe == "" {
		t.Fatal("REMUDA_EXE names the remuda.exe under test")
	}
	return exe
}

// SSH runs scripts on this Windows through its own OpenSSH server, whose default shell is Git Bash: what a cockpit's
// ssh master carries, but without the master, which Windows' ssh client cannot hold.
type SSH struct {
	Config string // the ssh config naming Host, its key and its known hosts
	Host   string
}

// Remote is the job's ssh to this very machine.
func Remote(t *testing.T) SSH {
	t.Helper()
	cfg, host := os.Getenv("REMUDA_SSH_CONFIG"), os.Getenv("REMUDA_SSH_HOST")
	if cfg == "" || host == "" {
		t.Fatal("REMUDA_SSH_CONFIG and REMUDA_SSH_HOST name the job's ssh to this machine")
	}
	return SSH{Config: cfg, Host: host}
}

func (s SSH) command(remote string) *exec.Cmd {
	return exec.Command(`C:\Windows\System32\OpenSSH\ssh.exe`, "-F", s.Config, s.Host, remote)
}

// Run is shell.Shell's Run over the machine's ssh.
func (s SSH) Run(script string, timeout time.Duration) ([]byte, error) {
	return s.RunWithInput(script, nil, timeout)
}

// RunWithInput runs a script with data on its stdin, as a cockpit hands a key over.
func (s SSH) RunWithInput(script string, input []byte, timeout time.Duration) ([]byte, error) {
	cmd := s.command(sshx.RemoteCommand(script))
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return out.Bytes(), &shell.ExitError{Code: exit.ExitCode(), Stderr: strings.TrimSpace(errb.String())}
		}
		return out.Bytes(), err
	case <-time.After(timeout):
		cmd.Process.Kill()
		<-done
		return out.Bytes(), fmt.Errorf("ssh %s: timed out after %s", s.Host, timeout)
	}
}

// Stream is shell.Shell's Stream over the machine's ssh, the output handed over whole at the end.
func (s SSH) Stream(script string, timeout time.Duration, sink func([]byte)) error {
	out, err := s.Run("exec 2>&1; "+script, timeout)
	if len(out) > 0 {
		sink(out)
	}
	return err
}

// Raw runs a command line over the machine's ssh as it is, no sh -c around it.
func (s SSH) Raw(command string) (string, error) {
	out, err := s.command(command).CombinedOutput()
	return string(out), err
}

// Dial is one exec over the machine's ssh as a connection: what sshx.Conn.DialExec does over the master.
func (s SSH) Dial(command string, timeout time.Duration) (net.Conn, error) {
	return sshx.CommandConn(s.command(command), timeout)
}

// Record keeps an answer of this Windows under REMUDA_RECORD_DIR, which the job uploads: the unit tests' fixtures come from there.
func Record(t *testing.T, name, answer string) {
	t.Helper()
	dir := os.Getenv("REMUDA_RECORD_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+`\`+name, []byte(answer), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Text is Run with the output as text, failing the test on an error.
func Text(t *testing.T, sh shell.Shell, script string) string {
	t.Helper()
	out, err := sh.Run(script, time.Minute)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	return string(out)
}
