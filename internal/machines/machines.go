// Package machines gives herdr-ui its multi-PC view: the local herdr plus any
// number of remote ones, each a machine with its own herdr server, reached
// over the user's own SSH. Per machine there is one ControlMaster, one RPC
// transport, shared terminal attaches, and a transcript cache that fetches
// remote session files by delta.
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

	"github.com/dev1antdev/herdr-ui/internal/herdr"
	"github.com/dev1antdev/herdr-ui/internal/sshx"
	"github.com/dev1antdev/herdr-ui/internal/state"
	"github.com/dev1antdev/herdr-ui/internal/transcript"
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
	Host         string `json:"host"`                    // ssh destination, e.g. "gog.remote"
	RemoteSocket string `json:"remote_socket,omitempty"` // default: herdr's socket under the remote home
	HerdrBin     string `json:"herdr_bin,omitempty"`     // default: autodetected
	Source       string `json:"source,omitempty"`        // SourceHerdr, or "" when added here
}

type Machine struct {
	Cfg   Config // as registered: herdr's list, machines.json, or this host's name
	Local bool
	label atomic.Value // the label shown — the user's own when set, else Cfg.Label

	mu          sync.Mutex
	state       string
	errMsg      string
	lastAttempt time.Time
	lastAlive   time.Time // last `ssh -O check`, so transport errors don't spawn one each
	removed     bool      // taken out of the registry; never reconnect, pollers stop
	serverStart time.Time // when herdr-ui last started `herdr server` there
	herdrBin    string    // herdr's binary on the machine, once known
	socket      string    // its server socket

	conn   *sshx.Conn
	client *herdr.Client
	attach *herdr.AttachManager
	home   string

	files    map[string]*remoteFile // remote transcript cache by path
	sessPath map[string]string      // session uuid -> resolved path
	sessMiss map[string]time.Time   // session uuid -> when it was last not found

	fwds map[int]*tcpForward // ports forwarded over the master, once asked for (code-server, a peer herdr-ui)
	peer *Peer               // the herdr-ui running on the machine itself, when one does

	contPath map[string]contCheck // session file -> its continuation (if any), as last looked
	predPath map[string]string    // session file -> the file it continues ("" when none)
}

type contCheck struct {
	next string
	at   time.Time
}

// tcpForward remembers a port forwarded over one master; a reconnect makes
// a new master, and the forward with it.
type tcpForward struct {
	conn *sshx.Conn
	port int
	addr string
}

// Peer is a herdr-ui running on a remote machine. herdr lets one client
// attach to a terminal at a time, so when the machine's own herdr-ui holds
// that attach, this one reaches the terminal through it (the pane
// WebSocket and the HTTP transport are relayed over the master, with the
// peer's token), and both sides can look at the same pane.
type Peer struct {
	Port    int
	Token   string
	Version string
	At      time.Time
}

type remoteFile struct {
	version transcript.Version
	data    []byte
}

const remoteTailCap = 8 * 1024 * 1024

