// Package mirror shows inside herdr what remuda knows of each agent, so the terminal at the desk says what the phone
// says: a pane's cost, context, model, pull request, task, the watchdog's note and its last answer as herdr tokens a
// person's sidebar rows name, the note as the label of the state it stands in, a workspace's review and cost, and —
// when the person asks — remuda's alerts as herdr's own toasts on the machine the agent runs on. Everything is written
// under the source "remuda", only when a value moved, again before its TTL runs out, by one writer per machine paced
// to a few writes a second, off the watcher's path; when remuda stops, what it wrote lapses by itself.
package mirror

import (
	"log"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/state"
)

// Source is the name remuda's values carry in herdr.
const Source = "remuda"

// How long herdr keeps a value, how old a write may grow before it is written again, and the pace of the writes.
const (
	TTL             = 5 * time.Minute
	Refresh         = 3 * time.Minute
	WritesPerSecond = 5
	staleAfter      = 3 * time.Second // a snapshot read this long after a write shows it, or herdr lost it
	refusedPause    = 10 * time.Minute
	tasksEvery      = 15 * time.Second
	warmUp          = 30 * time.Second // a writer just made waits it out: the services it reads are still reading
)

const settingsFile = "mirror.json"

// Settings are the cockpit's two switches and the toasts' quiet hours: one for every machine, since herdr's own
// ui.toast.delivery already says, machine by machine, whether a toast shows there at all.
type Settings struct {
	Figures bool        `json:"figures"` // remuda's figures as herdr tokens; on unless the person turned it off
	Toasts  bool        `json:"toasts"`  // remuda's alerts as herdr toasts too; off until the person turns it on
	Quiet   *push.Quiet `json:"quiet,omitempty"`
}

// Status is the settings as the page reads them, with what it needs to explain them.
type Status struct {
	Settings
	Closed       string    `json:"closed,omitempty"` // why this remuda writes nothing into herdr
	Tokens       []string  `json:"tokens"`
	Snippet      string    `json:"snippet"`
	ToastSnippet string    `json:"toast_snippet"`
	Last         []Outcome `json:"last,omitempty"`      // the newest toast on each machine
	Elsewhere    []Yield   `json:"elsewhere,omitempty"` // the machines another remuda writes into herdr on
}

// Service is the mirror: the switches, a writer per machine, the toasts' outcomes.
type Service struct {
	dir    string
	in     Inputs
	closed string
	now    func() time.Time
	sleep  func(time.Duration)

	mu       sync.Mutex
	settings Settings
	writers  map[string]*writer
	last     map[string]Outcome
	shut     bool
}

// New loads the switches from dir; closed, when set, says why this remuda writes nothing at all (a scratch instance
// beside the real one, which would fight it over the same panes).
func New(dir string, in Inputs, closed string) *Service {
	s := &Service{dir: dir, in: in, closed: closed, now: time.Now, sleep: time.Sleep,
		settings: Settings{Figures: true}, writers: map[string]*writer{}, last: map[string]Outcome{}}
	if err := state.LoadJSON(dir, settingsFile, &s.settings); err != nil {
		log.Printf("%s is unreadable (%v): remuda's figures go into herdr, its alerts do not, until Settings says otherwise", settingsFile, err)
		s.settings = Settings{Figures: true}
	}
	if s.settings.Quiet != nil && !s.settings.Quiet.Valid() {
		s.settings.Quiet = nil
	}
	return s
}

// Settings are the switches as they stand.
func (s *Service) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

// Put saves the switches and has every writer act on them at once: off clears what remuda wrote.
func (s *Service) Put(next Settings) error {
	if next.Quiet != nil && !next.Quiet.Valid() {
		next.Quiet = nil
	}
	if err := state.SaveJSON(s.dir, settingsFile, next); err != nil {
		return err
	}
	s.mu.Lock()
	s.settings = next
	writers := make([]*writer, 0, len(s.writers))
	for _, w := range s.writers {
		writers = append(writers, w)
	}
	s.mu.Unlock()
	for _, w := range writers {
		w.poke()
	}
	return nil
}

// Status is what the Settings section shows.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{Settings: s.settings, Closed: s.closed, Tokens: append(append([]string{}, PaneTokens...), TokenReview),
		Snippet: Snippet, ToastSnippet: ToastSnippet}
	for _, m := range s.machines() {
		if outcome, ok := s.last[m.ID]; ok {
			st.Last = append(st.Last, outcome)
		}
		if w := s.writers[m.ID]; w != nil {
			if other := w.yielding(); other != "" {
				st.Elsewhere = append(st.Elsewhere, Yield{Machine: m.ID, Label: m.Label, Writer: writerName(other)})
			}
		}
	}
	return st
}

func (s *Service) local(machineID string) bool { return s.in.Local != nil && s.in.Local(machineID) }

func (s *Service) machines() []Machine {
	if s.in.Machines == nil {
		return nil
	}
	return s.in.Machines()
}

// Observe hands a machine's snapshot to its writer, which takes the newest one waiting when it is free.
func (s *Service) Observe(e events.Snapshot) {
	if s.closed != "" || e.Snap == nil {
		return
	}
	s.mu.Lock()
	if s.shut {
		s.mu.Unlock()
		return
	}
	w := s.writers[e.Machine]
	if w == nil {
		w = newWriter(e.Machine, s)
		s.writers[e.Machine] = w
		go w.run()
	}
	s.mu.Unlock()
	w.offer(e.Snap, e.At)
}

// Forget ends a removed machine's writer; what it wrote lapses with the TTL.
func (s *Service) Forget(machineID string) {
	s.mu.Lock()
	w := s.writers[machineID]
	delete(s.writers, machineID)
	delete(s.last, machineID)
	s.mu.Unlock()
	if w != nil {
		w.end()
	}
}

// Close ends every writer; what they wrote lapses with the TTL, so a restart a minute later shows no gap.
func (s *Service) Close() {
	s.mu.Lock()
	s.shut = true
	writers := s.writers
	s.writers = map[string]*writer{}
	s.mu.Unlock()
	for _, w := range writers {
		w.end()
	}
}
