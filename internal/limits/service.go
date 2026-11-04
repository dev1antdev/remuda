package limits

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/state"
)

// Every is how often a machine's limits are read again; a sweep never runs twice at once on one machine.
var Every = 2 * time.Minute

// MaxAhead is how far ahead a booking to continue may be: a weekly window resets within a week.
const MaxAhead = 8 * 24 * time.Hour

const stateFile = "limits.json"

// Hit is an agent stopped on a subscription limit: when, and the words its harness wrote.
type Hit struct {
	At   time.Time `json:"at"`
	Text string    `json:"text,omitempty"`
}

// Booking is the person's word to continue a pane's agent at a moment, kept until it is said or called off.
type Booking struct {
	Machine string    `json:"machine"`
	Pane    string    `json:"pane"`
	Session string    `json:"session,omitempty"` // the conversation booked for: another one in the pane by then is left alone
	At      time.Time `json:"at"`
	Text    string    `json:"text"`
}

// Place is a pane as the watcher last saw it, what a booking checks before it speaks.
type Place struct {
	Agent     string // the agent herdr says runs there, "" for a shell
	Session   string
	Status    string
	Workspace string
}

// PaneLimit is what the roster says of a pane and the limits: a hit, a booking, or both.
type PaneLimit struct {
	HitAt      *time.Time `json:"hit_at,omitempty"`
	Text       string     `json:"text,omitempty"`
	ContinueAt *time.Time `json:"continue_at,omitempty"`
}

// Service holds every machine's limits, the warnings already given, the agents stopped on a limit and the bookings.
type Service struct {
	dir string
	now func() time.Time

	Notify  func(push.Payload)                       // the warnings, and what a booking did
	Changed func()                                   // something the roster carries moved
	Check   func(machine, pane string) (Hit, bool)   // whether the pane's agent stopped on a limit, from its transcript
	Submit  func(machine, pane, text string) error   // a booking's word, through the chat's own path
	Pane    func(machine, pane string) (Place, bool) // the pane as the watcher last saw it

	mu       sync.Mutex
	readings map[string]Reading
	sweeps   map[string]sweep
	hits     map[string]Hit
	checking map[string]bool
	seeded   map[string]bool
	kept     kept
}

type sweep struct {
	at   time.Time
	busy bool
}

// kept is what outlives a restart: the warnings given, by window and level, and the bookings, by machine|pane.
type kept struct {
	Alerted  map[string]int     `json:"alerted"`
	Bookings map[string]Booking `json:"bookings"`
}

// New loads the warnings and bookings kept in dir.
func New(dir string) *Service {
	s := &Service{dir: dir, now: time.Now, readings: map[string]Reading{}, sweeps: map[string]sweep{}, hits: map[string]Hit{}, checking: map[string]bool{}, seeded: map[string]bool{}}
	if err := state.LoadJSON(dir, stateFile, &s.kept); err != nil {
		log.Printf("%s is unreadable (%v): no limit warning is remembered as given, no booking kept", stateFile, err)
		s.kept = kept{}
	}
	if s.kept.Alerted == nil {
		s.kept.Alerted = map[string]int{}
	}
	if s.kept.Bookings == nil {
		s.kept.Bookings = map[string]Booking{}
	}
	return s
}

// Of is a machine's last reading.
func (s *Service) Of(machine string) Reading {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.readings[machine]
	r.Windows = append([]Window(nil), r.Windows...)
	return r
}

// PaneOf is what the roster says of a pane, nil when there is nothing to say.
func (s *Service) PaneOf(machine, pane string) *PaneLimit {
	key := machine + "|" + pane
	s.mu.Lock()
	defer s.mu.Unlock()
	hit, hasHit := s.hits[key]
	booking, booked := s.kept.Bookings[key]
	if !hasHit && !booked {
		return nil
	}
	out := &PaneLimit{}
	if hasHit {
		at := hit.At
		out.HitAt, out.Text = &at, hit.Text
	}
	if booked {
		at := booking.At
		out.ContinueAt = &at
	}
	return out
}

// Refresh reads a machine's limits in the background, at most every Every, never two at once.
func (s *Service) Refresh(host shell.Host, label string) {
	s.mu.Lock()
	sw := s.sweeps[host.ID()]
	if sw.busy || s.now().Sub(sw.at) < Every {
		s.mu.Unlock()
		return
	}
	s.sweeps[host.ID()] = sweep{at: s.now(), busy: true}
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			sw := s.sweeps[host.ID()]
			sw.busy = false
			s.sweeps[host.ID()] = sw
			s.mu.Unlock()
		}()
		s.Read(host, label)
	}()
}