// hostRe accepts ssh destinations — host, user@host, [v6]:port forms — and
// nothing that ssh would read as an option.
var hostRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@:%\[\]-]*$`)

// ---- manager ----

type Manager struct {
	mu       sync.Mutex
	machines map[string]*Machine
	order    []string
	path     string // machines.json
	dir      string // the state dir, where labels.json (the user's own names for machines) lives
	names    map[string]string
	herdrBin string
	saveMu   sync.Mutex // writes of machines.json, one at a time
}

// Label is the machine's name as shown: the user's own when they gave
// one, else what the registry says.
func (mc *Machine) Label() string {
	if l, ok := mc.label.Load().(string); ok && l != "" {
		return l
	}
	return mc.Cfg.Label
}

func (mc *Machine) setLabel(l string) { mc.label.Store(l) }

// Rename gives the machine a name of the user's own; an empty name goes
// back to the registry's. Kept in labels.json, so it survives a restart
// and herdr's own list.
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

// NewManager builds the registry: the local machine plus whatever
// machines.json lists. Machines saved in herdr itself are added by
// SyncHerdr, which the watcher calls periodically.
func NewManager(dir, localSocket, localHerdrBin string) *Manager {
	host, _ := os.Hostname()
	local := &Machine{
		Cfg:      Config{ID: LocalID, Label: "This Mac · " + strings.TrimSuffix(host, ".local")},
		Local:    true,
		state:    StateConnected,
		herdrBin: localHerdrBin,
		socket:   localSocket,
		client:   herdr.NewClient(localSocket),
		attach:   herdr.NewAttachManager(herdr.LocalAttachCommand(localHerdrBin, localSocket)),
	}
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
	return &Machine{Cfg: cfg, state: StateConnecting, files: map[string]*remoteFile{}, sessPath: map[string]string{}}
}

// herdrMachine is one row of `herdr machine list --json`.
type herdrMachine struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Target  string `json:"target"`
	Enabled bool   `json:"enabled"`
}

// SyncHerdr mirrors herdr's own saved SSH machines so a PC added in herdr
// shows up here without being added twice. herdr's stable id is ours too,
// so a rename in herdr keeps links and preferences intact.
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
	// one herdr no longer lists (removed, or disabled) goes here too
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

// drop takes a machine out of the registry and ends its connection; its
// watcher loop and pollers stop on their own.
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

// connect dials the master, learns the remote home and herdr binary, picks
// the RPC transport and verifies herdr answers through it.

// HerdrBin is herdr's binary on the machine, once known ("" before the
// first connect of a remote).
func (mc *Machine) HerdrBin() string {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.herdrBin
}

// ServerVersions reads the installed herdr's version and the running
// server's ("" when none runs), freshly.
func (mc *Machine) ServerVersions() (installed, running string, err error) {
	bin, sock := mc.herdrPaths()
	out, err := mc.Shell(`LC_ALL=C; `+sshx.Quote(bin)+` --version 2>/dev/null | head -1; HERDR_SOCKET_PATH=`+sshx.Quote(sock)+` `+sshx.Quote(bin)+` status server 2>/dev/null | sed -n 's/^version: //p' | head -1`, 20*time.Second)
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

// StopServer stops herdr's server on the machine. Every pane process ends
// with it; the session shape is herdr's to restore on the next start.
func (mc *Machine) StopServer() (string, error) {
	bin, sock := mc.herdrPaths()
	return mc.Shell(`HERDR_SOCKET_PATH=`+sshx.Quote(sock)+` `+sshx.Quote(bin)+` server stop 2>&1`, 30*time.Second)
}

// StartServer starts herdr's server on the machine now — the person asked
// (an update), so the spacing of the automatic starts does not apply.
func (mc *Machine) StartServer() error {
	bin, sock := mc.herdrPaths()
	if mc.Local {
		return herdr.StartServerNow(bin, sock)
	}
	conn, err := mc.remoteConn()
	if err != nil {
		return err
	}
	mc.mu.Lock()
	mc.serverStart = time.Now()
	mc.mu.Unlock()
	_, err = conn.Exec(herdr.StartServerScript(bin, sock), 25*time.Second)
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

// mayStartServer says whether herdr-ui may start `herdr server` on the
// machine now — once per two minutes — and notes that it did.
func (m *Machine) mayStartServer() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if time.Since(m.serverStart) < 2*time.Minute {
		return false
	}
	m.serverStart = time.Now()
	return true
}

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

	// one round trip: home, then the herdr binary (non-interactive ssh often
	// misses ~/.local/bin in PATH)
	// the script ends true, so a missing binary is a blank line, not a
	// failed exec; a configured herdr_bin skips the search
	probe := `printf '%s\n' "$HOME"`
	if mc.Cfg.HerdrBin == "" {
		probe += `; command -v herdr 2>/dev/null || ` +
			`for p in "$HOME/.local/bin/herdr" /opt/homebrew/bin/herdr /usr/local/bin/herdr /usr/bin/herdr; do [ -x "$p" ] && printf '%s\n' "$p" && break; done; true`
	}
	out, err := conn.Exec(probe, 10*time.Second)
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
		// no server there? herdr-ui runs one: the headless `herdr server`,
		// detached, then one more try (at most once per two minutes, so a
		// herdr that cannot come up is not started on every reconnect)
		if _, serr := conn.Exec(herdr.StartServerScript(bin, sock), 25*time.Second); serr == nil {
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

// rpcClient picks the RPC transport: a forwarded unix socket when the remote
// sshd allows it (one hop, fastest), else one `nc -U` exec per request over
// the same master — some sshds answer "unknown channel type" to streamlocal
// forwarding, and herdr's one-response-per-connection protocol makes the
// exec form exactly equivalent.
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

	script := "nc -U " + sshx.Quote(sock)
	client := herdr.NewClient(sock)
	client.Dial = func(timeout time.Duration) (net.Conn, error) {
		return conn.DialExec(script, timeout)
	}
	if _, _, err := client.Ping(); err == nil {
		return client, nil
	} else {
		errs = append(errs, "nc -U: "+err.Error())
	}
	return nil, fmt.Errorf("%s", strings.Join(errs, "; "))
}

// transportError runs when an RPC failed to reach herdr: if the SSH master
// is gone too, the machine is marked dead so Ready reconnects it. The alive
// check spawns a process, so it is rate-limited.
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

// Ready hands out the machine's client and attach manager, kicking off a
// reconnect for a failed remote at most every 15s.
func (mc *Machine) Ready() (*herdr.Client, *herdr.AttachManager, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if mc.Local {
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

// FollowContinuation returns the live end of a session's chain of files: a
// compacted conversation continues in a new file (see
// transcript.Continuation), and herdr goes on naming the first. The search
// costs a directory listing and a read, so it is redone at most every
// minute per file.
func (mc *Machine) FollowContinuation(path string) string {
	chain := mc.ContinuationChain(path)
	return chain[len(chain)-1]
}

// ContinuationChain is the file and every file it went on in, oldest
// first: path, its continuation, that one's continuation… up to 8 hops.
func (mc *Machine) ContinuationChain(path string) []string {
	chain := []string{path}
	for hops := 0; hops < 8; hops++ {
		next := mc.nextContinuation(path)
		if next == "" {
			break
		}
		chain = append(chain, next)
		path = next
	}
	return chain
}

// Predecessor is the file a session file continues (after a compaction
// the conversation went on in a new file; this finds the old one), cached
// for good — the past does not move. "" when there is none.
func (mc *Machine) Predecessor(path string) string {
	mc.mu.Lock()
	p, ok := mc.predPath[path]
	mc.mu.Unlock()
	if ok {
		return p
	}
	if mc.Local {
		p = transcript.Predecessor(path)
	} else if out, err := mc.exec(transcript.PredecessorScript(sshx.Quote(path)), 30*time.Second); err == nil {
		p = strings.TrimSpace(out)
	} else {
		return "" // not cached: asked again next time
	}
	mc.mu.Lock()
	if mc.predPath == nil {
		mc.predPath = map[string]string{}
	}
	mc.predPath[path] = p
	mc.mu.Unlock()
	return p
}

// nextContinuation is one hop, cached for a minute.
func (mc *Machine) nextContinuation(path string) string {
	mc.mu.Lock()
	c, ok := mc.contPath[path]
	mc.mu.Unlock()
	if ok && time.Since(c.at) <= time.Minute {
		return c.next
	}
	var next string
	if mc.Local {
		next = transcript.Continuation(path)
	} else if out, err := mc.exec(transcript.ContinuationScript(sshx.Quote(path)), 20*time.Second); err == nil {
		next = strings.TrimSpace(out)
	}
	mc.mu.Lock()
	if mc.contPath == nil {
		mc.contPath = map[string]contCheck{}
	}
	mc.contPath[path] = contCheck{next: next, at: time.Now()}
	mc.mu.Unlock()
	return next
}

// CodeAddr is where this machine's code-server (127.0.0.1:port on the
// machine itself) is reachable from here: the port as is on the local
// machine, a loopback port forwarded over the ssh master on a remote one.
func (mc *Machine) CodeAddr(port int) (string, error) { return mc.ForwardAddr(port) }

// ForwardAddr is a loopback address that reaches 127.0.0.1:port on the
// machine: the port itself locally, a port forwarded over the master
// remotely, remembered per master.
func (mc *Machine) ForwardAddr(port int) (string, error) {
	if mc.Local {
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

// SetPeer records the herdr-ui found running on the machine (nil: none).
func (mc *Machine) SetPeer(p *Peer) {
	mc.mu.Lock()
	mc.peer = p
	mc.mu.Unlock()
}

// Peer is the machine's own herdr-ui, nil when none runs there, this is
// the local machine, or the reading is over an hour old.
func (mc *Machine) Peer() *Peer {
	if mc.Local {
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

func (mc *Machine) exec(script string, timeout time.Duration) (string, error) {
	conn, err := mc.remoteConn()
	if err != nil {
		return "", err
	}
	return conn.Exec(script, timeout)
}

func (mc *Machine) execBytes(script string, timeout time.Duration) ([]byte, error) {
	conn, err := mc.remoteConn()
	if err != nil {
		return nil, err
	}
	return conn.ExecBytes(script, timeout)
}

// Home is the user's home directory on this machine.
func (mc *Machine) Home() string {
	if mc.Local {
		h, _ := os.UserHomeDir()
		return h
	}
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.home
}

// statScript prints the GNU form, falling back to BSD stat.
func statScript(path, gnuFmt, bsdFmt string) string {
	q := sshx.Quote(path)
	return "stat -c " + sshx.Quote(gnuFmt) + " " + q + " 2>/dev/null || stat -f " + sshx.Quote(bsdFmt) + " " + q
}

// ---- transcripts ----

// TranscriptVersion stats the session file; the version doubles as the ETag.
func (mc *Machine) TranscriptVersion(path string) (transcript.Version, error) {
	if mc.Local {
		return transcript.Stat(path)
	}
	out, err := mc.exec(statScript(path, "%s %Y", "%z %m"), 10*time.Second)
	if err != nil {
		return transcript.Version{}, err
	}
	var v transcript.Version
	if _, err := fmt.Sscanf(out, "%d %d", &v.Size, &v.MTime); err != nil {
		return transcript.Version{}, fmt.Errorf("stat %s: %q", path, out)
	}
	return v, nil
}

// DirVersions is the size and mtime of every .jsonl file directly in dir —
// a session's subagent transcripts — in one round trip; a missing dir is
// an empty map.
func (mc *Machine) DirVersions(dir string) (map[string]transcript.Version, error) {
	out := map[string]transcript.Version{}
	if mc.Local {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return out, nil
			}
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
				continue
			}
			if info, err := e.Info(); err == nil {
				out[filepath.Join(dir, e.Name())] = transcript.Version{Size: info.Size(), MTime: info.ModTime().UnixNano()}
			}
		}
		return out, nil
	}
	q := sshx.Quote(dir)
	script := `[ -d ` + q + ` ] || exit 0; for f in ` + q + `/*.jsonl; do [ -f "$f" ] || continue; stat -c '%s %Y %n' "$f" 2>/dev/null || stat -f '%z %m %N' "$f"; done`
	raw, err := mc.exec(script, 20*time.Second)
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(strings.TrimRight(raw, "\n"), "\n") {
		var v transcript.Version
		rest := ""
		if n, _ := fmt.Sscanf(line, "%d %d %s", &v.Size, &v.MTime, &rest); n == 3 {
			// the path may hold spaces: everything after the two numbers
			parts := strings.SplitN(line, " ", 3)
			out[parts[2]] = v
		}
	}
	return out, nil
}

// Transcript parses the session file at the given version — locally straight
// from disk, remotely from a cached tail that grows by delta.
func (mc *Machine) Transcript(path string, v transcript.Version) ([]transcript.Turn, error) {
	if mc.Local {
		return transcript.Parse(path)
	}
	data, err := mc.remoteTail(path, v)
	if err != nil {
		return nil, err
	}
	return transcript.ParseBytes(data)
}

// remoteTail returns the file tail, fetching only the delta since the cached
// copy when the file merely grew.
func (mc *Machine) remoteTail(path string, v transcript.Version) ([]byte, error) {
	mc.mu.Lock()
	cached := mc.files[path]
	mc.mu.Unlock()
	if cached != nil && cached.version == v {
		return cached.data, nil
	}

	// every read is cut at the size the stat saw: the agent may have written
	// more since, and that part belongs to the next version
	q := sshx.Quote(path)
	var data []byte
	if cached != nil && v.Size > cached.version.Size {
		// grew: fetch the delta only (tail -c +N is 1-indexed)
		delta, err := mc.execBytes(fmt.Sprintf("tail -c +%d %s | head -c %d", cached.version.Size+1, q, v.Size-cached.version.Size), 30*time.Second)
		if err == nil {
			data = append(cached.data[:len(cached.data):len(cached.data)], delta...) // the cached slice is shared: copy
		}
	}
	if data == nil {
		fetch := min(v.Size, remoteTailCap)
		var err error
		if data, err = mc.execBytes(fmt.Sprintf("head -c %d %s | tail -c %d", v.Size, q, fetch), 60*time.Second); err != nil {
			return nil, err
		}
	}
	if len(data) > remoteTailCap {
		data = append([]byte(nil), data[len(data)-remoteTailCap:]...)
	}

	mc.mu.Lock()
	mc.files[path] = &remoteFile{version: v, data: data}
	// the cache is per viewed session; keep it small
	if len(mc.files) > 8 {
		for k := range mc.files {
			if k != path {
				delete(mc.files, k)
				break
			}
		}
	}
	mc.mu.Unlock()
	return data, nil
}

// ResolveSession finds the Claude session file on this machine from herdr's
// agent_session (exact id first, then a glob across project dirs). Hits are
// cached; a local hit is re-checked with one stat.
func (mc *Machine) ResolveSession(cwd, kind, value string) (string, error) {
	if kind == "path" {
		return value, nil
	}
	mc.mu.Lock()
	cached, ok := mc.sessPath[value]
	missedAt, missed := mc.sessMiss[value]
	mc.mu.Unlock()
	if ok {
		if !mc.Local {
			return cached, nil
		}
		if _, err := os.Stat(cached); err == nil {
			return cached, nil
		}
	}
	if missed && time.Since(missedAt) < 30*time.Second {
		return "", os.ErrNotExist // a file not there yet is not looked for on every poll
	}

	var p string
	miss := func(err error) (string, error) {
		mc.mu.Lock()
		if mc.sessMiss == nil {
			mc.sessMiss = map[string]time.Time{}
		}
		mc.sessMiss[value] = time.Now()
		mc.mu.Unlock()
		return "", err
	}
	if mc.Local {
		var err error
		if p, err = transcript.FindSessionByID(cwd, value); err != nil {
			return miss(err)
		}
	} else {
		projects := mc.Home() + "/.claude/projects"
		primary := projects + "/" + transcript.EncodePath(cwd) + "/" + value + ".jsonl"
		script := `if [ -f ` + sshx.Quote(primary) + ` ]; then printf %s ` + sshx.Quote(primary) + `; else ls ` +
			sshx.Quote(projects) + `/*/` + sshx.Quote(value+".jsonl") + ` 2>/dev/null | head -1; fi`
		out, err := mc.exec(script, 15*time.Second)
		if err != nil {
			return miss(err)
		}
		if p = strings.TrimSpace(out); p == "" {
			return miss(os.ErrNotExist)
		}
	}
	mc.mu.Lock()
	if mc.sessPath == nil {
		mc.sessPath = map[string]string{}
	}
	mc.sessPath[value] = p
	delete(mc.sessMiss, value)
	mc.mu.Unlock()
	return p, nil
}
