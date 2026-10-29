package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/state"
)

// Webhooks are the places the person named for the alerts beside their devices: a Telegram bot, a Slack
// incoming webhook, an ntfy topic or any URL that takes JSON. Every alert the activity feed records goes to
// each enabled webhook whose kinds include it, so a phone without Web Push hears the agents too. The person
// sets every address; remuda calls nothing it was not given.

const webhooksFile = "webhooks.json"

// HookKinds are the kinds a webhook can take, as the panel offers them; HookKind maps a payload's kind to one.
var HookKinds = []string{"question", "finish", "watchdog", "digest", "schedule", "task", "budget", "review"}

// WebhookKinds are the kinds of webhook.
var WebhookKinds = []string{"telegram", "slack", "ntfy", "generic"}

const (
	webhookTimeout = 10 * time.Second
	webhookLogGap  = time.Hour
	maxWebhooks    = 20
	maxHookName    = 80
	maxAnswer      = 120
)

// Variables so a test can point the Bot API at a server of its own and skip the pause before the second try.
var (
	telegramAPI  = "https://api.telegram.org"
	webhookRetry = 2 * time.Second
)

// Webhook is one place alerts go.
type Webhook struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"` // telegram | slack | ntfy | generic
	Name    string   `json:"name"`
	URL     string   `json:"url,omitempty"`   // slack, ntfy, generic: the address
	Token   string   `json:"token,omitempty"` // telegram: the bot token
	Chat    string   `json:"chat,omitempty"`  // telegram: the chat id
	Kinds   []string `json:"kinds"`
	Enabled bool     `json:"enabled"`
	Quiet   *Quiet   `json:"quiet,omitempty"`
}

// WebhookView is a webhook as the page sees it: the secrets masked, the last delivery's outcome beside.
type WebhookView struct {
	Webhook
	HasToken    bool       `json:"has_token"`
	LastOK      *time.Time `json:"last_ok,omitempty"`
	LastError   string     `json:"last_error,omitempty"`
	LastErrorAt *time.Time `json:"last_error_at,omitempty"`
}

// Message is what a generic webhook receives, and what the other kinds are written from.
type Message struct {
	Kind    string    `json:"kind"`
	Title   string    `json:"title"`
	Body    string    `json:"body"`
	Machine string    `json:"machine,omitempty"`
	Pane    string    `json:"pane,omitempty"`
	Panel   string    `json:"panel,omitempty"`
	Link    string    `json:"link,omitempty"`
	At      time.Time `json:"at"`
}

type webhookFile struct {
	Base  string    `json:"base"`
	Hooks []Webhook `json:"hooks"`
}

// outcome is how the last sends to one webhook went, kept in memory only: the file is the person's own settings.
type outcome struct {
	ok     time.Time
	err    string
	at     time.Time
	logged time.Time
}

// Webhooks is the store and the sender, loaded by the push service from its state dir.
type Webhooks struct {
	dir    string
	client *http.Client
	now    func() time.Time

	mu       sync.Mutex
	base     string
	hooks    []Webhook
	outcomes map[string]*outcome
	sending  sync.WaitGroup
}

func loadWebhooks(dir string) *Webhooks {
	w := &Webhooks{dir: dir, client: &http.Client{Timeout: webhookTimeout}, now: time.Now, outcomes: map[string]*outcome{}}
	var f webhookFile
	if err := state.LoadJSON(dir, webhooksFile, &f); err != nil {
		log.Printf("%s is unreadable (%v): no webhook gets an alert until it is set again", webhooksFile, err)
		return w
	}
	w.base, w.hooks = f.Base, f.Hooks
	return w
}

func (w *Webhooks) save() error {
	return state.SaveJSON(w.dir, webhooksFile, webhookFile{Base: w.base, Hooks: w.hooks})
}

// Any says whether an enabled webhook exists at all.
func (w *Webhooks) Any() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, h := range w.hooks {
		if h.Enabled {
			return true
		}
	}
	return false
}

// Wants says whether an enabled webhook takes alerts of a payload's kind right now.
func (w *Webhooks) Wants(kind string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.takers(kind, w.now())) > 0
}

// takers are the enabled webhooks that take the kind and are not in their quiet hours; under the lock.
func (w *Webhooks) takers(kind string, now time.Time) []Webhook {
	want := HookKind(kind)
	var out []Webhook
	for _, h := range w.hooks {
		if !h.Enabled || !has(h.Kinds, want) || (h.Quiet != nil && h.Quiet.Covers(now)) {
			continue
		}
		out = append(out, h)
	}
	return out
}

