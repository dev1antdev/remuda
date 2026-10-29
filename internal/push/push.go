// Package push delivers Web Push alerts ("needs you" / "finished") to the phone with
// the app closed: RFC 8291/8292, self-generated VAPID keys, payloads encrypted to each
// browser's subscription and sent straight to the vendor's endpoint, no service between.
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
	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/state"
)

// Prefs are per-device: alert on a question, on a finish never, only after a long turn (≥60s), or always,
// and quiet hours during which nothing is sent at all.
type Prefs struct {
	Watchdog *bool  `json:"watchdog,omitempty"` // a stuck or long-waiting agent; nil = on
	Input    bool   `json:"input"`
	Done     string `json:"done"` // off | long | always
	Quiet    *Quiet `json:"quiet,omitempty"`
}

// Normalize fills the default, rejects unknown values and drops a quiet window that does not parse.
func (p *Prefs) Normalize() {
	switch p.Done {
	case "off", "long", "always":
	default:
		p.Done = "long"
	}
	if p.Quiet != nil && !p.Quiet.Valid() {
		p.Quiet = nil
	}
}

type Subscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	Prefs   Prefs     `json:"prefs"`
	Label   string    `json:"label,omitempty"`
	Created time.Time `json:"created"`
}

type Payload struct {
	Machine string   `json:"machine"`
	Pane    string   `json:"pane"`
	Title   string   `json:"title"`
	Body    string   `json:"body"`
	Tag     string   `json:"tag"`
	Kind    string   `json:"kind"` // input | done | test | stuck | waiting | loop | schedule | digest | task | budget | review
	Actions []Action `json:"actions,omitempty"`
	Panel   string   `json:"panel,omitempty"`   // a panel of the page a tap opens instead of a pane (standup)
	Choices []string `json:"choices,omitempty"` // a question's choices in full, the words behind its choice-N actions
}

// Action is a button on the notification; the service worker performs it.
type Action struct {
	Action string `json:"action"` // open | esc | choice-0 | choice-1
	Title  string `json:"title"`
}

type Service struct {
	dir     string
	pub     string
	priv    string
	subject string

	mu   sync.Mutex
	subs map[string]*Subscription // by endpoint

	timers map[events.PaneRef][]*time.Timer
	send   func(sub *Subscription, p Payload) error

	// OnAlert hears every alert as it goes out, or would have — the activity feed, whether or not a device is subscribed.
	OnAlert func(p Payload)
	// Question reads the dialog off a pane's screen when a question push goes out, so its choices ride as buttons.
	Question func(machine, pane string) (Asked, bool)
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
	s := &Service{dir: dir, subs: map[string]*Subscription{}, timers: map[events.PaneRef][]*time.Timer{},
		subject: "https://github.com/dev1antdev/remuda"}
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
		// a damaged file is an error, never a silent rotation: a new key orphans every subscribed device
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
	if err := json.Unmarshal(b, &list); err != nil {
		log.Printf("push-subscriptions.json is unreadable (%v): every device must enable alerts again", err)
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

// Subscribe registers or updates a device; oldEndpoint, when the browser rotated the subscription, carries the old prefs over.
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

// Send pushes one payload to one subscription; a 404/410 from the push service means the browser dropped it.
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

// topic collapses pending pushes for the same pane; the Topic header allows 32 URL-safe chars.
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

// Notify sends one payload now to every device that wants the watchdog's word (the pref defaults to on).
func (s *Service) Notify(p Payload) {
	s.record(p)
	var targets []*Subscription
	for _, sub := range s.list() {
		if sub.Prefs.Watchdog != nil && !*sub.Prefs.Watchdog {
			continue
		}
		targets = append(targets, sub)
	}
	s.deliver(targets, p)
}

// Broadcast sends one payload now to every device, whatever its watchdog pref: a schedule's word is the person's own.
func (s *Service) Broadcast(p Payload) {
	s.record(p)
	s.deliver(s.list(), p)
}

// deliver sends one payload to each device, skipping the ones inside their quiet hours right now.
func (s *Service) deliver(targets []*Subscription, p Payload) {
	now := time.Now()
	for _, sub := range targets {
		if muted(sub, now) {
			continue
		}
		if err := s.send(sub, p); err != nil {
			log.Printf("push to %.40s…: %v", sub.Endpoint, err)
		}
	}
}

// record hands an alert to the feed, when there is one.
func (s *Service) record(p Payload) {
	if s.OnAlert != nil {
		s.OnAlert(p)
	}
}

// SendTest proves the whole path for one device.
func (s *Service) SendTest(endpoint string) error {
	s.mu.Lock()
	sub := s.subs[endpoint]
	s.mu.Unlock()
	if sub == nil {
		return fmt.Errorf("unknown subscription")
	}
	return s.Send(sub, Payload{Title: "remuda", Body: "Alerts reach this device.", Tag: "test", Kind: "test"})
}

// OnStatus schedules or calls off alerts: any status change on a pane cancels whatever was pending for it.
// herdr says nothing about whether a person is at the desk; "changed that fast" is the proxy for presence.
func (s *Service) OnStatus(ev events.PaneStatus) {
	key := ev.Pane
	// cancel and schedule under one lock: two changes of the same pane arriving together must not leave a timer the later one meant to kill
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.timers[key] {
		t.Stop()
	}
	delete(s.timers, key)

	subs := make([]*Subscription, 0, len(s.subs))
	for _, sub := range s.subs {
		subs = append(subs, sub)
	}
	if len(subs) == 0 && s.OnAlert == nil {
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
	// note: the feed hears this alert when it fires, so a finish or a question answered at the desk is no news either;
	// dress reads what the pane shows at that moment into the payload (nil: sent as built)
	schedule := func(delay time.Duration, targets []*Subscription, p Payload, note bool, dress func(Payload) Payload) {
		if len(targets) == 0 && !(note && s.OnAlert != nil) {
			return
		}
		var t *time.Timer
		t = time.AfterFunc(delay, func() {
			if dress != nil {
				p = dress(p)
			}
			// drop only this timer: a sibling (the other device group's delay) must stay cancellable by a later status change
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
			if note {
				s.record(p)
			}
			s.deliver(targets, p)
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
		}, true, s.asked)
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
		schedule(doneDelay, long, p, false, nil)
		schedule(doneDelayFast, always, p, true, nil)
	default:
		return
	}
	if len(pending) > 0 {
		s.timers[key] = pending
	}
}

func paneLine(ev events.PaneStatus) string {
	if ev.PaneTitle != "" && ev.PaneTitle != ev.Agent {
		return " · " + ev.PaneTitle
	}
	return ""
}
