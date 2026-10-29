// Package machines gives remuda its multi-PC view: the local herdr plus remote
// ones reached over the user's own SSH, each with one ControlMaster, one RPC
// transport, shared terminal attaches and a transcript cache fetched by delta.
package machines

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
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

type Manager struct {
	mu       sync.Mutex
	machines map[string]*Machine
	order    []string
	path     string // machines.json
	dir      string // the state dir, where labels.json lives
	names    map[string]string
	herdrBin string
	door     *Door                                 // how every remote's agents reach the tasks here; nil opens none
	saveMu   sync.Mutex                            // writes of machines.json, one at a time
	onState  func(id, label, state, reason string) // hears every remote's connection move, set by OnState
}

// OnState sets who hears a remote machine's connection move: connected, or error with the reason.
func (m *Manager) OnState(fn func(id, label, state, reason string)) {
	m.mu.Lock()
	m.onState = fn
	m.mu.Unlock()
}

// stateMoved hands a machine's move to whoever OnState named, once there is one.
func (m *Manager) stateMoved(id, label, state, reason string) {
	m.mu.Lock()
	fn := m.onState
	m.mu.Unlock()
	if fn != nil {
		fn(id, label, state, reason)
	}
}

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
// Every remote gets the door given once it connects (nil: none).
func NewManager(dir, localSocket, localHerdrBin string, door *Door) *Manager {
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
		door:     door,
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
	mc.door = m.door
	mc.onState = m.stateMoved
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
