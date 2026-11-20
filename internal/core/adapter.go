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
	"github.com/dev1antdev/remuda/internal/shell"
)

// Adapter is a machine's remuda-core as herdr.API: a mirror of its live terminals and the `remuda` document, kept
// from one event subscription, so a snapshot is read from memory and every change reaches the watcher as it happens.
type Adapter struct {
	client *Client
	shell  shell.Shell // the machine's: the git its worktrees are made and read with, the core knowing none
	hooks  AdapterHooks
	life   context.Context // ends with Close
	stop   context.CancelFunc
	done   chan struct{}

	mu       sync.Mutex
	state    mirror
	held     bool          // the mirror holds what the core said
	answered chan struct{} // closed at the first fill or the first loss, whichever comes first
	lost     error         // why the mirror is not held, after it was
	streams  map[*eventStream]struct{}
	badDoc   bool // a document that is no document was logged already

	feedMu sync.Mutex            // join and leave one at a time; held over a feed's subscribe, never by a feed's teller
	feeds  map[OutputWatch]*feed // the output feeds that stand, by their watch

	docMu   sync.Mutex // one write of the document at a time from this remuda (editDoc)
	opening sync.Mutex // one WorktreeOpen at a time, so a second open of a checkout finds the first one's workspace
}

// AdapterHooks are what the machine hears from its adapter and lends it.
type AdapterHooks struct {
	// TransportError hears a failure to reach the core, never its refusal: the machine marks itself dead on it.
	TransportError func(error)
	// SessionOf names an agent's conversation by its process. It answers from memory, never waiting; nil for none.
	SessionOf SessionLookup
	// Reread has SessionOf's answers read again now, waiting for them; nil for none.
	Reread func() error
}

// SessionLookup names the conversation of the agent of a kind running as the process pid in the folder cwd; nil
// while it is not known.
type SessionLookup func(agent string, pid int, cwd string) *herdr.AgentSession

// How long a snapshot asked for before the first fill waits for it, and how the subscription is tried again.
const (
	firstFillWait = 10 * time.Second
	retryFrom     = 250 * time.Millisecond
	retryUpTo     = 5 * time.Second
)

// foregroundLag is how long the core may take to look at a terminal's foreground again (about twice a second): a
// status an agent reports in that time comes before the look that sees the agent.
const foregroundLag = 3 * time.Second

// quietFor is how long the subscription may say nothing: the core sends a heartbeat every 15 s, so three missed
// mean it stopped answering while its connection stays open. A test shortens it.
var quietFor = 45 * time.Second