// Read reads a machine's limits now and warns of a window that crossed 80 % or its limit.
func (s *Service) Read(host shell.Host, label string) {
	out, err := shell.Text(host, Script(), 30*time.Second)
	if err != nil {
		return
	}
	reading := Parse(out, s.now())
	s.mu.Lock()
	moved := fmt.Sprint(s.readings[host.ID()]) != fmt.Sprint(reading)
	s.readings[host.ID()] = reading
	s.mu.Unlock()
	s.warn(label, reading)
	if moved {
		s.changed()
	}
}

// warn pushes once per window and level: at 80 % and at the limit; the same window read on two machines warns once
func (s *Service) warn(label string, r Reading) {
	now := s.now()
	var due []push.Payload
	s.mu.Lock()
	for key := range s.kept.Alerted {
		if resets, ok := keyResets(key); ok && !resets.After(now) {
			delete(s.kept.Alerted, key)
		}
	}
	for _, w := range r.Windows {
		level := Level(w.Used)
		key := alertKey(w)
		if level == 0 || s.kept.Alerted[key] >= level {
			continue
		}
		s.kept.Alerted[key] = level
		due = append(due, Warning(w, level, label, now))
	}
	if len(due) > 0 {
		s.saveLocked()
	}
	s.mu.Unlock()
	for _, p := range due {
		s.notify(p)
	}
}

// Level is the warning a window's use earns: 100 at the limit, 80 from 80 %, else 0.
func Level(used float64) int {
	switch {
	case used >= 100:
		return 100
	case used >= 80:
		return 80
	}
	return 0
}

func alertKey(w Window) string {
	return fmt.Sprintf("%s|%s|%d", w.Agent, w.Name, w.ResetsAt.Unix()/60)
}

// keyResets is when the window a warning key names resets, read off the key's last field
func keyResets(key string) (time.Time, bool) {
	i := strings.LastIndexByte(key, '|')
	if i < 0 {
		return time.Time{}, false
	}
	minute, err := strconv.ParseInt(key[i+1:], 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(minute*60, 0), true
}

// Observe hears every snapshot: an agent pane first seen at rest is checked once for a limit it stopped on.
func (s *Service) Observe(machine string, snap *herdr.Snapshot) {
	if snap == nil {
		return
	}
	for i := range snap.Panes {
		p := &snap.Panes[i]
		key := machine + "|" + p.PaneID
		s.mu.Lock()
		seen := s.seeded[key]
		s.seeded[key] = true
		s.mu.Unlock()
		if !seen && p.Agent == "claude" && p.AgentStatus != herdr.StatusWorking {
			s.check(machine, p.PaneID)
		}
	}
}

// OnStatus hears every pane's move: work again clears a hit, a close forgets the pane and its booking, a rest checks.
func (s *Service) OnStatus(e events.PaneStatus) {
	key := e.Pane.String()
	switch {
	case e.To == events.Gone:
		s.mu.Lock()
		_, hit := s.hits[key]
		_, booked := s.kept.Bookings[key]
		delete(s.hits, key)
		delete(s.seeded, key)
		if booked {
			delete(s.kept.Bookings, key)
			s.saveLocked()
		}
		s.mu.Unlock()
		if hit || booked {
			s.changed()
		}
	case e.To == herdr.StatusWorking:
		s.mu.Lock()
		_, hit := s.hits[key]
		delete(s.hits, key)
		s.mu.Unlock()
		if hit {
			s.changed()
		}
	case e.Kind == "claude":
		s.check(e.Pane.Machine, e.Pane.Pane)
	}
}

// check reads the pane's transcript for a limit hit, off the caller's path, one read per pane at a time
func (s *Service) check(machine, pane string) {
	if s.Check == nil {
		return
	}
	key := machine + "|" + pane
	s.mu.Lock()
	if s.checking[key] {
		s.mu.Unlock()
		return
	}
	s.checking[key] = true
	s.mu.Unlock()
	go func() {
		hit, ok := s.Check(machine, pane)
		s.mu.Lock()
		delete(s.checking, key)
		before, had := s.hits[key]
		if ok {
			s.hits[key] = hit
		} else {
			delete(s.hits, key)
		}
		moved := had != ok || before != hit
		s.mu.Unlock()
		if moved {
			s.changed()
		}
	}()
}

// Book keeps the person's word to say text to the pane's agent at a moment ahead.
func (s *Service) Book(machine, pane string, at time.Time, text string) error {
	now := s.now()
	if !at.After(now) {
		return fmt.Errorf("that moment has passed")
	}
	if at.After(now.Add(MaxAhead)) {
		return fmt.Errorf("a booking can be at most a week ahead")
	}
	if strings.TrimSpace(text) == "" {
		text = "continue"
	}
	place, ok := s.place(machine, pane)
	if !ok {
		return fmt.Errorf("no such pane on that machine")
	}
	if place.Agent == "" {
		return fmt.Errorf("no agent runs in that pane")
	}
	s.mu.Lock()
	s.kept.Bookings[machine+"|"+pane] = Booking{Machine: machine, Pane: pane, Session: place.Session, At: at.UTC(), Text: text}
	err := s.saveLocked()
	s.mu.Unlock()
	s.changed()
	return err
}

// Cancel calls a booking off.
func (s *Service) Cancel(machine, pane string) error {
	s.mu.Lock()
	key := machine + "|" + pane
	_, booked := s.kept.Bookings[key]
	delete(s.kept.Bookings, key)
	var err error
	if booked {
		err = s.saveLocked()
	}
	s.mu.Unlock()
	if booked {
		s.changed()
	}
	return err
}

// Loop says the bookings that are due, every interval, until done.
func (s *Service) Loop(every time.Duration, done <-chan struct{}) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			s.Tick()
		}
	}
}

