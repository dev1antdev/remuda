package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/state"
)

const devicesFile = "devices.json"

// cookiePrefix tells a device cookie from a legacy one holding the token itself.
const cookiePrefix = "d."

// Device is one signed-in browser: a secret of its own in its cookie (only its hash is kept), a name, when and where it was last seen.
type Device struct {
	ID       string    `json:"id"`
	Hash     string    `json:"hash"`
	Name     string    `json:"name"`
	Created  time.Time `json:"created"`
	LastSeen time.Time `json:"last_seen"`
	IP       string    `json:"ip,omitempty"`
	Agent    string    `json:"agent,omitempty"`
}

// Kind is the browser and the system the device's user agent names, "Chrome on macOS".
func (d Device) Kind() string { return DeviceName(d.Agent) }

// Devices is the store of signed-in devices, <state dir>/devices.json.
type Devices struct {
	mu   sync.Mutex
	dir  string
	list map[string]*Device
	now  func() time.Time
}

// ErrNoDevice names an id the store does not hold.
var ErrNoDevice = errors.New("no such device")

// OpenDevices loads the store; a missing file is an empty store, a damaged one an error.
func OpenDevices(dir string) (*Devices, error) {
	var saved []Device
	if err := state.LoadJSON(dir, devicesFile, &saved); err != nil {
		return nil, fmt.Errorf("devices: %w", err)
	}
	devices := &Devices{dir: dir, list: map[string]*Device{}, now: time.Now}
	for i := range saved {
		device := saved[i]
		devices.list[device.ID] = &device
	}
	return devices, nil
}

// Mint signs a browser in: a random secret whose hash is kept, a name read off the user agent; the cookie value carries the id and the secret.
func (d *Devices) Mint(ip, agent string) (cookie string, device Device, err error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", Device{}, err
	}
	raw := hex.EncodeToString(secret)
	now := d.now()
	device = Device{ID: state.NewID(), Hash: hashOf(raw), Name: DeviceName(agent), Created: now, LastSeen: now, IP: ip, Agent: trimAgent(agent)}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.list[device.ID] = &device
	if err := d.saveLocked(); err != nil {
		delete(d.list, device.ID)
		return "", Device{}, err
	}
	return cookiePrefix + device.ID + "." + raw, device, nil
}

// IsDeviceCookie says whether a cookie value names a device rather than holding the token itself.
func IsDeviceCookie(value string) bool { return strings.HasPrefix(value, cookiePrefix) }

// Check verifies a device cookie against the stored hash in constant time and notes the device was seen, at most once a minute.
func (d *Devices) Check(cookie string) (Device, bool) {
	rest, ok := strings.CutPrefix(cookie, cookiePrefix)
	if !ok {
		return Device{}, false
	}
	id, secret, ok := strings.Cut(rest, ".")
	if !ok || id == "" || secret == "" {
		return Device{}, false
	}
	presented := hashOf(secret)
	d.mu.Lock()
	defer d.mu.Unlock()
	device, ok := d.list[id]
	if !ok || subtle.ConstantTimeCompare([]byte(device.Hash), []byte(presented)) != 1 {
		return Device{}, false
	}
	if now := d.now(); now.Sub(device.LastSeen) >= time.Minute {
		device.LastSeen = now
		if err := d.saveLocked(); err != nil {
			log.Printf("devices: %v", err)
		}
	}
	return *device, true
}

// List is every signed-in device, the most recently seen first.
// Name is the device's name, for a row that says who did something; false for an id it does not know.
func (d *Devices) Name(id string) (string, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	device, ok := d.list[id]
	if !ok {
		return "", false
	}
	return device.Name, true
}

func (d *Devices) List() []Device {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sortedLocked()
}

// Rename gives a device a name of the person's own; an empty name goes back to the one its user agent gives.
func (d *Devices) Rename(id, name string) (Device, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	device, ok := d.list[id]
	if !ok {
		return Device{}, ErrNoDevice
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = device.Kind()
	}
	if len(name) > 80 {
		name = name[:80]
	}
	device.Name = name
	if err := d.saveLocked(); err != nil {
		return Device{}, err
	}
	return *device, nil
}

// Revoke forgets a device, so its cookie is refused from then on; false for an id the store does not hold.
func (d *Devices) Revoke(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.list[id]; !ok {
		return false
	}
	delete(d.list, id)
	if err := d.saveLocked(); err != nil {
		log.Printf("devices: %v", err)
	}
	return true
}

// RevokeOthers forgets every device but the one to keep and says how many went.
func (d *Devices) RevokeOthers(keep string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	gone := 0
	for id := range d.list {
		if id != keep {
			delete(d.list, id)
			gone++
		}
	}
	if gone > 0 {
		if err := d.saveLocked(); err != nil {
			log.Printf("devices: %v", err)
		}
	}
	return gone
}

func (d *Devices) saveLocked() error {
	return state.SaveJSON(d.dir, devicesFile, d.sortedLocked())
}

func (d *Devices) sortedLocked() []Device {
	out := make([]Device, 0, len(d.list))
	for _, device := range d.list {
		out = append(out, *device)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].LastSeen.Equal(out[j].LastSeen) {
			return out[i].LastSeen.After(out[j].LastSeen)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func hashOf(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func trimAgent(agent string) string {
	agent = strings.TrimSpace(agent)
	if len(agent) > 200 {
		agent = agent[:200]
	}
	return agent
}

// DeviceName reads the browser and the system off a user agent, "Safari on iPhone", "Chrome on macOS", "a browser" when it says neither.
func DeviceName(agent string) string {
	if strings.Contains(agent, "remuda-mac/") {
		return "remuda on macOS"
	}
	browser := ""
	switch {
	case strings.Contains(agent, "Edg/"):
		browser = "Edge"
	case strings.Contains(agent, "OPR/"), strings.Contains(agent, "Opera"):
		browser = "Opera"
	case strings.Contains(agent, "Firefox/"), strings.Contains(agent, "FxiOS"):
		browser = "Firefox"
	case strings.Contains(agent, "Chrome/"), strings.Contains(agent, "CriOS"), strings.Contains(agent, "HeadlessChrome"):
		browser = "Chrome"
	case strings.Contains(agent, "Safari/"):
		browser = "Safari"
	}
	system := ""
	switch {
	case strings.Contains(agent, "iPhone"):
		system = "iPhone"
	case strings.Contains(agent, "iPad"):
		system = "iPad"
	case strings.Contains(agent, "Android"):
		system = "Android"
	case strings.Contains(agent, "Macintosh"), strings.Contains(agent, "Mac OS X"):
		system = "macOS"
	case strings.Contains(agent, "Windows"):
		system = "Windows"
	case strings.Contains(agent, "CrOS"):
		system = "ChromeOS"
	case strings.Contains(agent, "Linux"):
		system = "Linux"
	}
	switch {
	case browser != "" && system != "":
		return browser + " on " + system
	case browser != "":
		return browser
	case system != "":
		return "a browser on " + system
	}
	return "a browser"
}
