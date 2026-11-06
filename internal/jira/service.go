package jira

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"
)

var (
	// Every is the least time between two reads of one issue.
	Every = 3 * time.Minute
	// WantedFor is how long a key stays worth reading after a roster last named it.
	WantedFor = 10 * time.Minute
	// ProjectsEvery is how long the list of the site's projects is trusted.
	ProjectsEvery = time.Hour
	// MaxReads is how many issues one sweep reads at most.
	MaxReads = 30
	// ComplainEvery spaces the log lines about a site that does not answer.
	ComplainEvery = time.Hour
)

// cached is the last read of one key: the issue, or nil when the site knows none the account may see.
type cached struct {
	issue *Issue
	at    time.Time
}

// Service is the configuration and what was read of the site: the projects and the issues the roster names, read in one
// background sweep at a time and only when a page looks.
type Service struct {
	Store *Store
	// OnChange hears that a sweep read something new, so the page's roster moves; the app sets it.
	OnChange func()

	http *http.Client
	now  func() time.Time

	mu         sync.Mutex
	issues     map[string]cached
	wanted     map[string]time.Time
	projects   map[string]bool
	projectsAt time.Time
	reading    bool
	generation int       // moves with every change of the configuration: a sweep begun before writes nothing
	quietUntil time.Time // after the site failed, no sweep before this
	complained time.Time
}

// New is the service over the configuration kept in dir.
func New(dir string) *Service {
	return &Service{Store: Load(dir), now: time.Now, issues: map[string]cached{}, wanted: map[string]time.Time{}}
}

// Configured says whether a site is set: until then nothing is contacted and no chip is drawn.
func (s *Service) Configured() bool { return s != nil && s.Store.Configured() }

// Site is the configured site's address, "" when none is.
func (s *Service) Site() string {
	if !s.Configured() {
		return ""
	}
	return s.Store.Config().Site
}

// Replace sets the configuration and forgets what was read with the one before.
func (s *Service) Replace(site, account, token string) error {
	if err := s.Store.Replace(site, account, token); err != nil {
		return err
	}
	s.forget()
	return nil
}

// Clear forgets the configuration and everything read with it.
func (s *Service) Clear() error {
	if err := s.Store.Clear(); err != nil {
		return err
	}
	s.forget()
	return nil
}

func (s *Service) forget() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.generation++
	s.issues = map[string]cached{}
	s.wanted = map[string]time.Time{}
	s.projects = nil
	s.projectsAt = time.Time{}
	s.quietUntil = time.Time{}
}

// Issues is what was read of the keys, in their order, keys of projects the site does not know left out; it notes the
// keys as wanted, so the next Touch reads the ones not read yet. It asks the site nothing itself.
func (s *Service) Issues(keys []string) []Issue {
	if len(keys) == 0 || !s.Configured() {
		return nil
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Issue
	for _, key := range keys {
		if s.projects != nil && !s.projects[Project(key)] {
			continue
		}
		s.wanted[key] = now
		if c, ok := s.issues[key]; ok && c.issue != nil {
			out = append(out, *c.issue)
		}
	}
	return out
}

// Touch says a page just read the roster: a sweep starts when the projects or any wanted issue are due, one at a time.
func (s *Service) Touch() {
	if !s.Configured() {
		return
	}
	now := s.now()
	s.mu.Lock()
	if s.reading || now.Before(s.quietUntil) {
		s.mu.Unlock()
		return
	}
	projectsDue := s.projects == nil || now.Sub(s.projectsAt) >= ProjectsEvery
	due := s.dueLocked(now)
	if !projectsDue && len(due) == 0 {
		s.mu.Unlock()
		return
	}
	s.reading = true
	generation := s.generation
	s.mu.Unlock()
	go s.sweep(generation, projectsDue)
}

// dueLocked is the wanted keys whose last read is Every old, or never happened, oldest wish first; a key not wanted
// for WantedFor is dropped, and what was read of it with it, so a long day of branches does not pile up.
func (s *Service) dueLocked(now time.Time) []string {
	var due []string
	for key, at := range s.wanted {
		if now.Sub(at) > WantedFor {
			delete(s.wanted, key)
			delete(s.issues, key)
			continue
		}
		if s.projects != nil && !s.projects[Project(key)] {
			continue
		}
		if c, ok := s.issues[key]; ok && now.Sub(c.at) < Every {
			continue
		}
		due = append(due, key)
	}
	sort.Strings(due)
	if len(due) > MaxReads {
		due = due[:MaxReads]
	}
	return due
}

// sweep reads the projects when due, then every wanted issue that is due, and tells OnChange when anything moved.
func (s *Service) sweep(generation int, projectsDue bool) {
	client := NewClient(s.Store.Config(), s.http)
	changed := false
	defer func() {
		s.mu.Lock()
		s.reading = false
		s.mu.Unlock()
		if changed && s.OnChange != nil {
			s.OnChange()
		}
	}()
	if projectsDue {
		ctx, cancel := context.WithTimeout(context.Background(), 4*Timeout)
		keys, err := client.Projects(ctx)
		cancel()
		if err != nil {
			s.complain(err)
			s.mu.Lock()
			known := s.projects != nil
			s.mu.Unlock()
			if !known {
				return
			}
		} else {
			set := make(map[string]bool, len(keys))
			for _, key := range keys {
				set[key] = true
			}
			s.mu.Lock()
			if s.generation != generation {
				s.mu.Unlock()
				return
			}
			s.projects, s.projectsAt = set, s.now()
			s.mu.Unlock()
			changed = true
		}
	}
	s.mu.Lock()
	due := s.dueLocked(s.now())
	s.mu.Unlock()
	for _, key := range due {
		ctx, cancel := context.WithTimeout(context.Background(), Timeout)
		issue, err := client.Issue(ctx, key)
		cancel()
		var read *Issue
		switch {
		case err == nil:
			read = &issue
		case errors.Is(err, ErrNotFound):
		default:
			s.complain(err)
			return
		}
		s.mu.Lock()
		if s.generation != generation {
			s.mu.Unlock()
			return
		}
		before, had := s.issues[key]
		s.issues[key] = cached{issue: read, at: s.now()}
		s.mu.Unlock()
		if !had || !sameIssue(before.issue, read) {
			changed = true
		}
	}
}

func sameIssue(a, b *Issue) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// complain holds the sweeps off for Every after the site failed, and logs it once an hour: never the token, never the
// full address.
func (s *Service) complain(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quietUntil = s.now().Add(Every)
	if s.now().Sub(s.complained) < ComplainEvery {
		return
	}
	s.complained = s.now()
	log.Printf("jira: %v", err)
}

// Test reads who the account is and how many projects it sees, through the configuration kept.
func (s *Service) Test(ctx context.Context) (string, int, error) {
	if !s.Configured() {
		return "", 0, errors.New("no Jira site is set")
	}
	client := NewClient(s.Store.Config(), s.http)
	who, err := client.Myself(ctx)
	if err != nil {
		return "", 0, err
	}
	projects, err := client.Projects(ctx)
	if err != nil {
		return who, 0, err
	}
	return who, len(projects), nil
}

// Full reads one issue whole, for a task.
func (s *Service) Full(ctx context.Context, key string) (Full, error) {
	if !s.Configured() {
		return Full{}, errors.New("no Jira site is set: add it in Settings")
	}
	return NewClient(s.Store.Config(), s.http).Full(ctx, key)
}
