package machines

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/script"
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
	serverStart time.Time // when remuda last started `herdr server` there
	herdrBin    string    // herdr's binary on the machine, once known
	socket      string

	conn   *sshx.Conn
	client *herdr.Client
	attach *herdr.AttachManager
	home   string

	fwds map[int]*tcpForward // ports forwarded over the master, once asked for (code-server, a peer remuda)
	peer *Peer

	// Transcripts reads the session files on this machine.
	Transcripts *transcript.Reader
}

// tcpForward is a port forwarded over one master; a new master needs a new forward.
type tcpForward struct {
	conn *sshx.Conn
	port int
	addr string
}

// Peer is the remuda running on the machine itself. herdr lets one client attach to a terminal at a time,
// so when the peer holds that attach, this remuda reaches the pane through it (relayed over the master).
type Peer struct {
	Port    int
	Token   string
	Version string
	At      time.Time
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
	mc.state = StateError
	mc.errMsg = err.Error()
	conn := mc.conn
	mc.conn, mc.client, mc.attach = nil, nil, nil
	mc.mu.Unlock()
	if conn != nil {
		conn.Close()
	}
}

// connect dials the master, finds the remote home and herdr binary, picks the RPC transport and pings herdr.
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

	// one round trip for $HOME and the herdr binary: a non-interactive ssh PATH often misses ~/.local/bin;
	// the script ends true, so a missing binary is a blank line, not a failed exec
	probe := `printf '%s\n' "$HOME"`
	if mc.Cfg.HerdrBin == "" {
		probe += `; command -v herdr 2>/dev/null || ` +
			`for p in "$HOME/.local/bin/herdr" /opt/homebrew/bin/herdr /usr/local/bin/herdr /usr/bin/herdr; do [ -x "$p" ] && printf '%s\n' "$p" && break; done; true`
	}
	out, err := shell.Text(conn, probe, 10*time.Second)
	if err != nil {
		bail(fmt.Errorf("cannot read remote $HOME: %v", err))
		return
	}
	home, binLine, _ := strings.Cut(out, "\n")
	home = strings.TrimSpace(home)
	if home == "" {
		bail(fmt.Errorf("cannot read remote $HOME"))
		return
	}
	bin := mc.Cfg.HerdrBin
	if bin == "" {
		bin, _, _ = strings.Cut(strings.TrimSpace(binLine), "\n")
	}
	if bin == "" {
		bail(fmt.Errorf("herdr not found on %s (set herdr_bin in machines.json)", mc.Cfg.Host))
		return
	}

	sock := mc.Cfg.RemoteSocket
	if sock == "" {
		sock = herdr.DefaultSocket(home)
	}
	client, err := rpcClient(conn, sock)
	if err != nil && mc.mayStartServer() {
		if _, serr := conn.Run(herdr.StartServerScript(bin, sock), 25*time.Second); serr == nil {
			client, err = rpcClient(conn, sock)
		} else {
			log.Printf("%s: start herdr server: %v", mc.Cfg.Host, serr)
		}
	}
	if err != nil {
		bail(fmt.Errorf("herdr on %s not answering through %s: %v", mc.Cfg.Host, sock, err))
		return
	}
	client.OnTransportError = mc.transportError

	mc.mu.Lock()
	if mc.removed { // removed while this connect was in flight
		mc.mu.Unlock()
		conn.Close()
		return
	}
	mc.conn = conn
	mc.client = client
	mc.home = home
	mc.herdrBin, mc.socket = bin, sock
	mc.attach = herdr.NewAttachManager(func(termID string) *exec.Cmd {
		return conn.AttachCmd(bin, termID)
	})
	mc.state = StateConnected
	mc.errMsg = ""
	mc.mu.Unlock()
}

// rpcClient picks the RPC transport: a forwarded unix socket, else one `nc -U` exec per request over the master
// (some sshds refuse streamlocal forwarding with "unknown channel type"; herdr answers once per connection anyway).
func rpcClient(conn *sshx.Conn, sock string) (*herdr.Client, error) {
	var errs []string
	if local, err := conn.ForwardSocket(sock); err == nil {
		client := herdr.NewClient(local)
		if _, _, err := client.Ping(); err == nil {
			return client, nil
		} else {
			errs = append(errs, "forward: "+err.Error())
		}
	} else {
		errs = append(errs, err.Error())
	}

	bridge := "nc -U " + script.Quote(sock)
	client := herdr.NewClient(sock)
	client.Dial = func(timeout time.Duration) (net.Conn, error) {
		return conn.DialExec(bridge, timeout)
	}
	if _, _, err := client.Ping(); err == nil {
		return client, nil
	} else {
		errs = append(errs, "nc -U: "+err.Error())
	}
	return nil, fmt.Errorf("%s", strings.Join(errs, "; "))
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
		mc.fail(fmt.Errorf("connection lost: %v", err))
	}
}

// Ready hands out the machine's client and attach manager, reconnecting a failed remote at most every 15s.
func (mc *Machine) Ready() (*herdr.Client, *herdr.AttachManager, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if mc.local {
		return mc.client, mc.attach, nil
	}
	if mc.removed {
		return nil, nil, fmt.Errorf("machine %s was removed", mc.ID())
	}
	switch mc.state {
	case StateConnected:
		return mc.client, mc.attach, nil
	case StateConnecting:
		return nil, nil, fmt.Errorf("machine %s is connecting", mc.ID())
	default:
		if time.Since(mc.lastAttempt) > 15*time.Second {
			mc.lastAttempt = time.Now()
			go mc.connect()
		}
		return nil, nil, fmt.Errorf("machine %s: %s", mc.ID(), mc.errMsg)
	}
}

// CodeAddr is where this machine's code-server (127.0.0.1:port on the machine itself) is reachable from here.
func (mc *Machine) CodeAddr(port int) (string, error) { return mc.ForwardAddr(port) }

// ForwardAddr is a loopback address that reaches 127.0.0.1:port on the machine, a forward remembered per master.
func (mc *Machine) ForwardAddr(port int) (string, error) {
	if mc.local {
		return fmt.Sprintf("127.0.0.1:%d", port), nil
	}
	conn, err := mc.remoteConn()
	if err != nil {
		return "", err
	}
	mc.mu.Lock()
	if f := mc.fwds[port]; f != nil && f.conn == conn {
		mc.mu.Unlock()
		return f.addr, nil
	}
	mc.mu.Unlock()
	addr, err := conn.ForwardTCP(port)
	if err != nil {
		return "", err
	}
	mc.mu.Lock()
	if mc.fwds == nil {
		mc.fwds = map[int]*tcpForward{}
	}
	mc.fwds[port] = &tcpForward{conn: conn, port: port, addr: addr}
	mc.mu.Unlock()
	return addr, nil
}

// SetPeer records the remuda found running on the machine (nil: none).
func (mc *Machine) SetPeer(p *Peer) {
	mc.mu.Lock()
	mc.peer = p
	mc.mu.Unlock()
}

// Peer is the machine's own remuda: nil when none runs there, for the local machine, or past an hour.
func (mc *Machine) Peer() *Peer {
	if mc.local {
		return nil
	}
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if mc.peer == nil || time.Since(mc.peer.At) > time.Hour {
		return nil
	}
	cp := *mc.peer
	return &cp
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
