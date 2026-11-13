package push

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// A socket device hears its alerts over a WebSocket instead of Web Push: the Mac app. Its endpoint reads
// "socket:<id>" and carries no keys; the prefs, the label, the quiet hours and the test are any device's. The hub
// holds one live connection per endpoint and drops what it cannot hand over at once — a device away or behind is
// not queued for, the activity feed is its catch-up. A connection that says it takes withdrawals also hears the
// tags of its alerts that no longer stand, as {"type": "withdraw", "tags": [...]}, so it takes them down.

// SocketPrefix marks a socket device's endpoint.
const SocketPrefix = "socket:"

// socketBuffer is how many alerts a connection may fall behind before the next one is dropped.
const socketBuffer = 16

// dropLogEvery is how often a device's dropped alerts are logged.
const dropLogEvery = time.Hour

// IsSocket says whether an endpoint names a socket device.
func IsSocket(endpoint string) bool { return strings.HasPrefix(endpoint, SocketPrefix) }

func newSocketEndpoint() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return SocketPrefix + hex.EncodeToString(b[:])
}

// Frame is one message to a socket device: an alert, or the tags of alerts taken back.
type Frame struct {
	Alert    *Payload
	Withdraw []string
}

// MarshalJSON writes an alert as the payload itself, as the device read it before withdrawals, and a withdrawal as
// {"type": "withdraw", "tags": [...]}.
func (f Frame) MarshalJSON() ([]byte, error) {
	if f.Alert != nil {
		return json.Marshal(f.Alert)
	}
	return json.Marshal(struct {
		Type string   `json:"type"`
		Tags []string `json:"tags"`
	}{"withdraw", f.Withdraw})
}

// Sockets is the hub of the live connections, one per socket device.
type Sockets struct {
	mu      sync.Mutex
	open    map[string]*socketConn
	dropped map[string]*dropCount
	closed  bool
}

type socketConn struct {
	frames    chan Frame
	withdraws bool // the device takes withdrawals: one built before them would show a withdrawal as a blank alert
}

type dropCount struct {
	n       int
	lastLog time.Time
}

func newSockets() *Sockets {
	return &Sockets{open: map[string]*socketConn{}, dropped: map[string]*dropCount{}}
}

// Attach is one connection's channel of frames, withdrawals among them when the device takes them; a second Attach
// for the same endpoint replaces the first, whose channel is closed. The returned func lets the connection go.
func (h *Sockets) Attach(endpoint string, withdraws bool) (<-chan Frame, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	conn := &socketConn{frames: make(chan Frame, socketBuffer), withdraws: withdraws}
	if h.closed {
		close(conn.frames)
		return conn.frames, func() {}
	}
	if prev := h.open[endpoint]; prev != nil {
		close(prev.frames)
	}
	h.open[endpoint] = conn
	detach := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.open[endpoint] == conn {
			delete(h.open, endpoint)
			close(conn.frames)
		}
	}
	return conn.frames, detach
}

// Connected says whether a connection holds the endpoint right now.
func (h *Sockets) Connected(endpoint string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.open[endpoint] != nil
}

// Deliver hands one alert to the endpoint's connection without waiting; a device away or behind loses it, which is
// counted and logged once an hour per endpoint.
func (h *Sockets) Deliver(endpoint string, p Payload) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conn := h.open[endpoint]; conn != nil {
		select {
		case conn.frames <- Frame{Alert: &p}:
			return
		default:
			h.drop(endpoint, "behind")
			return
		}
	}
	h.drop(endpoint, "not connected")
}

// Withdraw hands a connection that takes withdrawals the tags of its alerts that no longer stand, without waiting;
// false when none took them, so they wait for the device's next connection.
func (h *Sockets) Withdraw(endpoint string, tags []string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	conn := h.open[endpoint]
	if conn == nil || !conn.withdraws {
		return false
	}
	select {
	case conn.frames <- Frame{Withdraw: tags}:
		return true
	default:
		return false
	}
}

func (h *Sockets) drop(endpoint, why string) {
	d := h.dropped[endpoint]
	if d == nil {
		d = &dropCount{}
		h.dropped[endpoint] = d
	}
	d.n++
	if now := time.Now(); now.Sub(d.lastLog) >= dropLogEvery {
		log.Printf("alerts to %.20s…: %d dropped since the last note, the device is %s", endpoint, d.n, why)
		d.lastLog, d.n = now, 0
	}
}

// Closed says whether the hub has been closed: a channel closed since then means remuda is stopping, not a
// connection replaced.
func (h *Sockets) Closed() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.closed
}

// Close ends every connection: remuda is stopping.
func (h *Sockets) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for endpoint, conn := range h.open {
		close(conn.frames)
		delete(h.open, endpoint)
	}
}

// SubscribeSocket mints a socket device when endpoint is empty, else updates that device's prefs (when given) and
// label; the endpoint is answered. An endpoint that is not a known socket device is an error.
func (s *Service) SubscribeSocket(endpoint string, prefs *Prefs, label string) (string, error) {
	if endpoint == "" {
		endpoint = newSocketEndpoint()
		p := Prefs{Input: true, Done: "long"}
		if prefs != nil {
			p = *prefs
		}
		s.Subscribe(Subscription{Endpoint: endpoint, Prefs: p, Label: label}, "")
		return endpoint, nil
	}
	if !IsSocket(endpoint) {
		return "", fmt.Errorf("not a socket device")
	}
	s.mu.Lock()
	prev := s.subs[endpoint]
	s.mu.Unlock()
	if prev == nil {
		return "", fmt.Errorf("unknown socket device")
	}
	sub := *prev
	if prefs != nil {
		sub.Prefs = *prefs
	}
	if label != "" {
		sub.Label = label
	}
	s.Subscribe(sub, "")
	return endpoint, nil
}

// HasSocket says whether the endpoint names a socket device the service knows.
func (s *Service) HasSocket(endpoint string) bool {
	if !IsSocket(endpoint) {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.subs[endpoint] != nil
}
