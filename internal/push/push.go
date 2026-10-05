// Package push delivers Web Push alerts ("needs you" / "finished") to the
// phone with the app closed. Standard RFC 8291/8292: self-generated VAPID
// keys, payloads encrypted to each browser's subscription, sent straight to
// the browser vendor's push endpoint. No third-party service in between.
package push

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/dev1antdev/herdr-ui/internal/herdr"
	"github.com/dev1antdev/herdr-ui/internal/state"
)

// Prefs are per-device: alert when an agent waits for an answer, and when it
// finishes a turn — never, only long turns (≥60s), or always.
type Prefs struct {
	Input bool   `json:"input"`
	Done  string `json:"done"` // off | long | always
}

// Normalize fills the default and rejects unknown values.
func (p *Prefs) Normalize() {
	switch p.Done {
	case "off", "long", "always":
	default:
		p.Done = "long"
	}
}

type Subscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	Prefs   Prefs     `json:"prefs"`
	Label   string    `json:"label,omitempty"` // user agent, for a future devices list
	Created time.Time `json:"created"`
}

type Payload struct {
	Machine string `json:"machine"`
	Pane    string `json:"pane"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Tag     string `json:"tag"`
	Kind    string `json:"kind"` // input | done | test
}

// PaneRef identifies a pane across machines.
type PaneRef struct{ Machine, Pane string }

func (r PaneRef) String() string { return r.Machine + "|" + r.Pane }

type Service struct {
	dir     string
	pub     string
	priv    string
	subject string

	mu   sync.Mutex
	subs map[string]*Subscription // by endpoint

	timers map[PaneRef][]*time.Timer // pending alerts
	send   func(sub *Subscription, p Payload) error
}

// Delays are variables so tests can shrink them.
var (
	inputDelay    = 10 * time.Second // a question answered at the desk within this never reaches the phone
	doneDelay     = 60 * time.Second
	doneDelayFast = 10 * time.Second
	longTurn      = 60 * time.Second
)

// New loads or creates the VAPID keypair and the subscription store in dir.
func New(dir string) (*Service, error) {
	s := &Service{dir: dir, subs: map[string]*Subscription{}, timers: map[PaneRef][]*time.Timer{},
		subject: "https://github.com/dev1antdev/herdr-ui"}
	s.send = s.Send
	if err := s.loadKeys(); err != nil {
		return nil, err
	}
	s.loadSubs()
	return s, nil
}

func (s *Service) loadKeys() error {
	path := filepath.Join(s.dir, "vapid.json")
	var k struct{ Public, Private string }
	if b, err := os.ReadFile(path); err == nil {
		// A malformed file is an error, never a silent rotation: rotating
		// the key orphans every subscribed device.
		if err := json.Unmarshal(b, &k); err != nil || k.Public == "" || k.Private == "" {
			return fmt.Errorf("%s is damaged; move it away to generate new keys (every device must then re-enable alerts)", path)
		}
		s.pub, s.priv = k.Public, k.Private
		return nil
	}
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return err
	}
	k.Public, k.Private = pub, priv
	b, _ := json.MarshalIndent(k, "", "  ")
	if err := state.WriteFile(path, b); err != nil {
		return err
	}
	s.pub, s.priv = pub, priv
	return nil
}

func (s *Service) loadSubs() {
	b, err := os.ReadFile(filepath.Join(s.dir, "push-subscriptions.json"))
	if err != nil {
		return
	}
	var list []*Subscription
	if json.Unmarshal(b, &list) != nil {
		return
	}
	for _, sub := range list {
		if sub.Endpoint != "" {
			sub.Prefs.Normalize()
			s.subs[sub.Endpoint] = sub
		}
	}
}

// list snapshots the subscriptions under the lock.
func (s *Service) list() []*Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Subscription, 0, len(s.subs))
	for _, sub := range s.subs {
		out = append(out, sub)
	}
	return out
}

func (s *Service) saveSubs() {
	b, _ := json.MarshalIndent(s.list(), "", "  ")
	if err := state.WriteFile(filepath.Join(s.dir, "push-subscriptions.json"), b); err != nil {
		log.Printf("push: save subscriptions: %v", err)
	}
}

func (s *Service) PublicKey() string { return s.pub }

// Subscribe registers or updates a device. oldEndpoint, when the browser
// rotated the subscription, carries the previous device's prefs over.
func (s *Service) Subscribe(sub Subscription, oldEndpoint string) {
	sub.Prefs.Normalize()
	s.mu.Lock()
	prev := s.subs[sub.Endpoint]
	if prev == nil && oldEndpoint != "" {
		if prev = s.subs[oldEndpoint]; prev != nil {
			sub.Prefs = prev.Prefs
			if sub.Label == "" {
				sub.Label = prev.Label
			}
			delete(s.subs, oldEndpoint)
		}
	}
	if prev != nil {
		sub.Created = prev.Created
	}
	if sub.Created.IsZero() {
		sub.Created = time.Now()
	}
	unchanged := prev != nil && prev.Endpoint == sub.Endpoint && reflect.DeepEqual(*prev, sub)
	s.subs[sub.Endpoint] = &sub
	s.mu.Unlock()
	if !unchanged { // every page load re-registers; don't rewrite the file for nothing
		s.saveSubs()
	}
}

func (s *Service) Unsubscribe(endpoint string) {
	s.mu.Lock()
	delete(s.subs, endpoint)
	s.mu.Unlock()
	s.saveSubs()
}

func (s *Service) Subscriptions() []Subscription {
	subs := s.list()
	out := make([]Subscription, 0, len(subs))
	for _, sub := range subs {
		out = append(out, *sub)
	}
	return out
}

// Send pushes one payload to one subscription. A 404/410 from the push
// service means the browser dropped the subscription: forget it.
func (s *Service) Send(sub *Subscription, p Payload) error {
	body, _ := json.Marshal(p)
	urgency := webpush.UrgencyNormal
	if p.Kind == "input" {
		urgency = webpush.UrgencyHigh
	}
	resp, err := webpush.SendNotification(body, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{Auth: sub.Keys.Auth, P256dh: sub.Keys.P256dh},
	}, &webpush.Options{
		HTTPClient:      &http.Client{Timeout: 10 * time.Second},
		Subscriber:      s.subject,
		VAPIDPublicKey:  s.pub,
		VAPIDPrivateKey: s.priv,
		TTL:             12 * 3600,
		Urgency:         urgency,
		Topic:           topic(p.Tag),
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == 404 || resp.StatusCode == 410:
		s.Unsubscribe(sub.Endpoint)
		return fmt.Errorf("subscription gone (%d)", resp.StatusCode)
	case resp.StatusCode >= 300:
		return fmt.Errorf("push service answered %d", resp.StatusCode)
	}
	return nil
}

// topic collapses pending pushes for the same pane: the Topic header allows
// 32 URL-safe chars.
func topic(tag string) string {
	const ok = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
	t := strings.Map(func(r rune) rune {
		if r < 128 && strings.IndexByte(ok, byte(r)) >= 0 {
			return r
		}
		return '_'
	}, tag)
	if len(t) > 32 {
		t = t[:32]
	}
	return t
}

// SendTest proves the whole path for one device.
func (s *Service) SendTest(endpoint string) error {
	s.mu.Lock()
	sub := s.subs[endpoint]
	s.mu.Unlock()
	if sub == nil {
		return fmt.Errorf("unknown subscription")
	}
	return s.Send(sub, Payload{Title: "herdr-ui", Body: "Alerts reach this device.", Tag: "test", Kind: "test"})
}

// Event is a pane's status transition as the watcher saw it.
type Event struct {
	Pane      PaneRef
	Workspace string // label for the title
	PaneTitle string
	Agent     string
	From, To  string        // herdr statuses; To "gone" when the pane closed
	Worked    time.Duration // how long the pane was working before this
}

// OnStatus schedules or calls off alerts. Any change on a pane cancels
// whatever was pending for it: a prompt answered at the desk, or a finish
// immediately followed by the next prompt, never reaches the phone. herdr
// says nothing about whether a human is at the PC, so "changed that fast"
// is the proxy for presence.
func (s *Service) OnStatus(ev Event) {
	key := ev.Pane
	s.mu.Lock()
	for _, t := range s.timers[key] {
		t.Stop()
	}
	delete(s.timers, key)
	s.mu.Unlock()

	subs := s.list()
	if len(subs) == 0 {
		return
	}

	label := ev.Workspace
	if label == "" {
		label = ev.Pane.Pane
	}
	agent := ev.Agent
	if agent == "" {
		agent = "agent"
	}

	var pending []*time.Timer
	schedule := func(delay time.Duration, targets []*Subscription, p Payload) {
		if len(targets) == 0 {
			return
		}
		var t *time.Timer
		t = time.AfterFunc(delay, func() {
			// drop only this timer: a sibling (the other device group's
			// delay) must stay cancellable by a later status change
			s.mu.Lock()
			rest := s.timers[key][:0]
			for _, o := range s.timers[key] {
				if o != t {
					rest = append(rest, o)
				}
			}
			if len(rest) == 0 {
				delete(s.timers, key)
			} else {
				s.timers[key] = rest
			}
			s.mu.Unlock()
			for _, sub := range targets {
				if err := s.send(sub, p); err != nil {
					log.Printf("push to %.40s…: %v", sub.Endpoint, err)
				}
			}
		})
		pending = append(pending, t)
	}

	switch ev.To {
	case herdr.StatusBlocked:
		var targets []*Subscription
		for _, sub := range subs {
			if sub.Prefs.Input {
				targets = append(targets, sub)
			}
		}
		schedule(inputDelay, targets, Payload{
			Machine: ev.Pane.Machine, Pane: ev.Pane.Pane, Kind: "input",
			Title: label + " needs you",
			Body:  agent + " is waiting for an answer" + paneLine(ev),
			Tag:   key.String(),
		})
	case herdr.StatusDone:
		if ev.From != herdr.StatusWorking {
			return
		}
		var long, always []*Subscription
		for _, sub := range subs {
			switch sub.Prefs.Done {
			case "always":
				always = append(always, sub)
			case "long":
				if ev.Worked >= longTurn {
					long = append(long, sub)
				}
			}
		}
		p := Payload{
			Machine: ev.Pane.Machine, Pane: ev.Pane.Pane, Kind: "done",
			Title: label + " finished",
			Body:  agent + " is done after " + ev.Worked.Round(time.Second).String() + paneLine(ev),
			Tag:   key.String(),
		}
		schedule(doneDelay, long, p)
		schedule(doneDelayFast, always, p)
	default:
		return
	}
	if len(pending) > 0 {
		s.mu.Lock()
		s.timers[key] = pending
		s.mu.Unlock()
	}
}

func paneLine(ev Event) string {
	if ev.PaneTitle != "" && ev.PaneTitle != ev.Agent {
		return " · " + ev.PaneTitle
	}
	return ""
}
