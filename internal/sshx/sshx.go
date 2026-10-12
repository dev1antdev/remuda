// Package sshx drives OpenSSH for remote machines: one ControlMaster per
// machine, and everything else — RPC socket forwarding, remote execs, the
// terminal attach — multiplexed over it. remuda never implements SSH
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

// VersionedBin is the shell test that a machine's own remuda at
// ~/.local/bin is this very version (an older one would not know the
// subcommand — or would start a server instead), and the quoted path to
// run it: `if <guard>; then <bin> …`.
func VersionedBin(home, version string) (bin, guard string) {
	bin = Quote(home + "/.local/bin/remuda")
	guard = "[ -x " + bin + " ] && [ \"$(" + bin + " -version 2>/dev/null)\" = " + Quote("remuda "+version) + " ]"
	return bin, guard
}

// Quote wraps s for a POSIX shell.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// SocketDir is where the control and forward sockets go when main sets
// it: the state dir, 0700 and the person's own. Unset, the temp dir —
// which on a shared Linux box is everyone's, where a socket of another
// user's could sit at the name and be reused as the master.
var SocketDir string

// tmpPath is a short, deterministic socket path — unix sockaddr paths
// have ~100 bytes to work with, so a long state dir falls back to the
// temp dir.
func tmpPath(prefix, seed string) string {
	sum := sha256.Sum256([]byte(seed))
	name := prefix + hex.EncodeToString(sum[:8])
	if SocketDir != "" && len(SocketDir)+len(name)+6 < 100 {
		return filepath.Join(SocketDir, name)
	}
	return filepath.Join(os.TempDir(), name)
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
	c := &Conn{Host: host, ctl: tmpPath("remuda-ssh-", host) + ".ctl"}
	if c.Alive() {
		return c, nil // a previous remuda run left a live master; reuse it
	}
	os.Remove(c.ctl)

	cmd := c.cmd("-M", "-N", host)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c.master = cmd
	exited := make(chan struct{})
	go func() { // reap whenever it exits
		cmd.Wait()
		close(exited)
	}()

	deadline := time.Now().Add(15 * time.Second)
wait:
	for time.Now().Before(deadline) {
		if c.Alive() {
			return c, nil
		}
		select {
		case <-exited: // master already died (auth failure etc.)
			break wait
		case <-time.After(200 * time.Millisecond):
		}
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

// run executes a remote command over the master with a deadline; stdin, when
// given, is streamed to it.
func (c *Conn) run(script string, timeout time.Duration, stdin io.Reader) ([]byte, error) {
	// every script here is POSIX sh; the remote login shell may be anything
	cmd := c.cmd(c.Host, "sh -c "+Quote(script))
	var out, errb bytes.Buffer
	cmd.Stdin = stdin
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

// Stream runs a command on the remote host and hands its output (stdout and
// stderr together, as it comes) to sink; the exit status is the error.
func (c *Conn) Stream(script string, timeout time.Duration, sink func([]byte)) error {
	cmd := c.cmd(c.Host, "sh -c "+Quote("exec 2>&1; "+script))
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		pw.Close()
		return err
	}
	done := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		pw.Close()
		done <- err
	}()
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 8192)
		for {
			n, err := pr.Read(buf)
			if n > 0 {
				sink(append([]byte(nil), buf[:n]...))
			}
			if err != nil {
				return
			}
		}
	}()
	select {
	case err := <-done:
		<-readDone
		if err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				return &ExitError{Code: ee.ExitCode()}
			}
			return fmt.Errorf("ssh %s: %w", c.Host, err)
		}
		return nil
	case <-time.After(timeout):
		cmd.Process.Kill()
		<-done
		<-readDone
		return fmt.Errorf("ssh %s: timed out after %s", c.Host, timeout)
	}
}

// Exec runs a command on the remote host and returns its stdout.
func (c *Conn) Exec(script string, timeout time.Duration) (string, error) {
	out, err := c.run(script, timeout, nil)
	return string(out), err
}

// ExecBytes is Exec for binary-safe stdout (transcript tails, file reads).
func (c *Conn) ExecBytes(script string, timeout time.Duration) ([]byte, error) {
	return c.run(script, timeout, nil)
}

// Put writes data to a remote file (creating its directory) by streaming it
// into `cat` over the master.
func (c *Conn) Put(path string, data []byte, timeout time.Duration) error {
	_, err := c.run("mkdir -p "+Quote(filepath.Dir(path))+" && cat > "+Quote(path), timeout, bytes.NewReader(data))
	return err
}

// ForwardSocket forwards a local unix socket to a remote one over the master
// and returns the local path.
func (c *Conn) ForwardSocket(remoteSock string) (string, error) {
	local := tmpPath("remuda-fw-", c.Host+"\x00"+remoteSock) + ".sock"
	os.Remove(local)
	cmd := c.cmd("-O", "forward", "-L", local+":"+remoteSock, c.Host)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ssh forward: %s", strings.TrimSpace(errb.String()))
	}
	return local, nil
}

// ForwardTCP forwards a fresh local loopback port to 127.0.0.1:remotePort on
// the host over the master and returns the local address. The forward lives
// as long as the master does.
func (c *Conn) ForwardTCP(remotePort int) (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	local := l.Addr().String()
	l.Close()
	cmd := c.cmd("-O", "forward", "-L", fmt.Sprintf("%s:127.0.0.1:%d", local, remotePort), c.Host)
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
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
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
	cc := &cmdConn{cmd: cmd, stdin: stdin, stdout: stdout, stderr: &stderr}
	cc.SetDeadline(time.Now().Add(timeout))
	return cc, nil
}

type cmdConn struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	stderr   *bytes.Buffer
	mu       sync.Mutex
	timer    *time.Timer
	closed   bool
	timedOut bool
}

// a read or write after the deadline fired says so, with what ssh said
func (c *cmdConn) wrap(n int, err error) (int, error) {
	if err == nil {
		return n, nil
	}
	c.mu.Lock()
	timedOut := c.timedOut
	c.mu.Unlock()
	if timedOut {
		return n, os.ErrDeadlineExceeded
	}
	if msg := strings.TrimSpace(c.stderr.String()); msg != "" {
		return n, fmt.Errorf("%w (%s)", err, msg)
	}
	return n, err
}

func (c *cmdConn) Read(b []byte) (int, error)  { return c.wrap(c.stdout.Read(b)) }
func (c *cmdConn) Write(b []byte) (int, error) { return c.wrap(c.stdin.Write(b)) }

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
			c.mu.Lock()
			c.timedOut = true
			c.mu.Unlock()
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
