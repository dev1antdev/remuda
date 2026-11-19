// Package sshx drives OpenSSH for remote machines: one ControlMaster per
// machine, with socket forwarding, remote execs and the terminal attach over it.
// remuda implements no SSH itself; BatchMode makes it fail instead of prompting.
package sshx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

type Conn struct {
	Host string

	ctl       string // control socket path
	master    *exec.Cmd
	masterErr *lockedBuffer // what the master said: why it could not connect, then nothing more is kept

	sessions sessionCount
}

// A connection carries a limited number of sessions: OpenSSH's MaxSessions is 10 by default, and past it the next one
// is refused. So what opens one takes a place first, and each kind stops short of the last places: herdr's requests can
// always get one, a short script can get one past the long ones and the terminals. Nine in all, as a script killed at its
// timeout is still counted by sshd until it exits on the machine.
const (
	maxSessions    = 9
	rpcReserve     = 2 // places only herdr's requests reach
	shortReserve   = 1 // and one more that long scripts and terminals leave to short scripts
	maxScripts     = 5
	maxLongScripts = 3
	maxTerminals   = 5
	longRun        = time.Minute // a script given more than this is a long one
)

type sessionKind int

const (
	kindRPC sessionKind = iota
	kindShort
	kindLong
	kindTerminal
)

func scriptKind(timeout time.Duration) sessionKind {
	if timeout > longRun {
		return kindLong
	}
	return kindShort
}

// sessionCount is what is open on a connection, by kind.
type sessionCount struct {
	mu                                   sync.Mutex
	total, rpc, scripts, long, terminals int
	freed                                chan struct{} // closed when a place comes free
}

func (s *sessionCount) add(kind sessionKind, by int) {
	s.total += by
	switch kind {
	case kindRPC:
		s.rpc += by
	case kindShort:
		s.scripts += by
	case kindLong:
		s.scripts += by
		s.long += by
	case kindTerminal:
		s.terminals += by
	}
}

// admit takes a place if the kind may have one now; else it answers what to wait on before asking again.
func (s *sessionCount) admit(kind sessionKind) (bool, <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	others := s.total - min(s.rpc, rpcReserve) // requests sit in their own places first
	room := false
	switch kind {
	case kindRPC:
		room = s.total < maxSessions
	case kindShort:
		room = s.scripts < maxScripts && others < maxSessions-rpcReserve
	case kindLong:
		room = s.long < maxLongScripts && s.scripts < maxScripts && others < maxSessions-rpcReserve-shortReserve
	case kindTerminal:
		room = s.terminals < maxTerminals && others < maxSessions-rpcReserve-shortReserve
	}
	if room {
		s.add(kind, 1)
		return true, nil
	}
	if s.freed == nil {
		s.freed = make(chan struct{})
	}
	return false, s.freed
}

func (s *sessionCount) leave(kind sessionKind) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.add(kind, -1)
	if s.freed != nil {
		close(s.freed)
		s.freed = nil
	}
}

const sessionRefused = "Session open refused"

// SessionRefused reads ssh's words for a machine that will not open one more session, and says it plainly.
func SessionRefused(host, said string) (string, bool) {
	if !strings.Contains(said, sessionRefused) {
		return "", false
	}
	return fmt.Sprintf("%s refused one more ssh session: too many are open on the connection (sshd's MaxSessions)", host), true
}

// enter waits for a place within timeout; it answers how to leave and how much of the timeout is left.
func (c *Conn) enter(kind sessionKind, timeout time.Duration) (leave func(), left time.Duration, err error) {
	began := time.Now()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	busy := fmt.Errorf("ssh %s: no session free within %s: the connection is busy", c.Host, timeout)
	for {
		in, freed := c.sessions.admit(kind)
		if in {
			left = timeout - time.Since(began)
			if left <= 0 && timeout > 0 { // its time went in the queue: it must not start now
				c.sessions.leave(kind)
				return nil, 0, busy
			}
			return func() { c.sessions.leave(kind) }, left, nil
		}
		select {
		case <-freed:
		case <-timer.C:
			return nil, 0, busy
		}
	}
}