// Tick says every booking that is due: to the same conversation, resting and asking nothing; anything else calls it off.
func (s *Service) Tick() {
	now := s.now()
	var due []Booking
	s.mu.Lock()
	for key, b := range s.kept.Bookings {
		if !b.At.After(now) {
			due = append(due, b)
			delete(s.kept.Bookings, key)
		}
	}
	if len(due) > 0 {
		s.saveLocked()
	}
	s.mu.Unlock()
	for _, b := range due {
		s.say(b)
	}
	if len(due) > 0 {
		s.changed()
	}
}

func (s *Service) say(b Booking) {
	place, ok := s.place(b.Machine, b.Pane)
	switch {
	case !ok:
		s.notify(push.Payload{Machine: b.Machine, Kind: "limits", Title: "Nothing continued after the limit reset", Body: "The pane closed before its booking came due."})
		return
	case b.Session != "" && place.Session != b.Session:
		s.notify(push.Payload{Machine: b.Machine, Pane: b.Pane, Kind: "limits", Title: "Nothing continued in " + place.Workspace, Body: "Another conversation runs there now than the one the booking was for."})
		return
	case place.Status == herdr.StatusWorking:
		return // somebody has it going again already
	case place.Status == herdr.StatusBlocked:
		s.notify(push.Payload{Machine: b.Machine, Pane: b.Pane, Kind: "limits", Title: "Nothing continued in " + place.Workspace, Body: "It waits on a question; the booking's word would land in it."})
		return
	}
	if s.Submit == nil {
		return
	}
	if err := s.Submit(b.Machine, b.Pane, b.Text); err != nil {
		s.notify(push.Payload{Machine: b.Machine, Pane: b.Pane, Kind: "limits", Title: "Could not continue " + place.Workspace, Body: err.Error()})
		return
	}
	s.notify(push.Payload{Machine: b.Machine, Pane: b.Pane, Kind: "limits", Title: "Continued " + place.Workspace + " after the limit reset", Body: "Sent «" + b.Text + "», as booked."})
}

// Forget drops a removed machine: its reading, its agents' hits and their bookings.
func (s *Service) Forget(machine string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.readings, machine)
	delete(s.sweeps, machine)
	prefix := machine + "|"
	for key := range s.hits {
		if strings.HasPrefix(key, prefix) {
			delete(s.hits, key)
		}
	}
	for key := range s.seeded {
		if strings.HasPrefix(key, prefix) {
			delete(s.seeded, key)
		}
	}
	removed := false
	for key := range s.kept.Bookings {
		if strings.HasPrefix(key, prefix) {
			delete(s.kept.Bookings, key)
			removed = true
		}
	}
	if removed {
		s.saveLocked()
	}
}

func (s *Service) place(machine, pane string) (Place, bool) {
	if s.Pane == nil {
		return Place{}, false
	}
	return s.Pane(machine, pane)
}

func (s *Service) notify(p push.Payload) {
	if s.Notify != nil {
		s.Notify(p)
	}
}

func (s *Service) changed() {
	if s.Changed != nil {
		s.Changed()
	}
}

// saveLocked writes the warnings and bookings; the caller holds mu
func (s *Service) saveLocked() error {
	if s.dir == "" {
		return nil
	}
	if err := state.SaveJSON(s.dir, stateFile, s.kept); err != nil {
		log.Printf("%s: %v", stateFile, err)
		return fmt.Errorf("the booking could not be kept: %w", err)
	}
	return nil
}
