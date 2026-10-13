// Package machines gives remuda its multi-PC view: the local herdr plus remote
// ones reached over the user's own SSH, each with one ControlMaster, one RPC
// transport, shared terminal attaches and a transcript cache fetched by delta.
package machines

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/sshx"
	"github.com/dev1antdev/remuda/internal/state"
	"github.com/dev1antdev/remuda/internal/transcript"
)

const (
	LocalID     = "local"
	labelsFile  = "labels.json" // the user's own names for machines, by id
	SourceHerdr = "herdr"       // mirrored from `herdr machine list`

	StateConnected  = "connected"
	StateConnecting = "connecting"
	StateError      = "error"
)

type Config struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Host         string `json:"host"`                    // ssh destination, e.g. "studio.remote"
	RemoteSocket string `json:"remote_socket,omitempty"` // default: herdr's socket under the remote home
	HerdrBin     string `json:"herdr_bin,omitempty"`     // default: autodetected
	Source       string `json:"source,omitempty"`        // SourceHerdr, or "" when added here
}

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

// ---- manager ----

type Manager struct {
	mu       sync.Mutex
	machines map[string]*Machine
	order    []string
	path     string // machines.json
	dir      string // the state dir, where labels.json lives
	names    map[string]string
	herdrBin string
	saveMu   sync.Mutex // writes of machines.json, one at a time
}

// Label is the machine's name as shown: the user's own when they gave one, else the registry's.
func (mc *Machine) Label() string {
	if l, ok := mc.label.Load().(string); ok && l != "" {
		return l
	}
	return mc.Cfg.Label
}

func (mc *Machine) setLabel(l string) { mc.label.Store(l) }

// Rename gives the machine a name of the user's own, kept in labels.json; an empty name goes back to the registry's.
func (m *Manager) Rename(id, label string) (*Machine, error) {
	label = strings.TrimSpace(label)
	if len(label) > 60 {
		return nil, fmt.Errorf("a name has 60 characters at most")
	}
	mc, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if label == "" {
		delete(m.names, id)
	} else {
		m.names[id] = label
	}
	names := map[string]string{}
	for k, v := range m.names {
		names[k] = v
	}
	m.mu.Unlock()
	mc.setLabel(label)
	if err := state.SaveJSON(m.dir, labelsFile, names); err != nil {
		return mc, fmt.Errorf("the name could not be kept: %w", err)
	}
	return mc, nil
}

// NewManager builds the registry: the local machine plus what machines.json lists; herdr's own come through SyncHerdr.
func NewManager(dir, localSocket, localHerdrBin string) *Manager {
	host, _ := os.Hostname()
	local := &Machine{
		Cfg:      Config{ID: LocalID, Label: "This Mac · " + strings.TrimSuffix(host, ".local")},
		local:    true,
		state:    StateConnected,
		herdrBin: localHerdrBin,
		socket:   localSocket,
		client:   herdr.NewClient(localSocket),
		attach:   herdr.NewAttachManager(herdr.LocalAttachCommand(localHerdrBin, localSocket)),
	}
	local.Transcripts = transcript.NewReader(local)
	m := &Manager{
		machines: map[string]*Machine{LocalID: local},
		order:    []string{LocalID},
		path:     filepath.Join(dir, "machines.json"),
		dir:      dir,
		names:    map[string]string{},
		herdrBin: localHerdrBin,
	}
	_ = state.LoadJSON(m.dir, labelsFile, &m.names)
	if m.names == nil {
		m.names = map[string]string{}
	}
	local.setLabel(m.names[LocalID])
	for _, cfg := range m.load() {
		m.add(cfg)
	}
	return m
}

func (m *Manager) load() []Config {
	b, err := os.ReadFile(m.path)
	if err != nil {
		return nil
	}
	var cfgs []Config
	if json.Unmarshal(b, &cfgs) != nil {
		return nil
	}
	return cfgs
}