// HoldTerminal takes a terminal's place for as long as it stands, waiting at most wait; what it answers gives it back.
func (c *Conn) HoldTerminal(wait time.Duration) (release func(), err error) {
	release, _, err = c.enter(kindTerminal, wait)
	if err != nil {
		return nil, fmt.Errorf("%s has too many ssh sessions open for one more terminal: close one and try again", c.Host)
	}
	return release, nil
}

// masterErrMax is how much of the master's stderr is kept: its first lines say why it could not connect, and over a
// life of days it goes on with a line for every channel the host refuses — a herdr down, a port nothing listens on.
const masterErrMax = 4096

// SocketDir is where the control and forward sockets go when main sets it: the state dir, the person's own.
// Unset, the temp dir, where on a shared box another user's socket could sit at the name and be reused.
var SocketDir string

// socketPathMax is the longest socket path handed to ssh: a unix sockaddr holds 104 bytes,
// and OpenSSH binds a master at the path plus a 17-byte random suffix ("too long for Unix domain socket").
const socketPathMax = 86

// tmpPath is a short, deterministic socket path under SocketDir, or the temp dir when that leaves no room — named by
// the state dir as well there: every instance shares the temp dir, and a scratch one must not ride the real one's master.
func tmpPath(prefix, seed, ext string) string {
	if SocketDir != "" {
		name := prefix + digest(seed) + ext
		if len(SocketDir)+1+len(name) <= socketPathMax {
			return filepath.Join(SocketDir, name)
		}
		seed = SocketDir + "\n" + seed
	}
	return filepath.Join(os.TempDir(), prefix+digest(seed)+ext)
}

func digest(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:8])
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

// session is a command that rides the master and nothing else. With the master gone, ssh would "fall back to
// connecting normally" (ssh_config, ControlMaster): a connection of its own for every script and every RPC, and an
// nc bridge that keeps answering that way never lets the machine notice its master died. The proxy that exits at
// once makes that fallback fail instead; a live master never consults it.
func (c *Conn) session(args ...string) *exec.Cmd {
	return c.cmd(append([]string{"-o", "ProxyCommand=false"}, args...)...)
}

// scriptSession is a session for a script on its stdin: no terminal, which a forced one would hold the stdin open
// for, and the stdin kept whatever the host's ssh config says, where this ssh knows the options.
func (c *Conn) scriptSession(line string, keep []string) *exec.Cmd {
	return c.session(append(append([]string{"-T"}, keep...), c.Host, line)...)
}

// stdinWithin is keepStdin within a script's time, before it takes a session: what the wait took is the script's. A
// script holding a NUL is refused: the shell drops the byte, so the script that ran would not be the one written.
func (c *Conn) stdinWithin(command string, timeout time.Duration) ([]string, time.Duration, error) {
	if strings.IndexByte(command, 0) >= 0 {
		return nil, 0, fmt.Errorf("ssh %s: a script holds a NUL byte", c.Host)
	}
	began := time.Now()
	keep, answered := keepStdin(timeout)
	if !answered {
		return nil, 0, fmt.Errorf("ssh %s: no time left for the script once ssh was asked what it knows", c.Host)
	}
	if timeout <= 0 {
		return keep, timeout, nil
	}
	if timeout -= time.Since(began); timeout <= 0 {
		return nil, 0, fmt.Errorf("ssh %s: no time left for the script once ssh was asked what it knows", c.Host)
	}
	return keep, timeout, nil
}

// stdinKept is, by the path of the ssh binary, whether it takes StdinNull and ForkAfterAuthentication (OpenSSH 8.7):
// an older one refuses an option it does not know, and every session with it. It is asked reading no ssh config
// (-F none: a Match exec there runs nothing), for at most stdinProbe, one probe at a time per binary whose answer
// every caller waiting on it shares; an answer is kept, a probe that timed out or did not start is asked again by a
// later caller.
var (
	stdinMu     sync.Mutex
	stdinKept   = map[string]bool{}
	stdinAsking = map[string]*stdinAsk{}
)

