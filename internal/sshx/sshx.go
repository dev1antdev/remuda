// Package sshx drives OpenSSH for remote machines: one ControlMaster per
// machine, with socket forwarding, remote execs and the terminal attach over it.
// remuda implements no SSH itself; BatchMode makes it fail instead of prompting.
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

// VersionedBin is the quoted path of a machine's own remuda and the shell guard that it is this very version
// (an older one would not know the subcommand, or would start a server instead).
func VersionedBin(home, version string) (bin, guard string) {
	bin = Quote(home + "/.local/bin/remuda")
	guard = "[ -x " + bin + " ] && [ \"$(" + bin + " -version 2>/dev/null)\" = " + Quote("remuda "+version) + " ]"
	return bin, guard
}

// Quote wraps s for a POSIX shell.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// SocketDir is where the control and forward sockets go when main sets it: the state dir, the person's own.
// Unset, the temp dir, where on a shared box another user's socket could sit at the name and be reused.
var SocketDir string

// socketPathMax is the longest socket path handed to ssh: a unix sockaddr holds 104 bytes,
// and OpenSSH binds a master at the path plus a 17-byte random suffix ("too long for Unix domain socket").
const socketPathMax = 86

// tmpPath is a short, deterministic socket path under SocketDir, or the temp dir when that leaves no room.
func tmpPath(prefix, seed, ext string) string {
	sum := sha256.Sum256([]byte(seed))
	name := prefix + hex.EncodeToString(sum[:8]) + ext
	if SocketDir != "" && len(SocketDir)+1+len(name) <= socketPathMax {
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
	c := &Conn{Host: host, ctl: tmpPath("remuda-ssh-", host, ".ctl")}
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
	go func() {
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

// run executes a remote command over the master with a deadline, stdin streamed to it when given.
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

// Stream runs a remote command, handing sink its output (stdout and stderr) as it comes; the exit status is the error.
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

// ExecBytes is Exec for binary-safe stdout.
func (c *Conn) ExecBytes(script string, timeout time.Duration) ([]byte, error) {
	return c.run(script, timeout, nil)
}

// Put writes data to a remote file, creating its directory, through cat over the master.
func (c *Conn) Put(path string, data []byte, timeout time.Duration) error {
	_, err := c.run("mkdir -p "+Quote(filepath.Dir(path))+" && cat > "+Quote(path), timeout, bytes.NewReader(data))
	return err
}

// ForwardSocket forwards a local unix socket to a remote one over the master and returns the local path.
func (c *Conn) ForwardSocket(remoteSock string) (string, error) {
	local := tmpPath("remuda-fw-", c.Host+"\x00"+remoteSock, ".sock")
	os.Remove(local)
	cmd := c.cmd("-O", "forward", "-L", local+":"+remoteSock, c.Host)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ssh forward: %s", strings.TrimSpace(errb.String()))
	}
	return local, nil
}

// ForwardTCP forwards a fresh loopback port to 127.0.0.1:remotePort on the host for as long as the master lives.
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

// DialExec runs a remote command over the master and exposes its stdin/stdout as a net.Conn:
// the RPC transport for an sshd that refuses unix-socket forwarding (`nc -U <socket>` bridges to herdr).
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

// wrap turns a failure after the deadline into os.ErrDeadlineExceeded, else adds what ssh said.
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

// SetDeadline kills the remote command when the deadline passes, which fails the pending Read.
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

// AttachCmd builds the remote `herdr terminal attach` to run under a local PTY; ssh -tt carries window-size changes.
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
