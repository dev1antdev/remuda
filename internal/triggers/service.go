package triggers

import (
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/push"
)

// The spells that keep a pane from saying the same thing over and over; vars so tests can shrink them.
var (
	TestsQuiet  = 30 * time.Minute // one failed-tests push per pane in this long: an agent testing all afternoon is one word
	RuleQuiet   = time.Minute      // one action per rule and pane in this long
	OfferKeep   = 15 * time.Minute // how long an offer to preview stands on the roster
	newFor      = time.Minute      // a pane born this recently while watched: what it prints first is news
	dupWindow   = 3 * time.Second  // the same row through the stream before and the one after it, while they overlap
	listenTries = []time.Duration{0, 1500 * time.Millisecond, 4 * time.Second}
)

// Service turns matches into what they mean. Its ears are set in internal/app; a nil one is a thing it does not do.
type Service struct {
	Rules *Store

	Notify      func(push.Payload)                                     // a push, and its row in the activity
	Feed        func(machine, pane, title, body string)                // a row in the activity alone
	Listening   func(machine string, port int) (label string, ok bool) // the port found listening by the previews
	MoveTask    func(machine, pane, column, why string) (string, bool, error)
	Viewed      func(machine, pane string) bool // someone has the pane's terminal open through this remuda now
	Changed     func()                          // an offer came: the roster moved
	Resubscribe func()                          // the rules moved: the streams' watches with them

	mu       sync.Mutex
	machines map[string]map[string]*paneView // machine → pane
	standing map[string]standing             // "machine|pane|watch" → the row last matched
	acted    map[string]time.Time            // "machine|pane|watch" → the last action, for the quiet spells
	offers   map[string]Offer                // "machine|pane"
	refused  map[string]string               // machine → why herdr refused the person's watches
}

type paneView struct {
	firstSeen time.Time
	born      bool // first seen after the machine's first snapshot: made while watched
	workspace string
	agent     string
	status    string
}

type standing struct {
	line string
	at   time.Time
}

// Offer is a dev server a pane announced, found listening: the page offers it as a preview once.
type Offer struct {
	Port  int       `json:"port"`
	URL   string    `json:"url,omitempty"`   // as the pane printed it
	Label string    `json:"label,omitempty"` // what serves it: vite, next, http.server…
	By    string    `json:"by,omitempty"`    // the trigger's name when one of the person's rules offered it
	At    time.Time `json:"at"`
}

// New is the service over the person's rules in dir.
func New(dir string) *Service {
	return &Service{Rules: Open(dir), machines: map[string]map[string]*paneView{}, standing: map[string]standing{},
		acted: map[string]time.Time{}, offers: map[string]Offer{}, refused: map[string]string{}}
}

func key(machine, pane string) string { return machine + "|" + pane }

// Observe keeps what the matches need to know of each pane: when it was first seen, whether it was born while
// watched, its workspace and its agent's status. A pane gone takes its standing rows and its offer with it.
func (s *Service) Observe(e events.Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	panes := s.machines[e.Machine]
	if panes == nil {
		panes = map[string]*paneView{}
		s.machines[e.Machine] = panes
	}
	seen := map[string]bool{}
	for i := range e.Snap.Panes {
		pane := &e.Snap.Panes[i]
		seen[pane.PaneID] = true
		view := panes[pane.PaneID]
		if view == nil {
			view = &paneView{firstSeen: e.At, born: !e.First}
			panes[pane.PaneID] = view
		}
		view.workspace, view.agent, view.status = e.Snap.WorkspaceLabel(pane.WorkspaceID), pane.Agent, pane.AgentStatus
	}
	for id := range panes {
		if !seen[id] {
			delete(panes, id)
			s.forgetPane(e.Machine, id)
		}
	}
}

// forgetPane drops a pane's rows, spells and offer; mu held.
func (s *Service) forgetPane(machine, pane string) {
	prefix := key(machine, pane) + "|"
	for k := range s.standing {
		if strings.HasPrefix(k, prefix) {
			delete(s.standing, k)
		}
	}
	for k := range s.acted {
		if strings.HasPrefix(k, prefix) {
			delete(s.acted, k)
		}
	}
	delete(s.offers, key(machine, pane))
}

// Forget drops a machine taken out of the fleet.
func (s *Service) Forget(machine string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.machines[machine] {
		s.forgetPane(machine, id)
	}
	delete(s.machines, machine)
	delete(s.refused, machine)
}

// OnMatch hears a match. herdr says a match once as it appears, and again for one standing on the screen whenever a
// stream begins: the same row a moment after a stream opened, or through two streams at once, is that row again; a
// row standing when a watch began on a pane that was there before is where things stood, not news — only a pane born
// while watched prints its first rows as news.
func (s *Service) OnMatch(e events.OutputMatched) {
	k := key(e.Pane.Machine, e.Pane.Pane) + "|" + e.Watch
	s.mu.Lock()
	prev, had := s.standing[k]
	s.standing[k] = standing{line: e.Line, at: e.At}
	view := s.machines[e.Pane.Machine][e.Pane.Pane]
	s.mu.Unlock()
	if had && prev.line == e.Line && (e.Fresh || e.At.Sub(prev.at) < dupWindow) {
		return
	}
	if view == nil {
		return // a pane no snapshot showed yet: its first snapshot comes before anything it printed is news
	}
	if e.Fresh && !e.Carried && !(view.born && e.At.Sub(view.firstSeen) < newFor) {
		return
	}
	pane := *view
	go s.act(e, pane)
}