type stdinAsk struct {
	done  chan struct{}
	known bool
}

var stdinProbe = 5 * time.Second

// keepStdin is the options, waiting at most wait (0: as long as the probe takes) for a probe under way; answered is
// false when the wait ran out first, the probe going on for the callers after.
func keepStdin(wait time.Duration) (keep []string, answered bool) {
	path, err := exec.LookPath("ssh")
	if err != nil {
		return nil, true
	}
	stdinMu.Lock()
	if known, kept := stdinKept[path]; kept {
		stdinMu.Unlock()
		return stdinOptions(known), true
	}
	ask := stdinAsking[path]
	if ask == nil {
		ask = &stdinAsk{done: make(chan struct{})}
		stdinAsking[path] = ask
		limit := stdinProbe // read here, not in the probe's goroutine
		go func() {
			known, settled := askStdin(path, limit)
			stdinMu.Lock()
			ask.known = known
			if settled {
				stdinKept[path] = known
			}
			delete(stdinAsking, path)
			stdinMu.Unlock()
			close(ask.done)
		}()
	}
	stdinMu.Unlock()
	if wait <= 0 {
		<-ask.done
		return stdinOptions(ask.known), true
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ask.done:
		return stdinOptions(ask.known), true
	case <-timer.C:
		return nil, false
	}
}

func stdinOptions(known bool) []string {
	if !known {
		return nil
	}
	return []string{"-o", "StdinNull=no", "-o", "ForkAfterAuthentication=no"}
}

// askStdin runs the probe: known, and settled unless it timed out or did not start.
func askStdin(path string, limit time.Duration) (known, settled bool) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	err := exec.CommandContext(ctx, path, "-F", "none", "-G", "-o", "StdinNull=no", "-o", "ForkAfterAuthentication=no", "remuda.invalid").Run()
	var refused *exec.ExitError
	return err == nil, err == nil || (errors.As(err, &refused) && ctx.Err() == nil)
}

