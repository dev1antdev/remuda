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
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/herdr-ui/internal/herdr"
	"github.com/dev1antdev/herdr-ui/internal/sshx"
	"github.com/dev1antdev/herdr-ui/internal/state"
	"github.com/dev1antdev/herdr-ui/internal/transcript"
)

const (
	LocalID     = "local"
	SourceHerdr = "herdr" // mirrored from `herdr machine list`

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
	Cfg   Config
	Local bool

	mu          sync.Mutex
	state       string
	errMsg      string
	lastAttempt time.Time
	lastAlive   time.Time // last `ssh -O check`, so transport errors don't spawn one each
	removed     bool      // taken out of the registry; never reconnect, pollers stop

	conn   *sshx.Conn
	client *herdr.Client
	attach *herdr.AttachManager
	home   string

	files    map[string]*remoteFile // remote transcript cache by path
	sessPath map[string]string      // session uuid -> resolved path
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
	herdrBin string
}

// NewManager builds the registry: the local machine plus whatever
// machines.json lists. Machines saved in herdr itself are added by
// SyncHerdr, which the watcher calls periodically.
func NewManager(dir, localSocket, localHerdrBin string) *Manager {
	local := &Machine{
		Cfg:    Config{ID: LocalID, Label: "This Mac"},
		Local:  true,
		state:  StateConnected,
		client: herdr.NewClient(localSocket),
		attach: herdr.NewAttachManager(herdr.LocalAttachCommand(localHerdrBin, localSocket)),
	}
	m := &Manager{
		machines: map[string]*Machine{LocalID: local},
		order:    []string{LocalID},
		path:     filepath.Join(dir, "machines.json"),
		herdrBin: localHerdrBin,
	}
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
	for _, h := range list {
		if !h.Enabled || h.ID == "" || !hostRe.MatchString(h.Target) || m.has(h.ID) || m.hasHost(h.Target) {
			continue
		}
		m.add(Config{ID: h.ID, Label: h.Label, Host: h.Target, Source: SourceHerdr})
	}
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
		return fmt.Errorf("%s comes from herdr's machine list — remove it with `herdr machine remove`", mc.Cfg.Label)
	}
	delete(m.machines, id)
	for i, oid := range m.order {
		if oid == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	m.mu.Unlock()
	if mc == nil {
		return fmt.Errorf("machine %q not found", id)
	}
	mc.mu.Lock()
	mc.removed = true
	mc.mu.Unlock()
	mc.fail(fmt.Errorf("removed"))
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
	out, err := conn.Exec(`printf '%s\n' "$HOME"; command -v herdr 2>/dev/null || `+
		`for p in "$HOME/.local/bin/herdr" /opt/homebrew/bin/herdr /usr/local/bin/herdr /usr/bin/herdr; do [ -x "$p" ] && printf '%s\n' "$p" && break; done`,
		10*time.Second)
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
		bin = strings.TrimSpace(strings.Split(binLine, "\n")[0])
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
	recent := time.Since(mc.lastAlive) < 5*time.Second
	mc.lastAlive = time.Now()
	mc.mu.Unlock()
	if conn == nil || recent {
		return
	}
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

// ---- remote execution ----

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

	q := sshx.Quote(path)
	var data []byte
	if cached != nil && v.Size > cached.version.Size {
		// grew: fetch the delta only (tail -c +N is 1-indexed)
		if delta, err := mc.execBytes(fmt.Sprintf("tail -c +%d %s", cached.version.Size+1, q), 30*time.Second); err == nil {
			data = append(cached.data, delta...)
		}
	}
	if data == nil {
		fetch := min(v.Size, remoteTailCap)
		var err error
		if data, err = mc.execBytes(fmt.Sprintf("tail -c %d %s", fetch, q), 60*time.Second); err != nil {
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
	mc.mu.Unlock()
	if ok {
		if !mc.Local {
			return cached, nil
		}
		if _, err := os.Stat(cached); err == nil {
			return cached, nil
		}
	}

	var p string
	if mc.Local {
		var err error
		if p, err = transcript.FindSessionByID(cwd, value); err != nil {
			return "", err
		}
	} else {
		projects := mc.Home() + "/.claude/projects"
		primary := projects + "/" + transcript.EncodePath(cwd) + "/" + value + ".jsonl"
		script := `if [ -f ` + sshx.Quote(primary) + ` ]; then printf %s ` + sshx.Quote(primary) + `; else ls ` +
			sshx.Quote(projects) + `/*/` + sshx.Quote(value+".jsonl") + ` 2>/dev/null | head -1; fi`
		out, err := mc.exec(script, 15*time.Second)
		if err != nil {
			return "", err
		}
		if p = strings.TrimSpace(out); p == "" {
			return "", os.ErrNotExist
		}
	}
	mc.mu.Lock()
	if mc.sessPath == nil {
		mc.sessPath = map[string]string{}
	}
	mc.sessPath[value] = p
	mc.mu.Unlock()
	return p, nil
}
