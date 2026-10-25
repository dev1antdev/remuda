// Package budget keeps spending limits — a sum per day, week or month over every machine, one machine or one of
// its folders — reads what the period has cost so far through the usage report, and says when a limit is near
// or crossed. Prices are API-equivalent, as everywhere in remuda: on a subscription a measure, not a bill.
package budget

import (
	"errors"
	"fmt"
	"log"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/state"
	"github.com/dev1antdev/remuda/internal/transcript"
	"github.com/dev1antdev/remuda/internal/usage"
)

const file = "budgets.json"

// Budget is one limit: its scope (every machine, one machine, one folder on it), the sum, the period, and which
// thresholds already fired for which period start.
type Budget struct {
	ID      string           `json:"id"`
	Machine string           `json:"machine,omitempty"` // "" = every machine
	Dir     string           `json:"dir,omitempty"`     // "" = every folder; only with a machine
	Limit   float64          `json:"limit"`             // USD, API-equivalent
	Period  string           `json:"period"`            // day | week | month
	Note    string           `json:"note,omitempty"`
	Alerted map[string][]int `json:"alerted,omitempty"` // period start (RFC 3339) → the thresholds that fired
}

// Status is a budget with what its period has cost so far.
type Status struct {
	Budget
	Spent     float64  `json:"spent"`
	Start     string   `json:"start"`               // the period's first moment, RFC 3339
	Collected string   `json:"collected,omitempty"` // when the spend was read; "" while it is not known yet
	Missing   []string `json:"missing,omitempty"`   // machines in the scope whose report the reading could not get: not counted
}

// Thresholds are the percentages of a limit that each push once per period, the higher one alone when both are
// crossed at once.
var Thresholds = []int{80, 100}

// Periods are the words a budget's period may be.
var Periods = []string{"day", "week", "month"}

// PeriodStart is the first moment of the period now falls in, by the clock of loc: the local midnight, the week's
// Monday, the month's first.
func PeriodStart(period string, now time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	t := now.In(loc)
	y, m, d := t.Date()
	switch period {
	case "week":
		back := (int(t.Weekday()) + 6) % 7 // Monday = 0 … Sunday = 6
		return time.Date(y, m, d-back, 0, 0, 0, 0, loc)
	case "month":
		return time.Date(y, m, 1, 0, 0, 0, 0, loc)
	default:
		return time.Date(y, m, d, 0, 0, 0, 0, loc)
	}
}

// Service holds the budgets and the spend of their periods, read in the background every so often and whenever
// a budget changes; Status answers from the cache, so the roster never waits on a scan.
type Service struct {
	dir string

	Hosts  func() []shell.Host // the connected machines
	Notify func(push.Payload)
	Loc    *time.Location

	collect func(hosts []shell.Host, since time.Time, loc *time.Location, tz string) []usage.MachineReport
	now     func() time.Time

	mu     sync.Mutex
	all    []Budget
	spends map[string]*spend // by period

	reading  sync.Mutex     // one reading at a time; a caller waits for the one under way rather than skipping
	readings sync.WaitGroup // the background readings Refresh started, for Wait
}

// spend is one period's reading: when it started, when it was read, and every machine's report or its failure.
type spend struct {
	start   time.Time
	at      time.Time
	reports map[string]*usage.Report
	errors  map[string]string
}

// New loads the budgets of the state dir; a damaged file keeps none until it is fixed.
func New(dir string, hosts func() []shell.Host, notify func(push.Payload)) *Service {
	s := &Service{dir: dir, Hosts: hosts, Notify: notify, Loc: time.Local, collect: usage.Collect, now: time.Now, spends: map[string]*spend{}}
	if err := state.LoadJSON(dir, file, &s.all); err != nil {
		log.Printf("%s is unreadable (%v): no budgets until it is fixed", file, err)
	}
	if s.all == nil {
		s.all = []Budget{}
	}
	return s
}

// List is the budgets as stored.
func (s *Service) List() []Budget {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyOf(s.all)
}

func copyOf(list []Budget) []Budget {
	return append(make([]Budget, 0, len(list)), list...)
}

