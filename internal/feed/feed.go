// Package feed keeps what happened across the agents while nobody was looking: every alert the pushes carry — a
// finish, a question, a watchdog note, a schedule that failed, a task that moved — as one list in the state dir,
// newest first and three hundred at most, so the page can tell the day's story and the bell can say there is news.
package feed

import (
	"log"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/state"
)

const (
	file = "feed.json"
	// Max is how many entries are kept: a few days of a busy fleet.
	Max = 300
)

// Entry is one thing that happened, as the push for it would have said.
type Entry struct {
	ID      int64     `json:"id"`
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"` // input | done | stuck | waiting | loop | schedule | task | test | budget
	Machine string    `json:"machine"`
	Pane    string    `json:"pane,omitempty"`
	Title   string    `json:"title"`
	Body    string    `json:"body,omitempty"`
}

// Service is the list, loaded once and saved on every addition.
type Service struct {
	dir string
	mu  sync.Mutex
	all []Entry // newest first
}

func New(dir string) *Service {
	s := &Service{dir: dir}
	if err := state.LoadJSON(dir, file, &s.all); err != nil {
		log.Printf("%s is unreadable (%v): the activity starts empty", file, err)
		s.all = nil
	}
	return s
}

// Add puts an entry first, stamped now and numbered after the newest, and keeps the list at Max.
func (s *Service) Add(e Entry) Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	e.At = time.Now()
	e.ID = 1
	if len(s.all) > 0 {
		e.ID = s.all[0].ID + 1
	}
	s.all = append([]Entry{e}, s.all...)
	if len(s.all) > Max {
		s.all = s.all[:Max]
	}
	if err := state.SaveJSON(s.dir, file, s.all); err != nil {
		log.Printf("saving %s: %v", file, err)
	}
	return e
}

// List is the newest entries, at most limit, never nil.
func (s *Service) List(limit int) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > len(s.all) {
		limit = len(s.all)
	}
	return append(make([]Entry, 0, limit), s.all[:limit]...)
}

// LastID is the newest entry's number, 0 when there is none: the roster carries it so a page knows there is news.
func (s *Service) LastID() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.all) == 0 {
		return 0
	}
	return s.all[0].ID
}
