package machines

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/sshx"
	"github.com/dev1antdev/remuda/internal/transcript"
)

type Machine struct {
	Cfg   Config // as registered: herdr's list, machines.json, or this host's name
	local bool
	label atomic.Value

	mu          sync.Mutex
	state       string
	errMsg      string
	lastAttempt time.Time
	lastAlive   time.Time // last `ssh -O check`, so transport errors don't spawn one each
	removed     bool      // taken out of the registry; never reconnect, pollers stop
	serverHost  bool      // the local one, when remuda serves the page only: its herdr is never asked (Manager.SetServer)
	herdrBin    string    // herdr's binary on the machine, once known
	socket      string
	os          string // darwin | linux | windows as the connection read it, "" before a remote's first connect

	conn   *sshx.Conn
	client herdr.API       // the adapter over the machine's remuda-core, nil before it answers
	attach herdr.Terminals // remuda-core's terminals, the adapter over the machine's own core
	home   string

	speaksCore bool      // reached through remuda-core: a label now (Core 26i made it every machine), gone with the herdr client
	link       *coreLink // the remuda-core it follows, while connected
	noCore     error     // why a remote connected on remuda-core follows no core: none installed there, or none answering

	fwds  map[forwardTo]*tcpForward // ports forwarded over the master, once asked for (code-server, a preview)
	fwdMu sync.Mutex                // one forward made at a time, so asks of a port at once make one, not one each

	onState func(id, label, state, reason string) // the manager's ear for connected / error, nil for the local one

	door     *Door      // the way its agents reach the tasks here, opened after every connect
	doorKey  string     // the key placed there, kept so a reconnect places the same one
	stateDir string     // the folder remuda keeps there
	doorMu   sync.Mutex // held for each step of opening the door: the next connection's waits for it

	// Transcripts reads the session files on this machine.
	Transcripts *transcript.Reader
}

// forwardTo is what a forward reaches on the machine.
type forwardTo struct {
	host string
	port int
}

// tcpForward is a port forwarded over one master; a new master needs a new forward.
type tcpForward struct {
	conn *sshx.Conn
	port int
	addr string
}