// Replace keeps the whole list as given, checked and trimmed. A budget whose scope, limit and period did not change
// keeps its alert state; one that did starts the period afresh, so a raised limit can be crossed again.
func (s *Service) Replace(list []Budget) ([]Status, error) {
	out := make([]Budget, 0, len(list))
	seen := map[string]bool{}
	s.mu.Lock()
	before := map[string]Budget{}
	for _, b := range s.all {
		before[b.ID] = b
	}
	s.mu.Unlock()
	for _, b := range list {
		b.Machine, b.Dir, b.Note, b.Period = strings.TrimSpace(b.Machine), strings.TrimSpace(b.Dir), strings.TrimSpace(b.Note), strings.TrimSpace(b.Period)
		if !validPeriod(b.Period) {
			return nil, fmt.Errorf("a budget's period must be day, week or month, not %q", b.Period)
		}
		if !(b.Limit > 0) || math.IsInf(b.Limit, 0) {
			return nil, errors.New("a budget's limit must be above zero")
		}
		if b.Dir != "" && b.Machine == "" {
			return nil, errors.New("a budget on a folder names its machine too")
		}
		for b.ID == "" || seen[b.ID] {
			b.ID = state.NewID()
		}
		seen[b.ID] = true
		b.Alerted = nil
		if was, ok := before[b.ID]; ok && was.Machine == b.Machine && was.Dir == b.Dir && was.Limit == b.Limit && was.Period == b.Period {
			b.Alerted = was.Alerted
		}
		out = append(out, b)
	}
	s.mu.Lock()
	if err := state.SaveJSON(s.dir, file, out); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.all = out
	s.mu.Unlock()
	s.Refresh()
	return s.Status(), nil
}

func validPeriod(p string) bool {
	for _, known := range Periods {
		if p == known {
			return true
		}
	}
	return false
}

// Status is every budget with what its period has cost so far, from the cache; a period not read yet, or read for
// an earlier start, asks for a reading in the background and answers without a figure.
func (s *Service) Status() []Status {
	now := s.now()
	s.mu.Lock()
	out := make([]Status, 0, len(s.all))
	stale := false
	for _, b := range s.all {
		st := Status{Budget: b}
		start := PeriodStart(b.Period, now, s.Loc)
		st.Start = start.Format(time.RFC3339)
		if sp := s.spends[b.Period]; sp != nil && sp.start.Equal(start) {
			st.Spent = spentOf(sp, b)
			st.Collected = sp.at.UTC().Format(time.RFC3339)
			st.Missing = missingOf(sp, b)
		} else {
			stale = true
		}
		out = append(out, st)
	}
	s.mu.Unlock()
	if stale {
		s.Refresh()
	}
	return out
}

// spentOf is what the reading says the budget's scope cost: every machine's total, one machine's, or the projects
// of one folder (named by the folder the records say, or by Claude Code's encoding of it as the project directory).
func spentOf(sp *spend, b Budget) float64 {
	sum := 0.0
	for id, rep := range sp.reports {
		if b.Machine != "" && id != b.Machine {
			continue
		}
		if b.Dir == "" {
			sum += rep.Total.Cost
			continue
		}
		encoded := transcript.EncodePath(b.Dir)
		for _, p := range rep.Projects {
			if p.Dir == b.Dir || p.Project == encoded {
				sum += p.Cost
			}
		}
	}
	return sum
}

