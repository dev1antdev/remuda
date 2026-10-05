package herdr

import (
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
)

const (
	replayCap     = 256 * 1024
	lingerAfter   = 45 * time.Second // keep an attach alive briefly after the last viewer leaves
	subscriberBuf = 256
)

// AttachManager shares one `herdr terminal attach <terminal_id>` PTY per
// terminal across every browser viewer. Never pass --takeover: stealing
// herdr's own attach slot is the user's call, not the bridge's.
// NewCommand builds the attach process — a local herdr exec, or ssh for a
// remote machine.
type AttachManager struct {
	NewCommand func(terminalID string) *exec.Cmd

	mu       sync.Mutex
	attaches map[string]*attach
}

type attach struct {
	termID string
	cmd    *exec.Cmd
	pty    *os.File

	mu     sync.Mutex
	subs   map[chan []byte]bool
	replay []byte
	dead   bool
	linger *time.Timer
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

// Subscribe attaches (or joins the existing attach of) a terminal. The caller
// receives the replay tail, a live channel, and must call cancel when done.
func (m *AttachManager) Subscribe(termID string, cols, rows int) (replay []byte, ch chan []byte, cancel func(), err error) {
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
	if a == nil {
		a, err = m.start(termID, cols, rows)
		if err != nil {
			m.mu.Unlock()
			return nil, nil, nil, err
		}
		m.attaches[termID] = a
	}
	m.mu.Unlock()

	ch = make(chan []byte, subscriberBuf)
	a.mu.Lock()
	if a.linger != nil {
		a.linger.Stop()
		a.linger = nil
	}
	a.subs[ch] = true
	replay = append([]byte(nil), a.replay...)
	a.mu.Unlock()

	cancel = func() {
		a.mu.Lock()
		delete(a.subs, ch)
		empty := len(a.subs) == 0 && !a.dead
		if empty && a.linger == nil {
			a.linger = time.AfterFunc(lingerAfter, func() {
				a.mu.Lock()
				idle := len(a.subs) == 0 && !a.dead
				a.mu.Unlock()
				if idle {
					m.kill(a)
				}
			})
		}
		a.mu.Unlock()
	}
	return replay, ch, cancel, nil
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
	a := &attach{termID: termID, cmd: cmd, pty: f, subs: map[chan []byte]bool{}}
	go a.readLoop(m)
	return a, nil
}

func (a *attach) readLoop(m *AttachManager) {
	buf := make([]byte, 32*1024)
	for {
		n, err := a.pty.Read(buf)
		if n > 0 {
			data := append([]byte(nil), buf[:n]...)
			a.mu.Lock()
			a.replay = append(a.replay, data...)
			// trim in big steps: slicing the front off on every chunk would
			// make each append reallocate and copy the whole tail
			if len(a.replay) > 2*replayCap {
				a.replay = append(a.replay[:0:0], a.replay[len(a.replay)-replayCap:]...)
			}
			for ch := range a.subs {
				select {
				case ch <- data:
				default: // slow viewer: drop; it resyncs from replay on reconnect
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

func (m *AttachManager) kill(a *attach) {
	a.mu.Lock()
	if a.dead {
		a.mu.Unlock()
		return
	}
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
		return nil, os.ErrNotExist
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

// Resize changes the attach PTY size; herdr reflows the pane for the smallest
// attached client, exactly like a terminal window resize.
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