// Deliver sends one alert to every webhook that takes its kind, each in the background.
func (w *Webhooks) Deliver(p Payload) {
	w.mu.Lock()
	hooks := w.takers(p.Kind, w.now())
	base := w.base
	w.mu.Unlock()
	for _, h := range hooks {
		w.sending.Add(1)
		go func(h Webhook) {
			defer w.sending.Done()
			w.send(h, p, base)
		}(h)
	}
}

// Wait blocks until every send in flight is done; for tests.
func (w *Webhooks) Wait() { w.sending.Wait() }

// Test sends one test alert to a webhook, whatever its kinds or quiet hours, and answers how it went.
func (w *Webhooks) Test(id string) error {
	w.mu.Lock()
	h, ok := w.find(id)
	base := w.base
	w.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown webhook")
	}
	return w.send(h, Payload{Title: "remuda", Body: "Alerts reach this webhook.", Tag: "test", Kind: "test"}, base)
}

func (w *Webhooks) find(id string) (Webhook, bool) {
	for _, h := range w.hooks {
		if h.ID == id {
			return h, true
		}
	}
	return Webhook{}, false
}

// Views is the base URL and every webhook with its secrets masked.
func (w *Webhooks) Views() (string, []WebhookView) {
	w.mu.Lock()
	defer w.mu.Unlock()
	views := make([]WebhookView, 0, len(w.hooks))
	for _, h := range w.hooks {
		v := WebhookView{Webhook: h, HasToken: h.Token != ""}
		v.Token = ""
		v.URL = Masked(h.URL)
		if o := w.outcomes[h.ID]; o != nil {
			if !o.ok.IsZero() {
				at := o.ok
				v.LastOK = &at
			}
			if o.err != "" {
				at := o.at
				v.LastError, v.LastErrorAt = o.err, &at
			}
		}
		views = append(views, v)
	}
	return w.base, views
}

// Replace sets the base URL and the whole list: a webhook with a known id keeps its stored token, and its URL
// when the masked one comes back; a new one gets an id. A list that does not pass leaves the store as it was.
func (w *Webhooks) Replace(base string, incoming []Webhook) error {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base != "" && !httpURL(base) {
		return fmt.Errorf("the page's address must start with http:// or https://")
	}
	if len(incoming) > maxWebhooks {
		return fmt.Errorf("at most %d webhooks", maxWebhooks)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	hooks := make([]Webhook, 0, len(incoming))
	seen := map[string]bool{}
	for _, h := range incoming {
		if prev, ok := w.find(h.ID); ok {
			if h.Token == "" {
				h.Token = prev.Token
			}
			if h.URL == "" || h.URL == Masked(prev.URL) {
				h.URL = prev.URL
			}
		} else {
			h.ID = state.NewID()
		}
		if seen[h.ID] {
			return fmt.Errorf("webhook %s is listed twice", h.ID)
		}
		seen[h.ID] = true
		if err := check(&h); err != nil {
			return err
		}
		hooks = append(hooks, h)
	}
	w.base, w.hooks = base, hooks
	for id := range w.outcomes {
		if !seen[id] {
			delete(w.outcomes, id)
		}
	}
	return w.save()
}

// check refuses a webhook the sender could not use and tidies what it can.
func check(h *Webhook) error {
	if !has(WebhookKinds, h.Kind) {
		return fmt.Errorf("unknown webhook kind %q", h.Kind)
	}
	h.Name = strings.TrimSpace(h.Name)
	if h.Name == "" {
		h.Name = strings.ToUpper(h.Kind[:1]) + h.Kind[1:]
	}
	if len(h.Name) > maxHookName {
		return fmt.Errorf("a webhook's name is at most %d characters", maxHookName)
	}
	h.URL, h.Token, h.Chat = strings.TrimSpace(h.URL), strings.TrimSpace(h.Token), strings.TrimSpace(h.Chat)
	switch h.Kind {
	case "telegram":
		if h.Token == "" {
			return fmt.Errorf("%s: a Telegram webhook needs the bot's token", h.Name)
		}
		if h.Chat == "" {
			return fmt.Errorf("%s: a Telegram webhook needs the chat id", h.Name)
		}
		h.URL = ""
	default:
		if !httpURL(h.URL) {
			return fmt.Errorf("%s: the address must start with http:// or https://", h.Name)
		}
		h.Token, h.Chat = "", ""
	}
	var kinds []string
	for _, k := range h.Kinds {
		if has(HookKinds, k) && !has(kinds, k) {
			kinds = append(kinds, k)
		}
	}
	h.Kinds = kinds
	if h.Quiet != nil && !h.Quiet.Valid() {
		h.Quiet = nil
	}
	return nil
}

func httpURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// HookKind is the panel's kind for a payload's: a question, a finish, the watchdog's three notes, the rest by name.
func HookKind(kind string) string {
	switch kind {
	case "input":
		return "question"
	case "done":
		return "finish"
	case "stuck", "waiting", "loop":
		return "watchdog"
	}
	return kind
}

// Masked is an address with everything past its host hidden: a Slack webhook's path is its secret.
func Masked(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "…"
	}
	return u.Scheme + "://" + u.Host + "/…"
}

