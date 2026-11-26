// Package machines gives remuda its multi-PC view: the local machine plus remote
// ones reached over the user's own SSH, each with one ControlMaster, one RPC
// transport, one remuda-core link and a transcript cache fetched by delta.
package machines

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/coreapi"
	"github.com/dev1antdev/remuda/internal/state"
	"github.com/dev1antdev/remuda/internal/transcript"
	"github.com/dev1antdev/remuda/internal/wake"
)

const (
	LocalID    = "local"
	labelsFile = "labels.json" // the user's own names for machines, by id
	macsFile   = "macs.json"   // the MAC address of a machine the person may want to wake, by id

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
	Source       string `json:"source,omitempty"`        // "" when added here: the roster's contract, which named herdr's list once
	HerdrStopped bool   `json:"herdr_stopped,omitempty"` // its herdr server was stopped at the person's press: this remuda writes checkouts there
}

type Manager struct {
	mu       sync.Mutex
	machines map[string]*Machine
	order    []string
	path     string // machines.json
	dir      string // the state dir, where labels.json and herdr-stopped.json live
	names    map[string]string
	macs     map[string]string                     // the MAC addresses, for the magic packet that wakes a sleeping machine
	placed   []string                              // the person's order of the machines, by id (order.json); order is the arrival's
	door     *Door                                 // how every remote's agents reach the tasks here; nil opens none
	saveMu   sync.Mutex                            // writes of machines.json, one at a time
	onState  func(id, label, state, reason string) // hears every remote's connection move, set by OnState
	server   bool                                  // the host serves the page only: no machine of the fleet (SetServer)
	core     bool                                  // every machine is reached through remuda-core (NewCoreManager)
	roots    *transcript.RootStore                 // the folders and conversations its agents named, kept across restarts
	// herdr's machines, never saved, were read once since the start, or there is no herdr: the fleet is whole (Gone)
}

// ErrServerHost is what a request for the host's own herdr gets when remuda runs as a server: it has no workspaces.
var ErrServerHost = errors.New("this remuda runs as a server: the machine it runs on has no workspaces of its own; pick one of its machines")

// SetServer makes the host no machine of the fleet: List leaves it out, so the roster, the watcher, the search, the
// usage never sees it, and its herdr is never asked; its vitals are still read (All).
func (m *Manager) SetServer(on bool) {
	m.mu.Lock()
	m.server = on
	local := m.machines[LocalID]
	m.mu.Unlock()
	local.mu.Lock()
	local.serverHost = on
	local.mu.Unlock()
}

// SetMovedHerdr is the herdr this host ran before the move to remuda-core, as remuda reached it: its binary and socket,
// "" for herdr's own lookup and socket (MovedHerdr).
func (m *Manager) SetMovedHerdr(bin, socket string) {
	m.mu.Lock()
	local := m.machines[LocalID]
	m.mu.Unlock()
	local.mu.Lock()
	local.herdrBin, local.socket = bin, socket
	local.mu.Unlock()
}

// Server says whether the host serves the page only.
func (m *Manager) Server() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.server
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

// SetMAC keeps the machine's MAC address, the one a wake-up packet names, in macs.json; an empty address forgets it.
func (m *Manager) SetMAC(id, mac string) (*Machine, error) {
	mac = strings.TrimSpace(mac)
	if mac != "" {
		canonical, err := wake.ParseMAC(mac)
		if err != nil {
			return nil, err
		}
		mac = canonical
	}
	mc, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if mac == "" {
		delete(m.macs, id)
	} else {
		m.macs[id] = mac
	}
	macs := map[string]string{}
	for k, v := range m.macs {
		macs[k] = v
	}
	m.mu.Unlock()
	if err := state.SaveJSON(m.dir, macsFile, macs); err != nil {
		return mc, fmt.Errorf("the address could not be kept: %w", err)
	}
	return mc, nil
}

// MAC is the machine's kept MAC address, "" when none.
func (m *Manager) MAC(id string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.macs[id]
}