// Dial starts (or reuses) a ControlMaster to host and verifies it answers.
func Dial(host string) (*Conn, error) {
	c := &Conn{Host: host, ctl: tmpPath("remuda-ssh-", host, ".ctl")}
	if c.listening() {
		return c, nil // a previous remuda run left a live master; reuse it
	}
	os.Remove(c.ctl)

	cmd := c.cmd("-M", "-N", host)
	stderr := &lockedBuffer{max: masterErrMax}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c.master, c.masterErr = cmd, stderr
	exited := make(chan struct{})
	go func() {
		cmd.Wait()
		close(exited)
	}()

	deadline := time.Now().Add(15 * time.Second)
wait:
	for time.Now().Before(deadline) {
		if c.listening() {
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

// listening is Alive asked only once the control socket is there: a master puts it in place once it has signed in,
// and until then every ask is an ssh started for nothing — five a second for as long as a host does not answer.
func (c *Conn) listening() bool {
	if _, err := os.Stat(c.ctl); err != nil {
		return false
	}
	return c.Alive()
}

// RemoteCommand is the command line a script travels as: through sh, since the remote login shell may be anything —
// zsh, fish, or on a Windows machine Git Bash, which Windows' OpenSSH hands the line to as it is (its quotes kept).
func RemoteCommand(command string) string {
	return "sh -c " + script.Quote(command)
}

// scriptEnd closes every script sent on stdin: one that arrives without it was cut, or never sent (a host whose ssh
// config sets StdinNull or ForkAfterAuthentication), and is not run.
const scriptEnd = "#remuda:end"

// loader is the command line that reads a script with read, then runs it, its stdin what is left.
func loader(read string) string {
	return RemoteCommand(`s=$(` + read + `); case "$s" in *"` + scriptEnd + `") eval "$s" ;; ` +
		`*) echo "remuda: the script did not reach the machine whole on the stdin of its ssh session (StdinNull or ForkAfterAuthentication in its ssh config?)" >&2; exit 125 ;; esac`)
}

// readScript is the command line of a script sent on stdin: read whole, then run with stdin at its end, as a script
// on the command line finds it.
var readScript = loader("cat")

// Remote is how a script travels to a machine: on the session's stdin, under a line every login shell reads as it is
// written (no backslash, no line break, no quote but its own pair), since sshd hands the line to the account's shell
// first — fish reads \ and \' inside single quotes, tcsh refuses a line break inside them — and Dropbear drops the
// whole connection over a command past 9000 bytes.
func Remote(command string) (line string, stdin io.Reader) {
	return readScript, strings.NewReader(command + "\n" + scriptEnd + "\n")
}

// RemoteWithInput is how a script travels with data for its stdin: the script first, read by dd a byte at a time to
// its length, which the line says, so what follows it stays the script's stdin; with none it is Remote.
func RemoteWithInput(command string, input io.Reader) (line string, stdin io.Reader) {
	if input == nil {
		return Remote(command)
	}
	whole := command + "\n" + scriptEnd + "\n"
	return loader(`dd bs=1 count=` + strconv.Itoa(len(whole)) + ` 2>/dev/null`), io.MultiReader(strings.NewReader(whole), input)
}

// CommandConn starts the command and exposes its stdin and stdout as a net.Conn that the deadline ends: the way an
// exec over ssh (nc -U, remuda bridge) carries herdr's RPC.
func CommandConn(cmd *exec.Cmd, timeout time.Duration) (net.Conn, error) {
	return newCmdConn(cmd, timeout)
}

// run executes a remote command over the master with a deadline, the script on the session's stdin (Remote), the stdin
// given after it (RemoteWithInput).
func (c *Conn) run(command string, timeout time.Duration, stdin io.Reader) ([]byte, error) {
	keep, timeout, err := c.stdinWithin(command, timeout)
	if err != nil {
		return nil, err
	}
	leave, timeout, err := c.enter(scriptKind(timeout), timeout)
	if err != nil {
		return nil, err
	}
	defer leave()
	line, stdin := RemoteWithInput(command, stdin)
	cmd := c.scriptSession(line, keep)
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
				said := strings.TrimSpace(errb.String())
				if words, refused := SessionRefused(c.Host, said); refused {
					said = words
				}
				return out.Bytes(), &shell.ExitError{Code: ee.ExitCode(), Stderr: said}
			}
			return out.Bytes(), fmt.Errorf("ssh %s: %w", c.Host, err)
		}
		return out.Bytes(), nil
	case <-time.After(timeout):
		cmd.Process.Kill()
		<-done
		return nil, fmt.Errorf("ssh %s: %q timed out", c.Host, command)
	}
}

