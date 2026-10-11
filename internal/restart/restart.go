// Package restart brings chats onto a freshly updated agent. A pane
// running Claude Code, Codex or Gemini CLI is asked to leave (the agent's
// own /exit) and the agent is started again in the same pane with its
// session resumed — right away when the chat is idle, and a little after
// it settles otherwise. herdr does the typing (agent.prompt, agent.start):
// nothing here writes raw bytes into a terminal, a pane without a session
// id is left alone, and a restart holds the same per-pane lock the rest
// of herdr-ui holds while it types, so nothing else lands in the shell
// in between.
package restart

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/herdr-ui/internal/herdr"
	"github.com/dev1antdev/herdr-ui/internal/machines"
	"github.com/dev1antdev/herdr-ui/internal/push"
)

const (
	// settleDelay is how long after a busy chat settles before it is asked
	// to leave: the person is most likely reading or typing right then.
	settleDelay = 25 * time.Second
	// bookingTTL: a chat booked this long ago (blocked, or never settling)
	// is forgotten
	bookingTTL = 12 * time.Hour
	// again: a pane is not restarted twice within this
	again = 2 * time.Minute
)

// agent is how to leave and come back for one kind.
type agent struct {
	Kind   string
	Title  string
	exit   string // the slash command that quits it
	resume func(session string) []string
}

var agents = map[string]agent{
	"claude": {"claude", "Claude Code", "/exit", func(id string) []string { return []string{"--resume", id} }},
	"codex":  {"codex", "Codex", "/quit", func(id string) []string { return []string{"resume", id} }},
	"gemini": {"gemini", "Gemini CLI", "/quit", func(id string) []string { return []string{"--resume", id} }},
}

// Kinds says which agents an update called `what` touches: claude, codex,
// gemini, or all three for "agents"; none for herdr or herdr-ui.
func Kinds(what string) []string {
	switch what {
	case "claude", "codex", "gemini":
		return []string{what}
	case "agents":
		return []string{"claude", "codex", "gemini"}
	}
	return nil
}

// Title is the agent's name for people.
func Title(kind string) string {
	if a, ok := agents[kind]; ok {
		return a.Title
	}
	return kind
}

// Resume is the command line a restart runs, for showing.
func Resume(kind, session string) string {
	a, ok := agents[kind]
	if !ok {
		return ""
	}
	return a.Kind + " " + strings.Join(a.resume(session), " ")
}

// Chats are the panes of these kinds that can be restarted (they have a
// session to resume): the ready ones and the busy ones.
func Chats(snap *herdr.Snapshot, kinds []string) (ready, busy []herdr.PaneInfo) {
	if snap == nil {
		return nil, nil
	}
	for i := range snap.Panes {
		p := snap.Panes[i]
		if !has(kinds, p.Agent) || sessionOf(&p) == "" {
			continue
		}
		if settled(p.AgentStatus) {
			ready = append(ready, p)
		} else {
			busy = append(busy, p)
		}
	}
	return
}

// Count is Chats, counted.
func Count(snap *herdr.Snapshot, kinds []string) (ready, busy int) {
	r, b := Chats(snap, kinds)
	return len(r), len(b)
}

func has(kinds []string, k string) bool {
	for _, x := range kinds {
		if x == k {
			return true
		}
	}
	return false
}

// settled: the agent is ready for input, so it can be asked to leave
func settled(status string) bool {
	return status == herdr.StatusIdle || status == herdr.StatusDone
}

func sessionOf(p *herdr.PaneInfo) string {
	if p.AgentSession == nil || p.AgentSession.Kind != "id" {
		return ""
	}
	return p.AgentSession.Value
}

// booking is a busy chat waiting for its restart.
type booking struct {
	kind, session string
	at            time.Time
	timer         *time.Timer // running once the chat settled; nil while busy
}

// Restarter restarts chats and remembers the busy ones.
type Restarter struct {
	machines *machines.Manager
	// Later runs a deferred restart so the person can see it — a job, in
	// main. Nil means it is logged only.
	Later func(mc *machines.Machine, title string, fn func(say func(string)) error)
	// Lock is the per-pane lock the rest of herdr-ui holds while it types
	// into a pane; a restart holds it from the exit to the resumed agent.
	Lock func(machineID, paneID string) sync.Locker

	mu      sync.Mutex
	pending map[string]*booking  // machine|pane
	last    map[string]time.Time // machine|pane → when it was last restarted
}

func New(m *machines.Manager) *Restarter {
	return &Restarter{machines: m, pending: map[string]*booking{}, last: map[string]time.Time{}}
}

