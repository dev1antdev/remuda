package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// Adapter is a machine's remuda-core as herdr.API: a mirror of its live terminals and the `remuda` document, kept
// from one event subscription, so a snapshot is read from memory and every change reaches the watcher as it happens.
type Adapter struct {
	client *Client
	hooks  AdapterHooks
	stop   context.CancelFunc
	done   chan struct{}

	mu      sync.Mutex
	state   mirror
	held    bool          // the mirror holds what the core said
	filled  chan struct{} // closed at the first fill
	lost    error         // why the mirror is not held, after it was
	streams map[*eventStream]struct{}
	badDoc  bool // a document that is no document was logged already
}

// AdapterHooks are what the machine hears from its adapter and lends it.
type AdapterHooks struct {
	// TransportError hears a failure to reach the core, never its refusal: the machine marks itself dead on it.
	TransportError func(error)
	// SessionOf names an agent's conversation by its process. It answers from memory, never waiting; nil for none.
	SessionOf func(agent string, pid int) *herdr.AgentSession
}

// How long a snapshot asked for before the first fill waits for it, and how the subscription is tried again.
const (
	firstFillWait = 10 * time.Second
	retryFrom     = 250 * time.Millisecond
	retryUpTo     = 5 * time.Second
)

// NewAdapter starts following the core client reaches; Close stops it.
func NewAdapter(client *Client, hooks AdapterHooks) *Adapter {
	ctx, stop := context.WithCancel(context.Background())
	a := &Adapter{
		client: client, hooks: hooks, stop: stop, done: make(chan struct{}),
		state: mirror{terminals: map[string]*Terminal{}}, filled: make(chan struct{}), streams: map[*eventStream]struct{}{},
	}
	go a.run(ctx)
	return a
}

// Close stops following the core and ends every event stream; the client stays its owner's.
func (a *Adapter) Close() error {
	a.stop()
	<-a.done
	return nil
}

var _ herdr.API = (*Adapter)(nil)