// NewAdapter starts following the core client reaches; Close stops it. sh is the machine's shell, which its worktrees
// need (nil only in a test that makes none: they are refused).
func NewAdapter(client *Client, sh shell.Shell, hooks AdapterHooks) *Adapter {
	ctx, stop := context.WithCancel(context.Background())
	a := &Adapter{
		client: client, shell: sh, hooks: hooks, life: ctx, stop: stop, done: make(chan struct{}),
		state: mirror{terminals: map[string]*Terminal{}}, answered: make(chan struct{}), streams: map[*eventStream]struct{}{},
		feeds: map[OutputWatch]*feed{},
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

// RereadSessions has the agents' sessions read off their processes again before it answers, for a step about to stop
// an agent and resume it: a snapshot answers what the last read found, which may be half a minute old.
func (a *Adapter) RereadSessions() error {
	if a.hooks.Reread == nil {
		return nil
	}
	return a.hooks.Reread()
}

// SessionSnapshot is the machine as the mirror holds it, without a request; before the core first answered it waits
// for that.
func (a *Adapter) SessionSnapshot() (*herdr.Snapshot, error) {
	select {
	case <-a.answered:
	case <-time.After(firstFillWait):
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
	var cn *conn
	var sub *Subscription
	var doc Document
	err = bounded(ctx, callWithin, func() *conn { return cn }, func(ctx context.Context) (err error) {
		if cn, err = a.checked(ctx); err != nil {
			return err
		}
		if sub, err = cn.subscribe(ctx, SubscribeParams{}); err != nil {
			return err
		}
		doc, err = a.readDoc(ctx, cn)
		return err
	})
	if sub != nil {
		defer sub.Close()
	}
	if err != nil {
		return false, err
	}
	version := cn.hello.Version
	onSub := func() *conn { return cn }
	a.keepOlder(a.fill(sub.Terminals, doc, version))
	for {
		var notice Notice
		err := bounded(ctx, quietFor, onSub, func(ctx context.Context) (err error) {
			notice, err = sub.Next(ctx)
			return err
		})
		if err != nil {
			return true, err
		}
		switch notice.Kind {
		case NoticeEvent:
			if err := a.apply(ctx, cn, notice.Event); err != nil {
				return true, err
			}
		case NoticeMissed:
			err := bounded(ctx, callWithin, onSub, func(ctx context.Context) (err error) {
				doc, err = a.readDoc(ctx, cn)
				return err
			})
			if err != nil {
				return true, err
			}
			a.keepOlder(a.fill(notice.Terminals, doc, version))
		}
	}
}

// keepOlder has the panes remuda started before it kept them kept too, so a reboot brings them back as well — each
// one carrying a workspace's label, never another program's terminal; one refused is tried again at the next fill.
func (a *Adapter) keepOlder(addresses []string) {
	if len(addresses) == 0 {
		return
	}
	go func() {
		for _, address := range addresses {
			ctx, cancel := context.WithTimeout(a.life, callWithin)
			err := a.client.Keep(ctx, address, true)
			cancel()
			if err != nil && !IsKind(err, "not_found") {
				if a.life.Err() != nil {
					return
				}
				log.Printf("remuda-core: %s is not kept, so a reboot would close it: %v", address, err)
				return
			}
		}
	}()
}

// bounded runs a step of the subscription within its time. A step the core lets run out on a connection it holds
// open saying nothing fails that very connection — never one dialled since — so the next attempt dials a new one.
func bounded(ctx context.Context, within time.Duration, on func() *conn, step func(context.Context) error) error {
	stepCtx, cancel := context.WithTimeout(ctx, within)
	defer cancel()
	err := step(stepCtx)
	if err != nil && ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
		stalled := &TransportError{fmt.Errorf("remuda-core said nothing for %s", within)}
		if cn := on(); cn != nil {
			cn.fail(stalled)
		}
		return stalled
	}
	return err
}

// checked is the client's connection when its core reads history without scrolling a program (a vt read, which
// came before output watches in remuda-core); an older core is refused.
func (a *Adapter) checked(ctx context.Context) (*conn, error) {
	cn, err := a.client.connection(ctx)
	if err == nil && !cn.hello.Has("output_watches") {
		err = &Error{Kind: "needs_update", Message: fmt.Sprintf("remuda-core %s is older than remuda needs: update it", cn.hello.Version)}
	}
	return cn, err
}

// readDoc reads the document on the subscription's own connection, so a snapshot and its document come from one
// core; a connection lost meanwhile fails the read and the subscription is made again whole.
func (a *Adapter) readDoc(ctx context.Context, cn *conn) (Document, error) {
	var raw Doc
	if err := cn.call(ctx, "doc.get", map[string]any{"name": DocName}, &raw, nil); err != nil {
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
// added, which come addressed as `machine/t7` — and tells every stream that the machine moved. A terminal waiting for
// a new shell, or whose shell is another than the one the mirror held before, was restored meanwhile; unkept are
// remuda's panes started before it kept them.
func (a *Adapter) fill(terminals []Terminal, doc Document, version string) (unkept []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	before := a.state.terminals
	a.state = mirror{terminals: make(map[string]*Terminal, len(terminals)), doc: doc, version: version, gone: map[string]bool{}}
	for i := range terminals {
		t := &terminals[i]
		if t.State != "live" || !local(t.Address) {
			continue
		}
		followShell(t)
		was := before[t.Address]
		t.restored = agentOf(t) == "" && (t.Reason != "" || was != nil && (was.restored || was.Pid != t.Pid))
		a.state.terminals[t.Address] = t
		if !t.Keep && t.Labels[LabelSpace] != "" {
			unkept = append(unkept, t.Address)
		}
	}
	a.held, a.lost = true, nil
	a.answer()
	a.tellLocked(herdr.Event{Kind: "layout.updated"})
	return unkept
}

// answer lets the snapshots waiting for the core's first answer go; the caller holds the lock.
func (a *Adapter) answer() {
	select {
	case <-a.answered:
	default:
		close(a.answered)
	}
}

// drop lets the mirror go when the subscription ended: snapshots answer why, and the streams end so their reader asks
// again once the core answers.
func (a *Adapter) drop(err error) {
	a.mu.Lock()
	a.held, a.lost = false, err
	a.answer()
	streams := a.streams
	a.streams = map[*eventStream]struct{}{}
	a.mu.Unlock()
	for s := range streams {
		s.end(err)
		a.leave(s)
	}
}

// apply is one change of the core's: the document read again when it moved, a terminal's fact otherwise.
func (a *Adapter) apply(ctx context.Context, cn *conn, ev Event) error {
	if ev.Machine != "" || ev.Terminal != "" && !local(ev.Terminal) {
		return nil
	}
	if ev.Type == "doc" {
		var moved struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(ev.Data, &moved) != nil || moved.Name != DocName {
			return nil
		}
		var doc Document
		err := bounded(ctx, callWithin, func() *conn { return cn }, func(ctx context.Context) (err error) {
			doc, err = a.readDoc(ctx, cn)
			return err
		})
		if err != nil {
			return err
		}
		a.setDoc(doc)
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if told, ok := a.state.apply(ev); ok {
		a.tellLocked(told)
	}
	return nil
}

// setDoc is the document as the mirror holds it now, which every stream hears as a layout that moved.
func (a *Adapter) setDoc(doc Document) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.doc = doc
	a.tellLocked(herdr.Event{Kind: "layout.updated"})
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
	now := time.Now()
	switch ev.Type {
	case "created", "restarted":
		var record Terminal
		if json.Unmarshal(ev.Data, &record) != nil || record.Address != ev.Terminal || !local(record.Address) {
			return told, false
		}
		if record.State != "live" {
			m.went(record.Address)
			return herdr.Event{Kind: "pane.closed", PaneID: record.Address}, t != nil
		}
		record.restored = ev.Type == "restarted"
		return m.add(record), true
	case "removed", "finished":
		m.went(ev.Terminal)
		if t == nil {
			return told, false
		}
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
		t.statusAt = now
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
		t.Pid, t.Foreground, t.restored = 0, nil, true
		told.Kind = "pane.exited"
	case "keep":
		json.Unmarshal(ev.Data, &struct {
			Keep *bool `json:"keep"`
		}{&t.Keep})
		return told, false
	default:
		return told, false
	}
	if became := agentOf(t); became != agent {
		if became != "" {
			t.agentAt, t.restored = now, false
			// a report naming this agent that came before the foreground's next look was the agent's own, its first
			if t.Status != nil && appAgent(t.Status.App) == became && now.Sub(t.statusAt) <= foregroundLag {
				t.agentAt = t.statusAt
			}
		}
		if told.Kind == "pane.updated" {
			told.Kind = "pane.agent_detected"
		}
	}
	return told, true
}

// went takes a terminal out of the mirror and remembers it went.
func (m *mirror) went(address string) {
	delete(m.terminals, address)
	if m.gone == nil {
		m.gone = map[string]bool{}
	}
	m.gone[address] = true
}

// add puts a live terminal in the mirror, its folder its shell's, and answers it as herdr's news of a pane made.
func (m *mirror) add(record Terminal) herdr.Event {
	followShell(&record)
	m.terminals[record.Address] = &record
	return herdr.Event{Kind: "pane.created", PaneID: record.Address}
}

// local says an address is this machine's terminal: one on a machine the core added comes as `machine/t7`.
func local(address string) bool { return address != "" && !strings.Contains(address, "/") }

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
