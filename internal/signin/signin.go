// Package signin warns before Claude Code's sign-in on a machine runs out, since an agent started after the end — a
// scout at three in the morning, a queued task — cannot reach Claude. Claude Code names the latest moment the sign-in
// ends and says it may end up to EarlyBy before it, so the warnings count to that earliest moment: one push per
// machine three days before it, one a day before and one once it may end at any time, kept across restarts so a
// restart never warns twice. remuda never signs in for the person; the words say where to run `claude auth login`.
package signin

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/state"
)

const stateFile = "signin.json"

// The warnings, in the order they come.
const (
	Soon  = 1 // three days or less to the earliest end
	Day   = 2 // a day or less to it
	Ended = 3 // it may end at any time now, or has ended
)

// SoonBefore and DayBefore are how long before the earliest end the first two warnings come.
var (
	SoonBefore = 72 * time.Hour
	DayBefore  = 24 * time.Hour
)

// EarlyBy is how long before the end `claude auth status` names the sign-in may already end: Claude Code says it
// "ends between" that day and the end, and that renewing it moves neither (2.1.318's loginExpiresAt).
var EarlyBy = 72 * time.Hour

// Earliest is the first moment a sign-in that ends by expires may end.
func Earliest(expires time.Time) time.Time { return expires.Add(-EarlyBy) }

// Level is the warning an end earns at now, counted to the earliest moment it may come: Ended once that has passed,
// Day within a day of it, Soon within three days, else 0.
func Level(expires, now time.Time) int {
	left := Earliest(expires).Sub(now)
	switch {
	case left <= 0:
		return Ended
	case left <= DayBefore:
		return Day
	case left <= SoonBefore:
		return Soon
	}
	return 0
}

// warned is what a machine was told of the sign-in ending at Expires: a later end, after a new sign-in, starts afresh.
type warned struct {
	Expires time.Time `json:"expires"`
	Level   int       `json:"level"`
}

// Service keeps the warnings given, by machine, in the state dir.
type Service struct {
	dir string
	now func() time.Time

	Notify func(push.Payload) // the warnings, through the devices that want the watchdog's word and the webhooks

	mu   sync.Mutex
	kept map[string]warned
}

// New loads the warnings kept in dir ("" keeps them in memory only).
func New(dir string) *Service {
	s := &Service{dir: dir, now: time.Now, kept: map[string]warned{}}
	if dir == "" {
		return s
	}
	if err := state.LoadJSON(dir, stateFile, &s.kept); err != nil {
		log.Printf("%s is unreadable (%v): no sign-in warning is remembered as given", stateFile, err)
		s.kept = map[string]warned{}
	}
	if s.kept == nil {
		s.kept = map[string]warned{}
	}
	return s
}

// Observe hears a machine's sign-in as the vitals last read it (zero: none known) and warns when it crossed a
// threshold not warned of yet for that end; a machine first seen past two thresholds hears the graver alone.
func (s *Service) Observe(machine, label string, expires time.Time) {
	if expires.IsZero() {
		return
	}
	now := s.now()
	level := Level(expires, now)
	s.mu.Lock()
	w, had := s.kept[machine]
	moved := !had || !w.Expires.Equal(expires)
	if moved {
		w = warned{Expires: expires}
	}
	due := level > w.Level
	if due {
		w.Level = level
	}
	if moved || due {
		s.kept[machine] = w
		s.saveLocked()
	}
	s.mu.Unlock()
	if due && s.Notify != nil {
		s.Notify(Warning(machine, label, expires, now, level))
	}
}

// Forget drops a removed machine's warnings.
func (s *Service) Forget(machine string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.kept[machine]; ok {
		delete(s.kept, machine)
		s.saveLocked()
	}
}

// Warning is the push for a level: how long is left to the earliest end and to the latest, that it may end at any
// time, or that it has ended, and what to run where.
func Warning(machine, label string, expires, now time.Time, level int) push.Payload {
	p := push.Payload{Machine: machine, Kind: "signin", Tag: "signin|" + machine}
	switch {
	case !now.Before(expires):
		p.Title = fmt.Sprintf("Claude Code's sign-in on %s has ended", label)
		p.Body = fmt.Sprintf("Its agents cannot reach Claude until you sign in again: run claude auth login on %s.", label)
	case level == Ended:
		p.Title = fmt.Sprintf("Claude Code's sign-in on %s may end at any time now", label)
		p.Body = fmt.Sprintf("It ends within %s at the latest, and its agents cannot reach Claude after that: run claude auth login on %s now.", Left(expires.Sub(now)), label)
	default:
		p.Title = fmt.Sprintf("Claude Code's sign-in on %s may end in %s", label, Left(Earliest(expires).Sub(now)))
		p.Body = fmt.Sprintf("It ends within %s at the latest; an agent started after it cannot reach Claude: run claude auth login on %s before then.", Left(expires.Sub(now)), label)
	}
	return p
}

// Left says how long is left: "3 days", "20 hours", "45 minutes".
func Left(d time.Duration) string {
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%d days", int(math.Round(d.Hours()/24)))
	case d >= 2*time.Hour:
		return fmt.Sprintf("%d hours", int(math.Round(d.Hours())))
	case d >= 2*time.Minute:
		return fmt.Sprintf("%d minutes", int(math.Round(d.Minutes())))
	}
	return "a minute"
}

// saveLocked writes the warnings; the caller holds mu
func (s *Service) saveLocked() {
	if s.dir == "" {
		return
	}
	if err := state.SaveJSON(s.dir, stateFile, s.kept); err != nil {
		log.Printf("%s: %v", stateFile, err)
	}
}