// SessionSnapshot is the machine as the mirror holds it, without a request; before the first fill it waits for one.
func (a *Adapter) SessionSnapshot() (*herdr.Snapshot, error) {
	a.mu.Lock()
	filled, lost := a.filled, a.lost
	a.mu.Unlock()
	if lost == nil {
		select {
		case <-filled:
		case <-time.After(firstFillWait):
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.held {
		return nil, a.whyNotHeld()
	}
	return a.state.snapshot(a.hooks.SessionOf), nil
}

func (a *Adapter) whyNotHeld() error {
	if a.lost != nil {
		return a.lost
	}
	return &TransportError{errors.New("no answer yet")}
}

func (a *Adapter) run(ctx context.Context) {
	defer close(a.done)
	defer a.drop(&TransportError{net.ErrClosed})
	wait := retryFrom
	for {
		filled, err := a.follow(ctx)
		if ctx.Err() != nil {
			return
		}
		a.drop(err)
		if IsTransport(err) && a.hooks.TransportError != nil {
			a.hooks.TransportError(err)
		}
		if filled {
			wait = retryFrom
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, retryUpTo)
	}
}

// follow subscribes, fills the mirror from the subscription's snapshot and the document read after it, and applies
// the changes until the subscription ends; filled says whether it got that far.
func (a *Adapter) follow(ctx context.Context) (filled bool, err error) {
	sub, err := a.client.Subscribe(ctx, SubscribeParams{})
	if err != nil {
		return false, err
	}
	defer sub.Close()
	hello, err := a.client.Hello(ctx)
	if err != nil {
		return false, err
	}
	doc, err := a.readDoc(ctx)
	if err != nil {
		return false, err
	}
	a.fill(sub.Terminals, doc, hello.Version)
	for {
		notice, err := sub.Next(ctx)
		if err != nil {
			return true, err
		}
		switch notice.Kind {
		case NoticeEvent:
			if err := a.apply(ctx, notice.Event); err != nil {
				return true, err
			}
		case NoticeMissed:
			doc, err := a.readDoc(ctx)
			if err != nil {
				return true, err
			}
			a.fill(notice.Terminals, doc, hello.Version)
		}
	}
}

func (a *Adapter) readDoc(ctx context.Context) (Document, error) {
	raw, err := a.client.DocGet(ctx, DocName)
	if err != nil {
		return Document{}, err
	}
	doc, err := parseDocument(raw.Value)
	if err != nil {
		a.mu.Lock()
		if !a.badDoc {
			log.Printf("remuda-core: the %q document is not one of remuda's, read as empty: %v", DocName, err)
		}
		a.badDoc = true
		a.mu.Unlock()
	}
	return doc, nil
}

// fill replaces the mirror with a snapshot of the core's — this machine's terminals, not those of a machine it
// added, which come addressed as `machine/t7` — and tells every stream that the machine moved.
func (a *Adapter) fill(terminals []Terminal, doc Document, version string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state = mirror{terminals: make(map[string]*Terminal, len(terminals)), doc: doc, version: version}
	for i := range terminals {
		if terminals[i].State == "live" && !strings.Contains(terminals[i].Address, "/") {
			followShell(&terminals[i])
			a.state.terminals[terminals[i].Address] = &terminals[i]
		}
	}
	a.held, a.lost = true, nil
	select {
	case <-a.filled:
	default:
		close(a.filled)
	}
	a.tellLocked(herdr.Event{Kind: "layout.updated"})
}

// drop lets the mirror go when the subscription ended: snapshots answer why, and the streams end so their reader asks
// again once the core answers.
func (a *Adapter) drop(err error) {
	a.mu.Lock()
	a.held, a.lost = false, err
	streams := a.streams
	a.streams = map[*eventStream]struct{}{}
	a.mu.Unlock()
	for s := range streams {
		s.end(err)
		s.closeOutput()
	}
}

// apply is one change of the core's: the document read again when it moved, a terminal's fact otherwise.
func (a *Adapter) apply(ctx context.Context, ev Event) error {
	if ev.Machine != "" {
		return nil
	}
	if ev.Type == "doc" {
		var moved struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(ev.Data, &moved) != nil || moved.Name != DocName {
			return nil
		}
		doc, err := a.readDoc(ctx)
		if err != nil {
			return err
		}
		a.mu.Lock()
		a.state.doc = doc
		a.tellLocked(herdr.Event{Kind: "layout.updated"})
		a.mu.Unlock()
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if told, ok := a.state.apply(ev); ok {
		a.tellLocked(told)
	}
	return nil
}

func (a *Adapter) tellLocked(ev herdr.Event) {
	for s := range a.streams {
		s.push(ev)
	}
}

// apply changes the mirror by one event (schema/README.md, "Events") and answers it in herdr's words: what the
// watcher reads as work to look at now, a spinner's title it reads lazily, a status change with the status.
func (m *mirror) apply(ev Event) (herdr.Event, bool) {
	told := herdr.Event{PaneID: ev.Terminal}
	t := m.terminals[ev.Terminal]
	switch ev.Type {
	case "created", "restarted":
		var record Terminal
		if json.Unmarshal(ev.Data, &record) != nil || record.Address == "" {
			return told, false
		}
		if record.State != "live" {
			delete(m.terminals, record.Address)
			return herdr.Event{Kind: "pane.closed", PaneID: record.Address}, t != nil
		}
		followShell(&record)
		m.terminals[record.Address] = &record
		told.Kind = "pane.created"
		return told, true
	case "removed", "finished":
		if t == nil {
			return told, false
		}
		delete(m.terminals, ev.Terminal)
		told.Kind = "pane.closed"
		return told, true
	}
	if t == nil {
		return told, false
	}
	agent := agentOf(t)
	told.Kind = "pane.updated"
	switch ev.Type {
	case "status":
		var status struct {
			Status   *Status           `json:"status"`
			Records  map[string]Status `json:"records"`
			Reported bool              `json:"reported"`
		}
		if json.Unmarshal(ev.Data, &status) != nil {
			return told, false
		}
		t.Status, t.Records, t.Reported = status.Status, status.Records, status.Reported
		told.Kind, told.AgentStatus = herdr.EventAgentStatus, statusOf(t)
	case "turn":
		json.Unmarshal(ev.Data, &struct {
			Turn *uint64 `json:"turn"`
		}{&t.Turn})
	case "title":
		json.Unmarshal(ev.Data, &struct {
			Title *string `json:"title"`
		}{&t.Title})
	case "labels":
		var labels struct {
			Labels map[string]string `json:"labels"`
		}
		if json.Unmarshal(ev.Data, &labels) != nil {
			return told, false
		}
		t.Labels = labels.Labels
		told.Kind = "pane.moved"
	case "size":
		json.Unmarshal(ev.Data, &struct {
			Cols *uint16 `json:"cols"`
			Rows *uint16 `json:"rows"`
		}{&t.Cols, &t.Rows})
		told.Kind = "layout.updated"
	case "foreground":
		var fg *Foreground
		if json.Unmarshal(ev.Data, &fg) != nil {
			return told, false
		}
		t.Foreground = fg
		followShell(t)
	case "lost":
		json.Unmarshal(ev.Data, &struct {
			Reason   *string `json:"reason"`
			ExitCode **int   `json:"exit_code"`
			Signal   **int   `json:"signal"`
		}{&t.Reason, &t.ExitCode, &t.Signal})
		t.Pid, t.Foreground = 0, nil
		told.Kind = "pane.exited"
	case "keep":
		json.Unmarshal(ev.Data, &struct {
			Keep *bool `json:"keep"`
		}{&t.Keep})
		return told, false
	default:
		return told, false
	}
	if told.Kind == "pane.updated" && agentOf(t) != agent {
		told.Kind = "pane.agent_detected"
	}
	return told, true
}

// followShell keeps a terminal's folder the shell's, as herdr's pane cwd is: where its shell stood when it last held
// the terminal, not where a program it started went.
func followShell(t *Terminal) {
	if fg := t.Foreground; fg != nil && fg.Pid == t.Pid && fg.Cwd != "" {
		t.Cwd = fg.Cwd
	}
}

// notOnCore is herdr's refusal of a request remuda-core does not answer yet.
func notOnCore(method string) error {
	return &herdr.Error{Code: "invalid_request", Message: fmt.Sprintf("%s is not on remuda-core yet", method)}
}

// refusal is the core's refusal in herdr's words, so the callers' herdr.IsCode checks hold (notFound names what was
// missing); a failure to reach the core is told to the machine and passed on.
func (a *Adapter) refusal(err error, notFound string) error {
	if err == nil {
		return nil
	}
	var refused *Error
	if errors.As(err, &refused) {
		code := refused.Kind
		switch code {
		case "not_found":
			code = notFound
		case "needs_update":
			code = "invalid_request"
		}
		return &herdr.Error{Code: code, Message: refused.Error()}
	}
	if IsTransport(err) && a.hooks.TransportError != nil {
		a.hooks.TransportError(err)
	}
	return err
}