// Stream runs a remote command, handing sink its output (stdout and stderr) as it comes; the exit status is the error.
func (c *Conn) Stream(command string, timeout time.Duration, sink func([]byte)) error {
	keep, timeout, err := c.stdinWithin(command, timeout)
	if err != nil {
		return err
	}
	leave, timeout, err := c.enter(scriptKind(timeout), timeout)
	if err != nil {
		return err
	}
	defer leave()
	line, stdin := Remote("exec 2>&1; " + command)
	cmd := c.scriptSession(line, keep)
	cmd.Stdin = stdin
	pr, pw := io.Pipe()
	stderr := &lockedBuffer{max: masterErrMax}
	cmd.Stdout, cmd.Stderr = pw, io.MultiWriter(pw, stderr)
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
				exit := &shell.ExitError{Code: ee.ExitCode()}
				if words, refused := SessionRefused(c.Host, stderr.String()); refused {
					exit.Stderr = words
				}
				return exit
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

// Run runs a script on the host and returns its stdout.
func (c *Conn) Run(command string, timeout time.Duration) ([]byte, error) {
	return c.run(command, timeout, nil)
}

// Put writes data to a remote file, creating its directory, through cat over the master.
func (c *Conn) Put(path string, data []byte, timeout time.Duration) error {
	_, err := c.run(PutScript(path), timeout, bytes.NewReader(data))
	return err
}

// PutScript writes stdin to the file, its folder made first. A Windows path (C:\…) is cut at its own separator and
// handed over with forward slashes, which mkdir -p walks one folder at a time under Git Bash.
func PutScript(path string) string {
	style := rpath.For(path)
	return "mkdir -p " + script.Quote(style.Slash(style.Dir(path))) + " && cat > " + script.Quote(style.Slash(path))
}

// RunWithInput runs a script on the host with data on its stdin: the way for a secret that must not travel on a command line.
func (c *Conn) RunWithInput(command string, stdin []byte, timeout time.Duration) ([]byte, error) {
	return c.run(command, timeout, bytes.NewReader(stdin))
}

// ForwardBack opens 127.0.0.1:remotePort on the host, or with 0 a port the host picks, leading to 127.0.0.1:localPort
// here, for as long as the master lives, and says which port it is; a forward of that port left on a reused master is
// cancelled first, so the new one is not refused as taken.
func (c *Conn) ForwardBack(remotePort, localPort int) (int, error) {
	c.CancelForwardBack(remotePort, localPort)
	cmd := c.forwardBackCmd(remotePort, localPort)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ssh reverse forward: %s", strings.TrimSpace(errb.String()))
	}
	if remotePort != 0 {
		return remotePort, nil
	}
	port, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("ssh reverse forward: no port in the answer %q", out)
	}
	return port, nil
}

// CancelForwardBack lets go of the forward ForwardBack was asked for with these ports, if the master holds one.
func (c *Conn) CancelForwardBack(remotePort, localPort int) {
	c.cmd("-O", "cancel", "-R", forwardBackSpec(remotePort, localPort), c.Host).Run()
}

func (c *Conn) forwardBackCmd(remotePort, localPort int) *exec.Cmd {
	return c.cmd("-O", "forward", "-R", forwardBackSpec(remotePort, localPort), c.Host)
}

// forwardBackSpec is the -R argument: the host's loopback port leading to this side's loopback port.
func forwardBackSpec(remotePort, localPort int) string {
	return fmt.Sprintf("127.0.0.1:%d:127.0.0.1:%d", remotePort, localPort)
}

// ForwardSocket forwards a local unix socket to a remote one over the master and returns the local path. A forward
// of that socket the master already holds — from an earlier connect over it, or an earlier remuda whose master lives
// on — is cancelled first: asked for the same forward again, the master answers "found existing forwarding" and
// binds nothing, so unlinking the old listener's file would leave no socket at all.
func (c *Conn) ForwardSocket(remoteSock string) (string, error) {
	local := tmpPath("remuda-fw-", c.Host+"\x00"+remoteSock, ".sock")
	spec := local + ":" + remoteSock
	c.cmd("-O", "cancel", "-L", spec, c.Host).Run()
	os.Remove(local)
	cmd := c.cmd("-O", "forward", "-L", spec, c.Host)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ssh forward: %s", strings.TrimSpace(errb.String()))
	}
	return local, nil
}

// ForwardTCP forwards a fresh loopback port to 127.0.0.1:remotePort on the host for as long as the master lives.
func (c *Conn) ForwardTCP(remotePort int) (string, error) {
	return c.ForwardTCPTo("127.0.0.1", remotePort)
}

// ForwardTCPTo forwards a fresh loopback port to host:remotePort as the machine dials it — ::1 for a server listening
// on the IPv6 loopback alone, as a dev server on a Mac often does — for as long as the master lives.
func (c *Conn) ForwardTCPTo(host string, remotePort int) (string, error) {
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	local := l.Addr().String()
	l.Close()
	cmd := c.cmd("-O", "forward", "-L", fmt.Sprintf("%s:%s:%d", local, host, remotePort), c.Host)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ssh forward: %s", strings.TrimSpace(errb.String()))
	}
	return local, nil
}