// Link is the page's address for an alert: the pane, else the panel, else the page itself; nothing without a base.
func Link(base string, p Payload) string {
	if base == "" {
		return ""
	}
	switch {
	case p.Pane != "":
		return base + "/?machine=" + url.QueryEscape(p.Machine) + "&pane=" + url.QueryEscape(p.Pane)
	case p.Panel != "":
		return base + "/?panel=" + url.QueryEscape(p.Panel)
	}
	return base
}

// messageOf is the alert as a generic webhook receives it.
func messageOf(p Payload, base string, at time.Time) Message {
	return Message{Kind: p.Kind, Title: p.Title, Body: p.Body, Machine: p.Machine, Pane: p.Pane, Panel: p.Panel, Link: Link(base, p), At: at}
}

// text is the alert as one piece of text: the title, the body, the link on lines of their own.
func (m Message) text() string {
	lines := []string{m.Title}
	if m.Body != "" {
		lines = append(lines, m.Body)
	}
	if m.Link != "" {
		lines = append(lines, m.Link)
	}
	return strings.Join(lines, "\n")
}

// request builds the call a webhook of each kind takes.
func request(ctx context.Context, h Webhook, m Message) (*http.Request, error) {
	switch h.Kind {
	case "telegram":
		body, _ := json.Marshal(map[string]any{"chat_id": h.Chat, "text": m.text(), "disable_web_page_preview": true})
		return jsonRequest(ctx, telegramAPI+"/bot"+h.Token+"/sendMessage", body)
	case "slack":
		body, _ := json.Marshal(map[string]string{"text": m.text()})
		return jsonRequest(ctx, h.URL, body)
	case "ntfy":
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, strings.NewReader(m.Body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "text/plain; charset=utf-8")
		req.Header.Set("Title", headerText(m.Title))
		if m.Link != "" {
			req.Header.Set("Click", m.Link)
		}
		if m.Kind == "input" {
			req.Header.Set("Priority", "high")
		}
		return req, nil
	default:
		body, _ := json.Marshal(m)
		return jsonRequest(ctx, h.URL, body)
	}
}

func jsonRequest(ctx context.Context, address string, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// headerText is a header value a server reads back whole: plain when ASCII, RFC 2047 encoded otherwise (ntfy decodes it).
func headerText(s string) string {
	for _, r := range s {
		if r > 126 || r < 32 {
			return mime.BEncoding.Encode("utf-8", s)
		}
	}
	return s
}

// send makes one attempt and one more after a moment on a network error or a server-side failure, then notes how it went.
func (w *Webhooks) send(h Webhook, p Payload, base string) error {
	m := messageOf(p, base, w.now())
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(webhookRetry)
		}
		var again bool
		err, again = w.attempt(h, m)
		if err == nil || !again {
			break
		}
	}
	w.note(h, err)
	return err
}

// attempt is one call; again says whether a second one is worth making.
func (w *Webhooks) attempt(h Webhook, m Message) (error, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), webhookTimeout)
	defer cancel()
	req, err := request(ctx, h, m)
	if err != nil {
		return plainError(err), false
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return plainError(err), true
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		answer, _ := io.ReadAll(io.LimitReader(resp.Body, maxAnswer))
		text := strings.TrimSpace(string(answer))
		if text != "" {
			text = ": " + text
		}
		return fmt.Errorf("answered %d%s", resp.StatusCode, text), resp.StatusCode >= 500
	}
	return nil, false
}

// plainError is a transport error without the address it names: the URL may hold the token.
func plainError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

// note keeps the outcome for the panel and logs a failure, once an hour per webhook, never with the address.
func (w *Webhooks) note(h Webhook, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	o := w.outcomes[h.ID]
	if o == nil {
		o = &outcome{}
		w.outcomes[h.ID] = o
	}
	now := w.now()
	if err == nil {
		o.ok, o.err = now, ""
		return
	}
	o.err, o.at = err.Error(), now
	if now.Sub(o.logged) >= webhookLogGap {
		o.logged = now
		log.Printf("webhook %q (%s): %v", h.Name, h.Kind, err)
	}
}