// hostRe accepts ssh destinations (host, user@host, [v6]:port) and nothing ssh would read as an option.
var hostRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@:%\[\]-]*$`)

// Label is the machine's name as shown: the user's own when they gave one, else the registry's.
func (mc *Machine) Label() string {
	if l, ok := mc.label.Load().(string); ok && l != "" {
		return l
	}
	return mc.Cfg.Label
}

func (mc *Machine) setLabel(l string) { mc.label.Store(l) }

func (mc *Machine) State() (state, errMsg string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.state, mc.errMsg
}

// Removed reports whether the machine was taken out of the registry.
func (mc *Machine) Removed() bool {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.removed
}

// fail tears the connection down and records why.
func (mc *Machine) fail(err error) {
	mc.mu.Lock()
	was := mc.state
	mc.state = StateError
	mc.errMsg = err.Error()
	conn, link := mc.conn, mc.link
	mc.conn, mc.client, mc.attach, mc.link, mc.noCore = nil, nil, nil, nil, nil
	hook := mc.onState
	mc.mu.Unlock()
	if link != nil {
		link.close()
	}
	if conn != nil {
		conn.Close()
	}
	if hook != nil && was != StateError {
		hook(mc.ID(), mc.Label(), StateError, err.Error())
	}
}

// connect dials the master, reads the remote home and system, and reaches remuda-core there.
func (mc *Machine) connect() {
	mc.mu.Lock()
	mc.state = StateConnecting
	mc.errMsg = ""
	mc.lastAttempt = time.Now()
	mc.mu.Unlock()

	conn, err := sshx.Dial(mc.Cfg.Host)
	if err != nil {
		mc.fail(err)
		return
	}
	bail := func(err error) {
		conn.Close()
		mc.fail(err)
	}

	// one round trip for $HOME and the system, and remuda-core's binary: a non-interactive ssh PATH often misses ~/.local/bin
	out, err := shell.Text(conn, probeScript()+coreLookup, 10*time.Second)
	if err != nil {
		if windowsShell(out + err.Error()) {
			bail(errWindowsShell(mc.Cfg.Host))
			return
		}
		bail(fmt.Errorf("cannot read remote $HOME: %v", err))
		return
	}
	found := parseProbe(out)
	if found.home == "" {
		bail(fmt.Errorf("cannot read remote $HOME"))
		return
	}
	mc.mu.Lock()
	mc.socket = herdrSocket(mc.Cfg, found) // herdr's, as remuda dialled it before the move: MovedHerdr
	mc.mu.Unlock()
	link, err := mc.reachCore(conn, found)
	if err != nil { // the master stays, so the vitals and Updates' Install remuda-core reach the machine
		mc.connected(conn, found, nil, nil, nil, err)
		return
	}
	mc.connected(conn, found, link.adapter, link.terminals, link, nil)
}

// connected makes the connection the machine's and tells whoever listens, unless the machine was removed while it
// was made; then opens the door there. noCore is why a machine on remuda-core follows none over it.
func (mc *Machine) connected(conn *sshx.Conn, found probe, client herdr.API, attach herdr.Terminals, link *coreLink, noCore error) {
	mc.mu.Lock()
	if mc.removed { // removed while this connect was in flight
		mc.mu.Unlock()
		if link != nil {
			link.close()
		}
		conn.Close()
		return
	}
	mc.conn = conn
	mc.client, mc.attach, mc.link, mc.noCore = client, attach, link, noCore
	mc.home = found.home
	mc.stateDir = found.state
	mc.os = found.os
	mc.state = StateConnected
	mc.errMsg = ""
	hook := mc.onState
	mc.mu.Unlock()
	go mc.openDoor(conn)
	if hook != nil {
		hook(mc.ID(), mc.Label(), StateConnected, "")
	}
}

// transportError marks the machine dead, for Ready to reconnect, when an RPC failed and the ssh master is gone too.
func (mc *Machine) transportError(err error) {
	mc.mu.Lock()
	conn := mc.conn
	if conn == nil || time.Since(mc.lastAlive) < 5*time.Second {
		mc.mu.Unlock()
		return
	}
	mc.lastAlive = time.Now() // stamped only when a check actually runs
	mc.mu.Unlock()
	if !conn.Alive() {
		mc.fail(fmt.Errorf("ssh connection lost: %v", err))
	}
}

// Ready hands out the machine's client and attach manager, reconnecting a failed remote at most every 15s; a remote
// connected without its remuda-core says why, and is looked at for one again as often.
func (mc *Machine) Ready() (herdr.API, herdr.Terminals, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if mc.local && mc.serverHost {
		return nil, nil, ErrServerHost
	}
	if mc.local {
		return mc.client, mc.attach, nil
	}
	if mc.removed {
		return nil, nil, fmt.Errorf("machine %s was removed", mc.ID())
	}
	switch mc.state {
	case StateConnected:
		if mc.noCore != nil {
			mc.tryAgain(mc.RetryCore)
			return nil, nil, &NoCoreError{Reason: mc.noCore}
		}
		return mc.client, mc.attach, nil
	case StateConnecting:
		return nil, nil, fmt.Errorf("machine %s is connecting", mc.ID())
	default:
		mc.tryAgain(mc.connect)
		return nil, nil, fmt.Errorf("machine %s: %s", mc.ID(), mc.errMsg)
	}
}

// tryAgain starts try in the background once 15 s have passed since the last attempt; called under mu.
func (mc *Machine) tryAgain(try func()) {
	if time.Since(mc.lastAttempt) > 15*time.Second {
		mc.lastAttempt = time.Now()
		go try()
	}
}

// CodeAddr is where this machine's code-server (127.0.0.1:port on the machine itself) is reachable from here.
func (mc *Machine) CodeAddr(port int) (string, error) { return mc.ForwardAddr(port) }

// ForwardAddr is a loopback address that reaches 127.0.0.1:port on the machine, a forward remembered per master.
func (mc *Machine) ForwardAddr(port int) (string, error) {
	return mc.ForwardHostAddr("127.0.0.1", port)
}

// ForwardHostAddr is an address here that reaches host:port as the machine dials it, a forward remembered per master.
func (mc *Machine) ForwardHostAddr(host string, port int) (string, error) {
	if mc.local {
		return net.JoinHostPort(host, strconv.Itoa(port)), nil
	}
	conn, err := mc.remoteConn()
	if err != nil {
		return "", err
	}
	to := forwardTo{host: host, port: port}
	mc.fwdMu.Lock()
	defer mc.fwdMu.Unlock()
	mc.mu.Lock()
	if f := mc.fwds[to]; f != nil && f.conn == conn {
		mc.mu.Unlock()
		return f.addr, nil
	}
	mc.mu.Unlock()
	addr, err := conn.ForwardTCPTo(host, port)
	if err != nil {
		return "", err
	}
	mc.mu.Lock()
	if mc.fwds == nil {
		mc.fwds = map[forwardTo]*tcpForward{}
	}
	mc.fwds[to] = &tcpForward{conn: conn, port: port, addr: addr}
	mc.mu.Unlock()
	return addr, nil
}

// ---- remote execution ----

// RemoteConn is the ssh master to the machine (an error for the local one).
func (mc *Machine) RemoteConn() (*sshx.Conn, error) { return mc.remoteConn() }

func (mc *Machine) remoteConn() (*sshx.Conn, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if mc.conn == nil {
		return nil, fmt.Errorf("not connected")
	}
	return mc.conn, nil
}