// save persists the manually added machines; herdr's own stay in herdr.
func (m *Manager) save() error {
	m.saveMu.Lock() // one save at a time, in order: a later Add must not lose to an earlier write
	defer m.saveMu.Unlock()
	m.mu.Lock()
	cfgs := []Config{}
	for _, id := range m.order {
		if mc := m.machines[id]; id != LocalID && mc != nil && mc.Cfg.Source != SourceHerdr {
			cfgs = append(cfgs, mc.Cfg)
		}
	}
	m.mu.Unlock()
	b, _ := json.MarshalIndent(cfgs, "", "  ")
	return state.WriteFile(m.path, b)
}

func (m *Manager) add(cfg Config) *Machine {
	mc := newRemote(cfg)
	m.mu.Lock()
	mc.setLabel(m.names[cfg.ID])
	m.machines[cfg.ID] = mc
	m.order = append(m.order, cfg.ID)
	m.mu.Unlock()
	go mc.connect()
	return mc
}

func newRemote(cfg Config) *Machine {
	mc := &Machine{Cfg: cfg, state: StateConnecting}
	mc.Transcripts = transcript.NewReader(mc)
	return mc
}

// herdrMachine is one row of `herdr machine list --json`.
type herdrMachine struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Target  string `json:"target"`
	Enabled bool   `json:"enabled"`
}

// SyncHerdr mirrors herdr's own saved SSH machines, under herdr's ids so a rename there keeps links and preferences.
func (m *Manager) SyncHerdr() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, m.herdrBin, "machine", "list", "--json").Output()
	if err != nil {
		return
	}
	var list []herdrMachine
	if json.Unmarshal(out, &list) != nil {
		return
	}
	live := map[string]bool{}
	for _, h := range list {
		if !h.Enabled || h.ID == "" || !hostRe.MatchString(h.Target) {
			continue
		}
		live[h.ID] = true
		if m.has(h.ID) || m.hasHost(h.Target) {
			continue
		}
		m.add(Config{ID: h.ID, Label: h.Label, Host: h.Target, Source: SourceHerdr})
	}
	m.mu.Lock()
	var gone []string
	for id, mc := range m.machines {
		if mc.Cfg.Source == SourceHerdr && !live[id] {
			gone = append(gone, id)
		}
	}
	m.mu.Unlock()
	for _, id := range gone {
		m.drop(id)
	}
}