// DialExec runs a remote command over the master and exposes its stdin/stdout as a net.Conn:
// the RPC transport for an sshd that refuses unix-socket forwarding (`nc -U <socket>` bridges to herdr).
func (c *Conn) DialExec(command string, timeout time.Duration) (net.Conn, error) {
	leave, left, err := c.enter(kindRPC, timeout)
	if err != nil {
		return nil, err
	}
	conn, err := newCmdConn(c.session(c.Host, command), left)
	if err != nil {
		leave()
		return nil, err
	}
	cc := conn.(*cmdConn)
	cc.leave, cc.host = leave, c.Host
	return cc, nil
}

// newCmdConn starts the command and wraps its stdin and stdout as a net.Conn that the deadline ends.
func newCmdConn(cmd *exec.Cmd, timeout time.Duration) (net.Conn, error) {
	stderr := &lockedBuffer{}
	cmd.Stderr = stderr
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
	cc := &cmdConn{cmd: cmd, stdin: stdin, stdout: stdout, stderr: stderr}
	cc.SetDeadline(time.Now().Add(timeout))
	return cc, nil
}

type cmdConn struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	stderr   *lockedBuffer
	mu       sync.Mutex
	timer    *time.Timer
	closed   bool
	timedOut bool
	deadline time.Time // nothing is written past it, even before the timer has ended the command
	leave    func()    // gives its place back, when it holds one
	host     string    // the machine, on a connection that rides a master
}

// lockedBuffer holds the command's stderr: exec copies into it on a goroutine of its own while a failing Read or
// Write reads it for its words.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int // the most it keeps, the first bytes; 0 keeps everything
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.max > 0 && b.buf.Len()+len(p) > b.max {
		b.buf.Write(p[:max(b.max-b.buf.Len(), 0)])
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
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
	msg := c.saidBy()
	if words, refused := SessionRefused(c.host, msg); refused {
		return n, errors.New(words) // no EOF: a caller must not take it for herdr hanging up
	}
	if msg != "" {
		return n, fmt.Errorf("%w (%s)", err, msg)
	}
	return n, err
}

// saidBy is what ssh wrote, given a moment on a session that failed: its reason can trail the end of its output and
// what else it says.
func (c *cmdConn) saidBy() string {
	for range 10 {
		msg := strings.TrimSpace(c.stderr.String())
		c.mu.Lock()
		closed := c.closed
		c.mu.Unlock()
		if _, refused := SessionRefused(c.host, msg); refused || closed || c.host == "" {
			return msg
		}
		time.Sleep(20 * time.Millisecond)
	}
	return strings.TrimSpace(c.stderr.String())
}

func (c *cmdConn) Read(b []byte) (int, error) { return c.wrap(c.stdout.Read(b)) }
func (c *cmdConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	late := !c.deadline.IsZero() && !time.Now().Before(c.deadline)
	c.mu.Unlock()
	if late {
		return 0, os.ErrDeadlineExceeded
	}
	return c.wrap(c.stdin.Write(b))
}

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
	if c.leave != nil {
		c.leave()
	}
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
	c.deadline = t
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
// The machine's herdr socket travels as HERDR_SOCKET_PATH, as the local attach's does, so a herdr listening off its
// default path is reached; through sh, since the remote login shell may be anything.
func (c *Conn) AttachCmd(herdrBin, socket, terminalID string) *exec.Cmd {
	remote := "exec " + script.Quote(herdrBin) + " terminal attach " + script.Quote(terminalID)
	if socket != "" {
		remote = "HERDR_SOCKET=" + script.Quote(socket) + " HERDR_SOCKET_PATH=" + script.Quote(socket) + " " + remote
	}
	cmd := c.session("-tt", c.Host, "sh -c "+script.Quote(remote))
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