// Rename gives the machine a name of the user's own, kept in labels.json; an empty name goes back to the registry's.
func (m *Manager) Rename(id, label string) (*Machine, error) {
	label = strings.TrimSpace(label)
	if utf8.RuneCountInString(label) > 60 {
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

// localLabel is the name the machine remuda runs on goes by until the person gives it one: "This Mac · studio" on a
// Mac, "This machine · rp-orange" on anything else (a cockpit on a Raspberry Pi said "This Mac" once)
func localLabel(goos, host string) string {
	host = strings.TrimSuffix(host, ".local")
	if goos == "darwin" {
		return "This Mac · " + host
	}
	return "This machine · " + host
}

// NewManager builds the registry: the local machine, reached through its remuda-core, plus what machines.json lists.
// Every remote gets the door given once it connects (nil: none). A host serving the page only (server) reaches no core
// of its own: none is started there.
func NewManager(dir string, door *Door, server bool) *Manager {
	local := localMachine(dir)
	local.speaksCore = true
	if !server {
		local.link = newCoreLink(local, core.New(dialLocalCore), nil)
		local.client, local.attach = local.link.adapter, local.link.terminals
	}
	m := newManager(dir, door, local, true)
	m.SetServer(server)
	return m
}

// NewManagerWith is NewManager with the local machine reached through the API and terminals given instead of a core of
// this remuda's: what a test puts in their place (internal/coreapi/corefake), so no core of the person's is dialled. A
// host serving the page only has none of either.
func NewManagerWith(dir string, door *Door, server bool, api coreapi.API, terminals coreapi.Terminals) *Manager {
	local := localMachine(dir)
	local.speaksCore = true
	if !server {
		local.client, local.attach = api, terminals
	}
	m := newManager(dir, door, local, true)
	m.SetServer(server)
	return m
}

// localMachine is this host as a machine of the fleet, before its herdr or core is given.
func localMachine(dir string) *Machine {
	host, _ := os.Hostname()
	local := &Machine{
		Cfg:      Config{ID: LocalID, Label: localLabel(runtime.GOOS, host)},
		local:    true,
		stateDir: dir,
		state:    StateConnected,
	}
	local.Transcripts = transcript.NewReader(local)
	return local
}

func newManager(dir string, door *Door, local *Machine, speaksCore bool) *Manager {
	m := &Manager{
		machines: map[string]*Machine{LocalID: local},
		order:    []string{LocalID},
		path:     filepath.Join(dir, "machines.json"),
		dir:      dir,
		names:    map[string]string{},
		door:     door,
		core:     speaksCore,
		roots:    transcript.OpenRootStore(dir),
	}
	local.Transcripts = local.Transcripts.Keeping(m.roots)
	_ = state.LoadJSON(m.dir, labelsFile, &m.names)
	if m.names == nil {
		m.names = map[string]string{}
	}
	_ = state.LoadJSON(m.dir, macsFile, &m.macs)
	if m.macs == nil {
		m.macs = map[string]string{}
	}
	_ = state.LoadJSON(m.dir, orderFile, &m.placed)
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

// save persists the machines the person added.
func (m *Manager) save() error {
	m.saveMu.Lock() // one save at a time, in order: a later Add must not lose to an earlier write
	defer m.saveMu.Unlock()
	m.mu.Lock()
	cfgs := []Config{}
	for _, id := range m.order {
		if mc := m.machines[id]; id != LocalID && mc != nil {
			cfgs = append(cfgs, mc.Cfg)
		}
	}
	m.mu.Unlock()
	return m.writeMachines(cfgs)
}

// writeMachines keeps the list given in machines.json, one save at a time.
func (m *Manager) writeMachines(cfgs []Config) error {
	b, _ := json.MarshalIndent(cfgs, "", "  ")
	return state.WriteFile(m.path, b)
}

func (m *Manager) add(cfg Config) *Machine {
	mc := newRemote(cfg)
	mc.Transcripts = mc.Transcripts.Keeping(m.roots)
	mc.door = m.door
	mc.onState = m.stateMoved
	mc.speaksCore = m.core
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

// Gone says the fleet has no such machine: it was removed, or it never was there.
func (m *Manager) Gone(id string) bool {
	if id == "" {
		id = LocalID
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.machines[id] == nil
}

// drop takes a machine out of the registry and ends its connection; its watcher loop and pollers stop on their own.
func (m *Manager) drop(id string) *Machine {
	if mc := m.take(id); mc != nil {
		m.retire(mc)
		return mc
	}
	return nil
}

// take takes a machine out of the registry, and nothing more: what ends its connection is retire's, outside any lock a
// caller holds — an ssh master that stopped answering can take a while to let go.
func (m *Manager) take(id string) *Machine {
	m.mu.Lock()
	defer m.mu.Unlock()
	mc := m.machines[id]
	delete(m.machines, id)
	for i, oid := range m.order {
		if oid == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	return mc
}

// retire marks a machine taken out of the registry as removed and ends its connection.
func (m *Manager) retire(mc *Machine) {
	mc.mu.Lock()
	mc.removed = true
	mc.mu.Unlock()
	mc.fail(fmt.Errorf("removed"))
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
	gone, err := m.removeFromFleet(id)
	if err != nil {
		return err
	}
	if gone != nil {
		m.retire(gone) // outside the lock: a master that stopped answering may take a while to let go
	}
	return nil
}

// removeFromFleet checks, writes the fleet without the machine and takes it out of the registry, one at a time: a
// second removal's list is built after the first machine left, so it can never write a removed machine back. What ends
// the connection is the caller's, outside the lock.
func (m *Manager) removeFromFleet(id string) (*Machine, error) {
	m.saveMu.Lock()
	defer m.saveMu.Unlock()
	m.mu.Lock()
	if m.machines[id] == nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("machine %q not found", id)
	}
	// the list without the machine is written before it goes: one that could not be written stays a machine of the
	// fleet, and nothing of it — its checkpoints, its panes — is let go for a removal that did not land
	cfgs := []Config{}
	for _, oid := range m.order {
		if oid != id && oid != LocalID && m.machines[oid] != nil {
			cfgs = append(cfgs, m.machines[oid].Cfg)
		}
	}
	m.mu.Unlock()
	if err := m.writeMachines(cfgs); err != nil {
		return nil, err
	}
	return m.take(id), nil
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

// List is the fleet the cockpit works with: every machine, the host too unless it serves the page only, in the order
// the person gave them (SetOrder), the ones it does not name after those as they arrived.
func (m *Manager) List() []*Machine {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Machine, 0, len(m.order))
	for _, id := range arranged(m.order, m.placed) {
		if m.server && id == LocalID {
			continue
		}
		out = append(out, m.machines[id])
	}
	return out
}

// All is every machine, the host always: its vitals are read and its connections closed whatever it serves.
func (m *Manager) All() []*Machine {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Machine, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.machines[id])
	}
	return out
}

func (m *Manager) Close() {
	for _, mc := range m.All() {
		mc.mu.Lock()
		conn, link := mc.conn, mc.link
		mc.mu.Unlock()
		if link != nil {
			link.close()
		}
		if conn != nil {
			conn.Close()
		}
	}
}
