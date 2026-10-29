package herdr

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/creack/pty"
)

const (
	lingerAfter   = 45 * time.Second
	subscriberBuf = 256
)

// AttachManager shares one `herdr terminal attach` PTY per terminal across every viewer.
// Never pass --takeover: stealing herdr's attach slot is the user's call alone.
type AttachManager struct {
	NewCommand func(terminalID string) *exec.Cmd

	mu       sync.Mutex
	attaches map[string]*attach
}

type attach struct {
	termID string
	cmd    *exec.Cmd
	pty    *os.File
	early  chan []byte // the attach's first chunks, while watched: herdr prints its refusal into the PTY
	watch  atomic.Bool
	exited chan struct{}

	mu     sync.Mutex
	subs   map[chan []byte]bool
	dead   bool
	linger *time.Timer
}

// RefusedError is herdr declining the attach ("already has an attached client; retry with --takeover").
type RefusedError struct{ Msg string }

func (e *RefusedError) Error() string { return e.Msg }

// Refused reports whether err is herdr declining an attach because another client holds the terminal.
func Refused(err error) bool {
	var r *RefusedError
	return errors.As(err, &r) && strings.Contains(r.Msg, "already has an attached client")
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07]*\x07|\x1b[()][A-Z0-9]`)

// refusal finds herdr's verdict in the attach's first output, returned without escape sequences:
// a line "herdr: server shut down: terminal attach failed: …", or any "attach failed".
func refusal(first []byte) (string, bool) {
	text := ansiRe.ReplaceAllString(string(first), "")
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if strings.Contains(line, "attach failed") {
			if i := strings.Index(line, "terminal attach failed: "); i >= 0 {
				line = line[i+len("terminal attach failed: "):]
			}
			return line, true
		}
	}
	return "", false
}

// firstWait is how long a fresh attach that prints nothing is watched for herdr's refusal.
const firstWait = 1500 * time.Millisecond

// watchStart reads the attach's first output for herdr's verdict: a refusal comes in a chunk
// or two and the process exits right after; a real attach paints the screen and stays.
func (a *attach) watchStart() error {
	defer a.watch.Store(false)
	var acc []byte
	deadline := time.After(firstWait)
	var settle <-chan time.Time
	for {
		select {
		case chunk := <-a.early:
			acc = append(acc, chunk...)
			if msg, refused := refusal(acc); refused {
				select {
				case <-a.exited:
				case <-time.After(2 * time.Second):
				}
				return &RefusedError{Msg: msg}
			}
			if len(acc) > 512 {
				return nil
			}
			if settle == nil {
				settle = time.After(300 * time.Millisecond)
			}
		case <-settle:
			return nil
		case <-a.exited:
			if msg, refused := refusal(acc); refused {
				return &RefusedError{Msg: msg}
			}
			return fmt.Errorf("attach ended before it began")
		case <-deadline:
			return nil
		}
	}
}

func NewAttachManager(newCommand func(terminalID string) *exec.Cmd) *AttachManager {
	return &AttachManager{NewCommand: newCommand, attaches: map[string]*attach{}}
}

// LocalAttachCommand builds the attach command for the herdr on this machine.
func LocalAttachCommand(bin, socket string) func(string) *exec.Cmd {
	return func(terminalID string) *exec.Cmd {
		cmd := exec.Command(bin, "terminal", "attach", terminalID)
		cmd.Env = append(os.Environ(),
			"HERDR_SOCKET="+socket,
			"HERDR_SOCKET_PATH="+socket,
			"TERM=xterm-256color",
		)
		return cmd
	}
}

func validSize(cols, rows int) bool {
	return cols >= 20 && rows >= 5 && cols <= 500 && rows <= 300
}

// Subscribe attaches (or joins the existing attach of) a terminal; the caller must call cancel when done.
// A joiner is caught up by a repaint, never a byte tail: TUIs draw with cursor moves relative to where it was.
func (m *AttachManager) Subscribe(termID string, cols, rows int) (ch chan []byte, cancel func(), err error) {
	m.mu.Lock()
	a := m.attaches[termID]
	if a != nil {
		a.mu.Lock()
		dead := a.dead
		a.mu.Unlock()
		if dead {
			a = nil
		}
	}
	joined := a != nil
	if a == nil {
		a, err = m.start(termID, cols, rows)
		if err != nil {
			m.mu.Unlock()
			return nil, nil, err
		}
		m.attaches[termID] = a
	}
	m.mu.Unlock()

	ch = make(chan []byte, subscriberBuf)
	a.mu.Lock()
	if a.dead { // it died between the lookup and here: start over
		a.mu.Unlock()
		return m.Subscribe(termID, cols, rows)
	}
	if a.linger != nil {
		a.linger.Stop()
		a.linger = nil
	}
	a.subs[ch] = true
	a.mu.Unlock()

	if !joined {
		if err := a.watchStart(); err != nil {
			m.kill(a)
			return nil, nil, err
		}
	}
	if joined && validSize(cols, rows) {
		go a.repaint(cols, rows)
	}

	cancel = func() {
		a.mu.Lock()
		delete(a.subs, ch)
		empty := len(a.subs) == 0 && !a.dead
		if empty && a.linger == nil {
			a.linger = time.AfterFunc(lingerAfter, func() { m.killIfIdle(a) })
		}
		a.mu.Unlock()
	}
	return ch, cancel, nil
}

// repaint has the pane redraw for a joining viewer: a row smaller, then the size asked for.
// Back to back, ssh and the app see only the final (unchanged) size and draw nothing.
func (a *attach) repaint(cols, rows int) {
	_ = pty.Setsize(a.pty, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows - 1)})
	time.Sleep(250 * time.Millisecond)
	a.mu.Lock()
	dead := a.dead
	a.mu.Unlock()
	if dead {
		return
	}
	_ = pty.Setsize(a.pty, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

// start must be called with m.mu held.
func (m *AttachManager) start(termID string, cols, rows int) (*attach, error) {
	cmd := m.NewCommand(termID)
	if !validSize(cols, rows) {
		cols, rows = 80, 24
	}
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}
	a := &attach{termID: termID, cmd: cmd, pty: f, subs: map[chan []byte]bool{}, early: make(chan []byte, 64), exited: make(chan struct{})}
	a.watch.Store(true)
	go a.readLoop(m)
	return a, nil
}

func (a *attach) readLoop(m *AttachManager) {
	defer close(a.exited)
	buf := make([]byte, 32*1024)
	var queries queryScanner
	for {
		n, err := a.pty.Read(buf)
		if n > 0 {
			data := append([]byte(nil), buf[:n]...)
			if hasQuery(data) || len(queries.carry) > 0 {
				var replies []byte
				data, replies = queries.scan(data)
				if len(replies) > 0 {
					_, _ = a.pty.Write(replies)
				}
			}
			if len(data) == 0 {
				continue
			}
			if a.watch.Load() {
				select {
				case a.early <- data:
				default:
				}
			}
			a.mu.Lock()
			for ch := range a.subs {
				select {
				case ch <- data:
				default: // slow viewer: drop; a reconnect repaints
				}
			}
			a.mu.Unlock()
		}
		if err != nil {
			break
		}
	}
	m.kill(a)
}

// killIfIdle ends an attach nobody watches, the test and the kill under one lock so a joiner is not cut off.
func (m *AttachManager) killIfIdle(a *attach) {
	a.mu.Lock()
	if len(a.subs) > 0 || a.dead {
		a.mu.Unlock()
		return
	}
	m.killLocked(a)
}

func (m *AttachManager) kill(a *attach) {
	a.mu.Lock()
	if a.dead {
		a.mu.Unlock()
		return
	}
	m.killLocked(a)
}

// killLocked is kill with a.mu held; it releases it.
func (m *AttachManager) killLocked(a *attach) {
	a.dead = true
	subs := a.subs
	a.subs = map[chan []byte]bool{}
	a.mu.Unlock()

	for ch := range subs {
		close(ch)
	}
	a.pty.Close()
	if a.cmd.Process != nil {
		a.cmd.Process.Kill()
	}
	a.cmd.Wait()

	m.mu.Lock()
	if m.attaches[a.termID] == a {
		delete(m.attaches, a.termID)
	}
	m.mu.Unlock()
}

func (m *AttachManager) get(termID string) (*attach, error) {
	m.mu.Lock()
	a := m.attaches[termID]
	m.mu.Unlock()
	if a == nil {
		return nil, fmt.Errorf("terminal %s is not attached", termID)
	}
	return a, nil
}

// Write types raw bytes into the attached terminal, like a keyboard would.
func (m *AttachManager) Write(termID string, data []byte) error {
	a, err := m.get(termID)
	if err != nil {
		return err
	}
	_, err = a.pty.Write(data)
	return err
}

// Resize changes the attach PTY size; herdr reflows the pane for the smallest attached client.
func (m *AttachManager) Resize(termID string, cols, rows int) error {
	if !validSize(cols, rows) {
		return nil
	}
	a, err := m.get(termID)
	if err != nil {
		return err
	}
	return pty.Setsize(a.pty, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}