// missingOf names the machines of the budget's scope that gave no report, in a stable order.
func missingOf(sp *spend, b Budget) []string {
	var out []string
	for id := range sp.errors {
		if b.Machine == "" || id == b.Machine {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// Refresh reads the spend of every period in use in the background, after any reading under way.
func (s *Service) Refresh() {
	s.readings.Add(1)
	go func() {
		defer s.readings.Done()
		s.refresh()
	}()
}

// Wait blocks until the background readings Refresh started are done (tests).
func (s *Service) Wait() { s.readings.Wait() }

// Loop reads the spend every so often while budgets exist.
func (s *Service) Loop(every time.Duration) {
	for {
		s.refresh()
		time.Sleep(every)
	}
}

func (s *Service) refresh() {
	s.reading.Lock()
	defer s.reading.Unlock()
	s.mu.Lock()
	if len(s.all) == 0 {
		s.mu.Unlock()
		return
	}
	periods := map[string]bool{}
	for _, b := range s.all {
		periods[b.Period] = true
	}
	s.mu.Unlock()

	now := s.now()
	hosts := s.Hosts()
	tz := s.Loc.String()
	if tz == "Local" {
		tz = "" // "Local" means nothing to another machine; empty leaves it its own
	}
	names := make([]string, 0, len(periods))
	for p := range periods {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, period := range names {
		start := PeriodStart(period, now, s.Loc)
		sp := &spend{start: start, at: s.now(), reports: map[string]*usage.Report{}, errors: map[string]string{}}
		for _, rep := range s.collect(hosts, start, s.Loc, tz) {
			if rep.Report != nil {
				sp.reports[rep.ID] = rep.Report
			} else {
				sp.errors[rep.ID] = rep.Error
			}
		}
		s.mu.Lock()
		s.spends[period] = sp
		s.mu.Unlock()
	}
	s.alert(hosts, now)
}

// alert fires the thresholds a budget crossed in its current period and not yet announced — the higher one alone
// when both are crossed in one reading — and forgets the periods before it.
func (s *Service) alert(hosts []shell.Host, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	var fire []push.Payload
	for i := range s.all {
		b := &s.all[i]
		sp := s.spends[b.Period]
		start := PeriodStart(b.Period, now, s.Loc)
		if sp == nil || !sp.start.Equal(start) {
			continue
		}
		key := start.Format(time.RFC3339)
		if len(b.Alerted) > 1 || (len(b.Alerted) == 1 && b.Alerted[key] == nil) {
			b.Alerted = map[string][]int{}
			changed = true
		}
		spent := spentOf(sp, *b)
		pct := 100 * spent / b.Limit
		crossed := 0
		for _, th := range Thresholds {
			if pct >= float64(th) && !contains(b.Alerted[key], th) {
				crossed = th
			}
		}
		if crossed == 0 {
			continue
		}
		if b.Alerted == nil {
			b.Alerted = map[string][]int{}
		}
		for _, th := range Thresholds {
			if th <= crossed && !contains(b.Alerted[key], th) {
				b.Alerted[key] = append(b.Alerted[key], th)
			}
		}
		changed = true
		fire = append(fire, push.Payload{
			Machine: b.Machine, Kind: "budget", Tag: "budget|" + b.ID,
			Title: alertTitle(crossed, spent, *b),
			Body:  alertBody(*b, hosts),
		})
	}
	if changed {
		if err := state.SaveJSON(s.dir, file, s.all); err != nil {
			log.Printf("budgets: %v", err)
		}
	}
	if s.Notify != nil {
		for _, p := range fire {
			s.Notify(p)
		}
	}
}

func contains(list []int, n int) bool {
	for _, x := range list {
		if x == n {
			return true
		}
	}
	return false
}

// alertTitle: "Budget crossed: $21.30 of $20 today" at the limit, "Budget at 82 %: $16.40 of $20 this week" before it.
func alertTitle(threshold int, spent float64, b Budget) string {
	if threshold >= 100 {
		return fmt.Sprintf("Budget crossed: %s of %s %s", USD(spent), USD(b.Limit), PeriodWords(b.Period))
	}
	return fmt.Sprintf("Budget at %d %%: %s of %s %s", int(math.Floor(100*spent/b.Limit+1e-9)), USD(spent), USD(b.Limit), PeriodWords(b.Period))
}

// alertBody is the budget's note, else its scope in words.
func alertBody(b Budget, hosts []shell.Host) string {
	if b.Note != "" {
		return b.Note
	}
	return Scope(b, func(id string) string {
		for _, h := range hosts {
			if h.ID() == id {
				return h.Label()
			}
		}
		return id
	})
}

// Scope says what a budget covers: "every machine", a machine's name, or "<machine> · <folder>".
func Scope(b Budget, label func(id string) string) string {
	switch {
	case b.Machine == "":
		return "every machine"
	case b.Dir == "":
		return label(b.Machine)
	default:
		return label(b.Machine) + " · " + filepath.Base(b.Dir)
	}
}

// PeriodWords is the period as the sentence wants it: today, this week, this month.
func PeriodWords(period string) string {
	switch period {
	case "week":
		return "this week"
	case "month":
		return "this month"
	default:
		return "today"
	}
}

// USD writes a sum the way the page does: whole dollars from a hundred, one decimal from ten, else cents.
func USD(n float64) string {
	switch {
	case n >= 100:
		return fmt.Sprintf("$%.0f", n)
	case n >= 10:
		return fmt.Sprintf("$%.1f", n)
	default:
		return fmt.Sprintf("$%.2f", n)
	}
}
