// Package live is the page's one event stream. A Hub hands every connected
// page a subscription; the app's subscriptions tell it when the roster may
// have moved (it announces a change only when the roster's fingerprint did),
// and each page says which panes it shows, so a transcript that grew is
// announced within a second. The page keeps its polls, slower, as the
// safety net: a dropped frame costs a few seconds, never a stale screen.
package live

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// Event is one frame to the page: the roster moved, or a pane's transcript did.
type Event struct {
	Topic   string `json:"topic"` // roster | chat
	Machine string `json:"machine,omitempty"`
	Pane    string `json:"pane,omitempty"`
	Version string `json:"version,omitempty"`
}

// Inputs is what the hub asks the rest of remuda.
type Inputs struct {
	Fingerprint func() string                             // the roster as it would be served, hashed; an event goes out only when it moved
	Version     func(machine, pane string) (string, bool) // the pane's live transcript version, false without a session file
	Remote      func(machine string) bool                 // a remote machine's transcripts are stat'ed over ssh, so less often
}

// Hub fans the events out; one per remuda.
type Hub struct {
	in          Inputs
	Debounce    time.Duration // roster checks are this far apart at most
	LocalEvery  time.Duration // how often a watched local pane's transcript is stat'ed
	RemoteEvery time.Duration // and a remote one's

	mu      sync.Mutex
	subs    map[*Subscription]struct{}
	roster  string               // the fingerprint last announced
	pending *time.Timer          // a roster check already booked
	checks  map[string]paneCheck // "machine|pane" → what was seen last
	closed  bool
}

type paneCheck struct {
	version string
	at      time.Time
}

// Subscription is one page's stream: the events it reads, and the panes it watches.
type Subscription struct {
	Events <-chan Event
	ch     chan Event
	hub    *Hub
	panes  map[string]bool
	closed bool
}

// queue is how many events a slow page may fall behind before frames are dropped (its polls catch it up).
const queue = 64

// MaxWatched caps the panes one page may watch: a wall shows tens, never thousands.
const MaxWatched = 200

func New(in Inputs) *Hub {
	return &Hub{in: in, Debounce: 300 * time.Millisecond, LocalEvery: time.Second, RemoteEvery: 3 * time.Second,
		subs: map[*Subscription]struct{}{}, checks: map[string]paneCheck{}}
}

// Key names a pane the way a page's subscription frame does.
func Key(machine, pane string) string { return machine + "|" + pane }

// Subscribe opens a stream; Close ends it.
func (h *Hub) Subscribe() *Subscription {
	ch := make(chan Event, queue)
	s := &Subscription{Events: ch, ch: ch, hub: h, panes: map[string]bool{}}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		s.closed = true
		close(ch)
		return s
	}
	h.subs[s] = struct{}{}
	return s
}

// Watch replaces the set of panes the page shows; a key is "machine|pane".
func (s *Subscription) Watch(keys []string) {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	s.panes = map[string]bool{}
	for _, key := range keys {
		if len(s.panes) >= MaxWatched {
			break
		}
		if machine, pane, ok := strings.Cut(key, "|"); ok && machine != "" && pane != "" && len(key) < 200 {
			s.panes[key] = true
		}
	}
}

// Watching is the keys the page watches, sorted (tests).
func (s *Subscription) Watching() []string {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	keys := make([]string, 0, len(s.panes))
	for key := range s.panes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Close ends the stream: the page went away.
func (s *Subscription) Close() {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	delete(s.hub.subs, s)
	close(s.ch)
}

// RosterChanged books a check of the roster's fingerprint; calls within the debounce share one check.
func (h *Hub) RosterChanged() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || h.pending != nil {
		return
	}
	h.pending = time.AfterFunc(h.Debounce, h.checkRoster)
}

// checkRoster announces the roster once when its fingerprint moved since the last word.
func (h *Hub) checkRoster() {
	h.mu.Lock()
	h.pending = nil
	closed := h.closed
	h.mu.Unlock()
	if closed || h.in.Fingerprint == nil {
		return
	}
	fingerprint := h.in.Fingerprint()
	h.mu.Lock()
	defer h.mu.Unlock()
	if fingerprint == h.roster {
		return
	}
	h.roster = fingerprint
	h.broadcastLocked(Event{Topic: "roster"}, nil)
}

// PaneChanged tells the pane's watchers at once: its status moved, so its question or state line may have.
func (h *Hub) PaneChanged(machine, pane string) {
	key := Key(machine, pane)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.broadcastLocked(Event{Topic: "chat", Machine: machine, Pane: pane, Version: h.checks[key].version}, func(s *Subscription) bool { return s.panes[key] })
}

// Tick stats every watched pane whose turn it is and announces the ones that moved; Run calls it, tests call it by hand.
func (h *Hub) Tick(now time.Time) {
	if h.in.Version == nil {
		return
	}
	h.mu.Lock()
	due := map[string]bool{}
	for s := range h.subs {
		for key := range s.panes {
			if due[key] {
				continue
			}
			machine, _, _ := strings.Cut(key, "|")
			every := h.LocalEvery
			if h.in.Remote != nil && h.in.Remote(machine) {
				every = h.RemoteEvery
			}
			if last, seen := h.checks[key]; !seen || now.Sub(last.at) >= every {
				due[key] = true
			}
		}
	}
	h.mu.Unlock()
	keys := make([]string, 0, len(due))
	for key := range due {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		machine, pane, _ := strings.Cut(key, "|")
		version, ok := h.in.Version(machine, pane)
		if !ok {
			version = ""
		}
		h.mu.Lock()
		last, seen := h.checks[key]
		h.checks[key] = paneCheck{version: version, at: now}
		if seen && version != last.version {
			h.broadcastLocked(Event{Topic: "chat", Machine: machine, Pane: pane, Version: version}, func(s *Subscription) bool { return s.panes[key] })
		}
		h.mu.Unlock()
	}
	h.forgetUnwatchedLocked()
}

// forgetUnwatchedLocked drops the checks of panes nobody watches any more, so a pane closed long ago costs nothing.
func (h *Hub) forgetUnwatchedLocked() {
	h.mu.Lock()
	defer h.mu.Unlock()
	watched := map[string]bool{}
	for s := range h.subs {
		for key := range s.panes {
			watched[key] = true
		}
	}
	for key := range h.checks {
		if !watched[key] {
			delete(h.checks, key)
		}
	}
}

// Run ticks until ctx ends.
func (h *Hub) Run(ctx context.Context) {
	ticker := time.NewTicker(h.LocalEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			h.Tick(now)
		}
	}
}

// Close ends every stream: remuda is stopping.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	if h.pending != nil {
		h.pending.Stop()
		h.pending = nil
	}
	for s := range h.subs {
		s.closed = true
		close(s.ch)
		delete(h.subs, s)
	}
}

// broadcastLocked hands the event to every subscription the filter admits, dropping it for one whose queue is full.
func (h *Hub) broadcastLocked(ev Event, admit func(*Subscription) bool) {
	for s := range h.subs {
		if admit != nil && !admit(s) {
			continue
		}
		select {
		case s.ch <- ev:
		default:
		}
	}
}
