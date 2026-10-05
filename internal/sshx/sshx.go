// Package sshx drives OpenSSH for remote machines: one ControlMaster per
// machine, and everything else — RPC socket forwarding, remote execs, the
// terminal attach — multiplexed over it. herdr-ui never implements SSH
// itself: the user's own ssh config, keys and agent do the authentication,
// and BatchMode guarantees we fail instead of prompting.
package sshx

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Conn struct {
	Host string

	ctl    string // control socket path
	master *exec.Cmd
}

// ExitError is a remote command that ran and exited non-zero.
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

// Quote wraps s for a POSIX shell.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// tmpPath is a short, deterministic path under the temp dir — unix sockaddr
// paths have ~100 bytes to work with.
func tmpPath(prefix, seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return filepath.Join(os.TempDir(), prefix+hex.EncodeToString(sum[:8]))
}

func (c *Conn) cmd(args ...string) *exec.Cmd {
	base := []string{
		"-S", c.ctl,
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=4",
		"-o", "StreamLocalBindUnlink=yes",
	}
	return exec.Command("ssh", append(base, args...)...)
}

// Dial starts (or reuses) a ControlMaster to host and verifies it answers.
func Dial(host string) (*Conn, error) {
	c := &Conn{Host: host, ctl: tmpPath("herdr-ui-ssh-", host) + ".ctl"}
	if c.Alive() {
		return c, nil // a previous herdr-ui run left a live master; reuse it
	}
	os.Remove(c.ctl)

	cmd := c.cmd("-M", "-N", host)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c.master = cmd
	go cmd.Wait() // reap whenever it exits

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if c.Alive() {
			return c, nil
		}
		if cmd.ProcessState != nil { // master already died (auth failure etc.)
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	cmd.Process.Kill()
	msg := strings.TrimSpace(stderr.String())
	if msg == "" {
		msg = "ssh master did not come up"
	}
	return nil, fmt.Errorf("ssh %s: %s", host, msg)
}

// Alive reports whether the master still answers.
func (c *Conn) Alive() bool { return c.cmd("-O", "check", c.Host).Run() == nil }

// run executes a remote command over the master with a deadline.
func (c *Conn) run(script string, timeout time.Duration) ([]byte, error) {
	cmd := c.cmd(c.Host, script)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				return out.Bytes(), &ExitError{Code: ee.ExitCode(), Stderr: strings.TrimSpace(errb.String())}
			}
			return out.Bytes(), fmt.Errorf("ssh %s: %w", c.Host, err)
		}
		return out.Bytes(), nil
	case <-time.After(timeout):
		cmd.Process.Kill()
		<-done
		return nil, fmt.Errorf("ssh %s: %q timed out", c.Host, script)
	}
}

// Exec runs a command on the remote host and returns its stdout.
func (c *Conn) Exec(script string, timeout time.Duration) (string, error) {
	out, err := c.run(script, timeout)
	return string(out), err
}

// ExecBytes is Exec for binary-safe stdout (transcript tails, file reads).
func (c *Conn) ExecBytes(script string, timeout time.Duration) ([]byte, error) {
	return c.run(script, timeout)
}

// Put writes data to a remote file (creating its directory) by streaming it
// into `cat` over the master.
func (c *Conn) Put(path string, data []byte, timeout time.Duration) error {
	script := "mkdir -p " + Quote(filepath.Dir(path)) + " && cat > " + Quote(path)
	cmd := c.cmd(c.Host, script)
	cmd.Stdin = bytes.NewReader(data)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("ssh %s: %s", c.Host, strings.TrimSpace(errb.String()))
		}
		return nil
	case <-time.After(timeout):
		cmd.Process.Kill()
		<-done
		return fmt.Errorf("ssh %s: upload of %s timed out", c.Host, path)
	}
}

// ForwardSocket forwards a local unix socket to a remote one over the master
// and returns the local path.
func (c *Conn) ForwardSocket(remoteSock string) (string, error) {
	local := tmpPath("herdr-ui-fw-", c.Host+"\x00"+remoteSock) + ".sock"
	os.Remove(local)
	cmd := c.cmd("-O", "forward", "-L", local+":"+remoteSock, c.Host)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ssh forward: %s", strings.TrimSpace(errb.String()))
	}
	return local, nil
}

// DialExec runs a remote command over the master and exposes its stdin/stdout
// as a net.Conn — the RPC transport for sshds that refuse unix-socket
// forwarding (`nc -U <socket>` on the remote side bridges to herdr).
func (c *Conn) DialExec(script string, timeout time.Duration) (net.Conn, error) {
	cmd := c.cmd(c.Host, script)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	cc := &cmdConn{cmd: cmd, stdin: stdin, stdout: stdout}
	cc.SetDeadline(time.Now().Add(timeout))
	return cc, nil
}

type cmdConn struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	mu     sync.Mutex
	timer  *time.Timer
	closed bool
}

func (c *cmdConn) Read(b []byte) (int, error)  { return c.stdout.Read(b) }
func (c *cmdConn) Write(b []byte) (int, error) { return c.stdin.Write(b) }

func (c *cmdConn) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	if c.timer != nil {
		c.timer.Stop()
	}
	c.mu.Unlock()
	c.stdin.Close()
	c.stdout.Close()
	if c.cmd.Process != nil {
		c.cmd.Process.Kill()
	}
	c.cmd.Wait()
	return nil
}

// A deadline kills the remote command, which fails the pending Read.
func (c *cmdConn) SetDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	if !t.IsZero() && !c.closed {
		c.timer = time.AfterFunc(time.Until(t), func() {
			if c.cmd.Process != nil {
				c.cmd.Process.Kill()
			}
		})
	}
	return nil
}
func (c *cmdConn) SetReadDeadline(t time.Time) error  { return c.SetDeadline(t) }
func (c *cmdConn) SetWriteDeadline(t time.Time) error { return c.SetDeadline(t) }
func (c *cmdConn) LocalAddr() net.Addr                { return sshAddr("ssh-exec") }
func (c *cmdConn) RemoteAddr() net.Addr               { return sshAddr(c.cmd.Args[len(c.cmd.Args)-2]) }

type sshAddr string

func (a sshAddr) Network() string { return "ssh" }
func (a sshAddr) String() string  { return string(a) }

// AttachCmd builds the remote `herdr terminal attach` command to run under a
// local PTY. ssh propagates window-size changes to the remote tty.
func (c *Conn) AttachCmd(herdrBin, terminalID string) *exec.Cmd {
	cmd := c.cmd("-tt", c.Host, Quote(herdrBin)+" terminal attach "+Quote(terminalID))
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	return cmd
}

// Close stops the master (and with it every forwarded socket and attach).
func (c *Conn) Close() {
	c.cmd("-O", "exit", c.Host).Run()
	if c.master != nil && c.master.Process != nil {
		c.master.Process.Kill()
	}
	os.Remove(c.ctl)
}