// Now restarts every ready chat of these kinds on the machine, saying what
// it does, and books the busy ones for when they settle.
func (r *Restarter) Now(mc *machines.Machine, kinds []string, say func(string)) error {
	client, _, err := mc.Ready()
	if err != nil {
		return err
	}
	snap, err := client.SessionSnapshot()
	if err != nil {
		return err
	}
	var failed []string
	n := 0
	for i := range snap.Panes {
		p := snap.Panes[i]
		if !has(kinds, p.Agent) {
			continue
		}
		n++
		sid := sessionOf(&p)
		if sid == "" {
			say(fmt.Sprintf("· %s (%s): no session id to resume — left as it is", p.DisplayTitle(), p.PaneID))
			continue
		}
		if !settled(p.AgentStatus) {
			r.book(mc.Cfg.ID, p.PaneID, p.Agent, sid)
			say(fmt.Sprintf("· %s (%s): %s — restarts once it has settled for a while", p.DisplayTitle(), p.PaneID, p.AgentStatus))
			continue
		}
		if err := r.one(mc, p.PaneID, p.Agent, sid, say); err != nil {
			say("  ✗ " + err.Error())
			failed = append(failed, p.DisplayTitle())
		}
	}
	if n == 0 {
		say("no chats of that kind here")
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d chat(s) not restarted: %s", len(failed), strings.Join(failed, ", "))
	}
	return nil
}

// one restarts a single chat under the pane's lock: a fresh look at the
// pane (still that agent, that session, settled), leave, wait for the
// shell, start with the session resumed.
func (r *Restarter) one(mc *machines.Machine, paneID, kind, session string, say func(string)) error {
	a, ok := agents[kind]
	if !ok {
		return nil
	}
	key := mc.Cfg.ID + "|" + paneID
	if r.Lock != nil {
		l := r.Lock(mc.Cfg.ID, paneID)
		l.Lock()
		defer l.Unlock()
	}
	r.mu.Lock()
	if t, ok := r.last[key]; ok && time.Since(t) < again {
		r.mu.Unlock()
		say(fmt.Sprintf("· %s: restarted %s ago — not again", paneID, time.Since(t).Round(time.Second)))
		return nil
	}
	r.mu.Unlock()
	client, _, err := mc.Ready()
	if err != nil {
		r.book(mc.Cfg.ID, paneID, kind, session) // the machine is away: when it is back and the chat settles
		return fmt.Errorf("%s: %v — booked for later", paneID, err)
	}
	p, found := pane(client, paneID)
	if !found {
		say(fmt.Sprintf("· %s: the pane is gone", paneID))
		return nil
	}
	title := p.DisplayTitle()
	switch {
	case p.Agent != kind:
		say(fmt.Sprintf("· %s (%s): runs %q now, not %s — left as it is", title, paneID, p.Agent, a.Title))
		return nil
	case sessionOf(&p) != session:
		say(fmt.Sprintf("· %s (%s): another session now — left as it is", title, paneID))
		return nil
	case !settled(p.AgentStatus):
		r.book(mc.Cfg.ID, paneID, kind, session)
		say(fmt.Sprintf("· %s (%s): %s again — once it has settled", title, paneID, p.AgentStatus))
		return nil
	}
	r.mu.Lock()
	r.last[key] = time.Now()
	r.mu.Unlock()
	say(fmt.Sprintf("· %s (%s): %s %s", title, paneID, a.Title, a.exit))
	if err := client.AgentPrompt(paneID, a.exit); err != nil {
		return fmt.Errorf("%s: %s: %v", title, a.exit, err)
	}
	state := r.waitShell(client, paneID, kind, 12*time.Second)
	if state == stillThere {
		say("  still there — Ctrl-C twice")
		if err := client.PaneSendKeys(paneID, []string{"ctrl+c", "ctrl+c"}); err != nil {
			return fmt.Errorf("%s: Ctrl-C: %v", title, err)
		}
		state = r.waitShell(client, paneID, kind, 8*time.Second)
	}
	switch state {
	case paneGone:
		return fmt.Errorf("%s: the pane closed with the agent (no shell under it); open a new one and run `%s`", title, Resume(a.Kind, session))
	case unseen:
		return fmt.Errorf("%s: could not see the pane after %s — left as it is", title, a.exit)
	case somethingElse:
		return fmt.Errorf("%s: something else runs in the pane now — left as it is", title)
	case stillThere:
		return fmt.Errorf("%s: %s did not leave — left as it is", title, a.Title)
	}
	time.Sleep(700 * time.Millisecond) // the prompt settles
	args := a.resume(session)
	say(fmt.Sprintf("  %s %s", a.Kind, strings.Join(args, " ")))
	if err := client.AgentStartWith(paneID, a.Kind, agentName(a.Kind, paneID), args, 90*time.Second); err != nil {
		return fmt.Errorf("%s: start %s: %v — the pane is at its shell; run `%s` there", title, a.Title, err, Resume(a.Kind, session))
	}
	say(fmt.Sprintf("  ✓ %s is back, session %s resumed", a.Title, short(session)))
	return nil
}