// drop takes a machine out of the registry and ends its connection; its watcher loop and pollers stop on their own.
func (m *Manager) drop(id string) *Machine {
	m.mu.Lock()
	mc := m.machines[id]
	delete(m.machines, id)
	for i, oid := range m.order {
		if oid == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	m.mu.Unlock()
	if mc != nil {
		mc.mu.Lock()
		mc.removed = true
		mc.mu.Unlock()
		mc.fail(fmt.Errorf("removed"))
	}
	return mc
}

func (m *Manager) has(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.machines[id] != nil
}

func (m *Manager) hasHost(host string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, mc := range m.machines {
		if mc.Cfg.Host == host {
			return true
		}
	}
	return false
}

var idRe = regexp.MustCompile(`[^a-z0-9-]+`)

func slug(s string) string {
	return strings.Trim(idRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// Add registers a machine by hand and starts connecting.
func (m *Manager) Add(label, host string) (*Machine, error) {
	host = strings.TrimSpace(host)
	if !hostRe.MatchString(host) {
		return nil, fmt.Errorf("%q is not an ssh destination", host)
	}
	if label == "" {
		label = host
	}
	if m.hasHost(host) {
		return nil, fmt.Errorf("a machine for %s already exists", host)
	}
	id := slug(label)
	if id == "" || id == LocalID || m.has(id) {
		id = slug(host)
	}
	if m.has(id) {
		return nil, fmt.Errorf("machine %q already exists", id)
	}
	mc := m.add(Config{ID: id, Label: label, Host: host})
	return mc, m.save()
}

func (m *Manager) Remove(id string) error {
	if id == LocalID {
		return fmt.Errorf("cannot remove the local machine")
	}
	m.mu.Lock()
	mc := m.machines[id]
	if mc != nil && mc.Cfg.Source == SourceHerdr {
		m.mu.Unlock()
		return fmt.Errorf("%s comes from herdr's machine list — remove it with `herdr machine remove`", mc.Label())
	}
	m.mu.Unlock()
	if m.drop(id) == nil {
		return fmt.Errorf("machine %q not found", id)
	}
	return m.save()
}

func (m *Manager) Get(id string) (*Machine, error) {
	if id == "" {
		id = LocalID
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	mc := m.machines[id]
	if mc == nil {
		return nil, fmt.Errorf("machine %q not found", id)
	}
	return mc, nil
}

func (m *Manager) List() []*Machine {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Machine, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.machines[id])
	}
	return out
}

func (m *Manager) Close() {
	for _, mc := range m.List() {
		mc.mu.Lock()
		conn := mc.conn
		mc.mu.Unlock()
		if conn != nil {
			conn.Close()
		}
	}
}

// ---- machine lifecycle ----

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

// HerdrBin is herdr's binary on the machine, "" before a remote's first connect.
func (mc *Machine) HerdrBin() string {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.herdrBin
}

// ServerVersions reads the installed herdr's version and the running server's ("" when none runs), freshly.
func (mc *Machine) ServerVersions() (installed, running string, err error) {
	bin, sock := mc.herdrPaths()
	out, err := mc.text(`LC_ALL=C; `+script.Quote(bin)+` --version 2>/dev/null | head -1; HERDR_SOCKET_PATH=`+script.Quote(sock)+` `+script.Quote(bin)+` status server 2>/dev/null | sed -n 's/^version: //p' | head -1`, 20*time.Second)
	if err != nil && strings.TrimSpace(out) == "" {
		return "", "", err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	installed = herdrSemver.FindString(lines[0])
	if len(lines) > 1 {
		running = herdrSemver.FindString(lines[1])
	}
	return installed, running, nil
}

var herdrSemver = regexp.MustCompile(`\d+\.\d+\.\d+`)

// StopServer stops herdr's server on the machine, and every pane process with it.
func (mc *Machine) StopServer() (string, error) {
	bin, sock := mc.herdrPaths()
	return mc.text(`HERDR_SOCKET_PATH=`+script.Quote(sock)+` `+script.Quote(bin)+` server stop 2>&1`, 30*time.Second)
}

// StartServer starts herdr's server on the machine now, without the spacing of the automatic starts.
func (mc *Machine) StartServer() error {
	bin, sock := mc.herdrPaths()
	if mc.local {
		return herdr.StartServerNow(bin, sock)
	}
	conn, err := mc.remoteConn()
	if err != nil {
		return err
	}
	mc.mu.Lock()
	mc.serverStart = time.Now()
	mc.mu.Unlock()
	_, err = conn.Run(herdr.StartServerScript(bin, sock), 25*time.Second)
	return err
}

func (mc *Machine) herdrPaths() (bin, sock string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	bin, sock = mc.herdrBin, mc.socket
	if bin == "" {
		bin = "herdr"
	}
	return bin, sock
}

// mayStartServer allows one `herdr server` start per two minutes, noting the one it allows.
func (m *Machine) mayStartServer() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if time.Since(m.serverStart) < 2*time.Minute {
		return false
	}
	m.serverStart = time.Now()
	return true
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
		return nil, nil, fmt.Errorf("machine %s was removed", mc.Cfg.ID)
	}
	switch mc.state {
	case StateConnected:
		return mc.client, mc.attach, nil
	case StateConnecting:
		return nil, nil, fmt.Errorf("machine %s is connecting", mc.Cfg.ID)
	default:
		if time.Since(mc.lastAttempt) > 15*time.Second {
			mc.lastAttempt = time.Now()
			go mc.connect()
		}
		return nil, nil, fmt.Errorf("machine %s: %s", mc.Cfg.ID, mc.errMsg)
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