// act does what a match means; off the stream's goroutine, since an offer runs a script on the machine.
func (s *Service) act(e events.OutputMatched, pane paneView) {
	machine, paneID := e.Pane.Machine, e.Pane.Pane
	line := cut(strings.TrimSpace(e.Line), 160)
	switch {
	case e.Watch == WatchServe:
		s.offer(machine, paneID, e.Line, "")
	case e.Watch == WatchTests:
		if pane.status == herdr.StatusWorking || s.Viewed != nil && s.Viewed(machine, paneID) || !s.spell(e, TestsQuiet) {
			return // the agent is on it, or the person is looking at it, or it said so already
		}
		if s.Notify != nil {
			s.Notify(push.Payload{Machine: machine, Pane: paneID, Kind: "tests", Title: "Tests failed in " + where(pane), Body: line, Tag: "tests|" + key(machine, paneID)})
		}
	case strings.HasPrefix(e.Watch, rulePrefix):
		rule, ok := s.rule(strings.TrimPrefix(e.Watch, rulePrefix))
		if !ok || !s.spell(e, RuleQuiet) {
			return
		}
		s.do(rule, machine, paneID, pane, e.Line)
	}
}

// do is a rule's action.
func (s *Service) do(rule Rule, machine, paneID string, pane paneView, row string) {
	title := rule.Name + " · " + where(pane)
	line := cut(strings.TrimSpace(row), 160)
	switch rule.Action {
	case ActionPush:
		if s.Notify != nil {
			s.Notify(push.Payload{Machine: machine, Pane: paneID, Kind: "trigger", Title: title, Body: line, Tag: "trigger|" + rule.ID + "|" + key(machine, paneID)})
		}
	case ActionFeed:
		if s.Feed != nil {
			s.Feed(machine, paneID, title, line)
		}
	case ActionPreview:
		s.offer(machine, paneID, row, rule.Name)
	case ActionTask:
		if s.MoveTask == nil {
			return
		}
		words, moved, err := s.MoveTask(machine, paneID, rule.Column, "trigger “"+rule.Name+"”: the pane printed "+line)
		if err != nil || !moved || s.Feed == nil {
			return
		}
		s.Feed(machine, paneID, words, "Trigger “"+rule.Name+"”: "+line)
	}
}

// spell says whether the watch may act on the pane now, and notes it when it may.
func (s *Service) spell(e events.OutputMatched, quiet time.Duration) bool {
	k := key(e.Pane.Machine, e.Pane.Pane) + "|" + e.Watch
	s.mu.Lock()
	defer s.mu.Unlock()
	if last, ok := s.acted[k]; ok && e.At.Sub(last) < quiet {
		return false
	}
	s.acted[k] = e.At
	return true
}

// offer keeps a dev server the row names as an offer to preview once the previews find its port listening; a few
// tries, since a server may say where it serves a moment before it does.
func (s *Service) offer(machine, paneID, row, by string) {
	port, url, ok := ServedAt(row)
	if !ok || s.Listening == nil {
		return
	}
	for _, wait := range listenTries {
		time.Sleep(wait)
		label, listening := s.Listening(machine, port)
		if !listening {
			continue
		}
		s.mu.Lock()
		if s.machines[machine][paneID] == nil {
			s.mu.Unlock()
			return // the pane went meanwhile
		}
		s.offers[key(machine, paneID)] = Offer{Port: port, URL: url, Label: label, By: by, At: time.Now()}
		s.mu.Unlock()
		if s.Changed != nil {
			s.Changed()
		}
		return
	}
}

// OfferOf is the preview a pane's dev server is offered as, while it stands; nil for none.
func (s *Service) OfferOf(machine, pane string) *Offer {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.offers[key(machine, pane)]
	if !ok || time.Since(o.At) > OfferKeep {
		return nil
	}
	return &o
}

func (s *Service) rule(id string) (Rule, bool) {
	for _, r := range s.Rules.List() {
		if r.ID == id && r.Enabled {
			return r, true
		}
	}
	return Rule{}, false
}

// Replace keeps the person's rules and has every stream take them up.
func (s *Service) Replace(list []Rule) ([]Rule, error) {
	out, err := s.Rules.Replace(list)
	if err == nil && s.Resubscribe != nil {
		s.Resubscribe()
	}
	return out, err
}

// Refused hears herdr refusing the person's watches on a machine, or taking them again (nil).
func (s *Service) Refused(machine string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		delete(s.refused, machine)
		return
	}
	s.refused[machine] = err.Error()
}

// Refusals is why herdr refused the person's watches, by machine.
func (s *Service) Refusals() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(s.refused))
	for machine, why := range s.refused {
		out[machine] = why
	}
	return out
}

// where names a pane's place for a person: its workspace, else its agent, else "a terminal".
func where(p paneView) string {
	switch {
	case p.workspace != "":
		return p.workspace
	case p.agent != "":
		return p.agent
	}
	return "a terminal"
}

// cut keeps the first n runes of s, an ellipsis after them.
func cut(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}