type paneState int

const (
	shellBack paneState = iota
	paneGone
	stillThere
	somethingElse
	unseen // no snapshot came back at all: nothing is known
)

// waitShell polls until the pane has no agent (shell back), is gone, or
// runs something else; stillThere after the timeout.
func (r *Restarter) waitShell(client *herdr.Client, paneID, kind string, timeout time.Duration) paneState {
	deadline := time.Now().Add(timeout)
	seen := false
	for time.Now().Before(deadline) {
		time.Sleep(time.Second)
		p, found, err := look(client, paneID)
		if err != nil {
			continue
		}
		seen = true
		switch {
		case !found:
			return paneGone
		case p.Agent == "":
			return shellBack
		case p.Agent != kind:
			return somethingElse
		}
	}
	if !seen {
		return unseen
	}
	return stillThere
}

func pane(client *herdr.Client, paneID string) (herdr.PaneInfo, bool) {
	p, found, _ := look(client, paneID)
	return p, found
}

func look(client *herdr.Client, paneID string) (herdr.PaneInfo, bool, error) {
	snap, err := client.SessionSnapshot()
	if err != nil {
		return herdr.PaneInfo{}, false, err
	}
	for i := range snap.Panes {
		if snap.Panes[i].PaneID == paneID {
			return snap.Panes[i], true, nil
		}
	}
	return herdr.PaneInfo{}, false, nil
}

func (r *Restarter) book(machineID, paneID, kind, session string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b := r.pending[machineID+"|"+paneID]; b != nil && b.timer != nil {
		b.timer.Stop()
	}
	r.pending[machineID+"|"+paneID] = &booking{kind: kind, session: session, at: time.Now()}
}

// OnStatus is the watcher's hook. A booked chat that settled gets its
// restart a little later — any change before then calls it off until the
// next settle; a booked pane whose agent left, or that closed, is
// forgotten.
func (r *Restarter) OnStatus(ev push.Event) {
	key := ev.Pane.Machine + "|" + ev.Pane.Pane
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.pending[key]
	if b == nil {
		return
	}
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	if ev.To == "gone" || ev.Kind != b.kind || time.Since(b.at) > bookingTTL {
		delete(r.pending, key)
		return
	}
	if !settled(ev.To) {
		return // still busy; the next settle
	}
	b.timer = time.AfterFunc(settleDelay, func() { r.fire(key, ev) })
}

// fire is the settle timer going off: the booking becomes a restart job.
func (r *Restarter) fire(key string, ev push.Event) {
	r.mu.Lock()
	b := r.pending[key]
	if b == nil || b.timer == nil {
		r.mu.Unlock()
		return
	}
	delete(r.pending, key)
	kind, session := b.kind, b.session
	r.mu.Unlock()
	mc, err := r.machines.Get(ev.Pane.Machine)
	if err != nil {
		return
	}
	title := fmt.Sprintf("Restart %s in %s", Title(kind), firstNonEmpty(ev.PaneTitle, ev.Pane.Pane))
	run := func(say func(string)) error { return r.one(mc, ev.Pane.Pane, kind, session, say) }
	if r.Later != nil {
		r.Later(mc, title, run)
		return
	}
	go func() {
		if err := run(func(line string) { log.Printf("restart %s: %s", ev.Pane, line) }); err != nil {
			log.Printf("restart %s: %v", ev.Pane, err)
		}
	}()
}

// Pending says how many chats on the machine wait for a restart.
func (r *Restarter) Pending(machineID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for k, b := range r.pending {
		if time.Since(b.at) > bookingTTL {
			if b.timer != nil {
				b.timer.Stop()
			}
			delete(r.pending, k)
			continue
		}
		if strings.HasPrefix(k, machineID+"|") {
			n++
		}
	}
	return n
}

var nameRe = regexp.MustCompile(`[^a-z0-9_-]+`)

// agentName is a live-agent name herdr accepts ([a-z][a-z0-9_-]{0,31}),
// unique per pane.
func agentName(kind, paneID string) string {
	n := kind + "-" + nameRe.ReplaceAllString(strings.ToLower(paneID), "-")
	if len(n) > 32 {
		n = n[:32]
	}
	return strings.TrimRight(n, "-")
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
