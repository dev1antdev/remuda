package usage

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/state"
)

// The year's heatmap reads each machine's days once. A day that ended is kept in <state>/usage-days.json by machine and
// the viewer's zone, read after it ended and never again; today is read again at most every TodayFresh while a page
// looks. A first look on a long history reads in the background, the answer saying how many days are still being read.

const (
	YearDays    = 365
	TodayFresh  = 2 * time.Minute
	RetryAfter  = time.Minute
	DaysTimeout = 10 * time.Minute
	ShelfIdle   = 60 * 24 * time.Hour // a machine and zone nobody looked at for this long is dropped
	bookFile    = "usage-days.json"
)

// Book keeps every machine's days for the heatmap.
type Book struct {
	path string
	// Read is one machine's days since a moment; ReadHost with the days by default
	Read func(host shell.Host, since time.Time, loc *time.Location, tz string) (Report, error)
	Now  func() time.Time

	mu      sync.Mutex
	saving  sync.Mutex // one write at a time, each of the newest state
	shelves map[string]*shelf
	reads   sync.WaitGroup
}

// shelf is one machine's days in one zone.
type shelf struct {
	Machine string         `json:"machine"`
	Zone    string         `json:"zone"`
	Looked  time.Time      `json:"looked"`
	Days    map[string]Day `json:"days"` // the days that ended, each read after it ended; a day nobody worked is kept empty

	today     *Day
	todayDate string
	todayAt   time.Time
	busy      bool
	pending   int // the ended days never read, being read now
	failed    string
	failedAt  time.Time
}

// YearHost is a machine of the fleet as the heatmap asks it: one away is answered from what was kept.
type YearHost struct {
	shell.Host
	Away bool
}

// OpenBook reads what was kept; a damaged file is an empty book, said once.
func OpenBook(dir string) *Book {
	b := &Book{path: filepath.Join(dir, bookFile), shelves: map[string]*shelf{}, Now: time.Now}
	b.Read = func(host shell.Host, since time.Time, loc *time.Location, tz string) (Report, error) {
		return ReadHost(host, since, loc, tz, true, DaysTimeout)
	}
	data, err := os.ReadFile(b.path)
	if errors.Is(err, os.ErrNotExist) {
		return b
	}
	var kept struct {
		Shelves []*shelf `json:"shelves"`
	}
	if err != nil || json.Unmarshal(data, &kept) != nil {
		log.Printf("usage: %s could not be read; the year's days are read again", b.path)
		return b
	}
	for _, s := range kept.Shelves {
		if s.Days == nil {
			s.Days = map[string]Day{}
		}
		b.shelves[shelfKey(s.Machine, s.Zone)] = s
	}
	return b
}

func shelfKey(machine, zone string) string { return machine + "|" + zone }

// dayOf is a moment's local day.
func dayOf(t time.Time, loc *time.Location) string { return t.In(loc).Format("2006-01-02") }

// yearDays is the last YearDays local days, the oldest first, today last.
func yearDays(now time.Time, loc *time.Location) []string {
	local := now.In(loc)
	out := make([]string, YearDays)
	for i := range out {
		out[i] = time.Date(local.Year(), local.Month(), local.Day()-(YearDays-1-i), 12, 0, 0, 0, loc).Format("2006-01-02")
	}
	return out
}

// midnight is the start of a local day.
func midnight(date string, loc *time.Location) time.Time {
	t, _ := time.ParseInLocation("2006-01-02", date, loc)
	return t
}

// Look answers the year on every machine from what is kept, and starts in the background the reads it lacks: the ended
// days never read, and today once its reading is TodayFresh old.
func (b *Book) Look(hosts []YearHost, loc *time.Location, tz string) Year {
	now := b.Now()
	days := yearDays(now, loc)
	today := days[len(days)-1]
	year := Year{From: days[0], To: today, Zone: loc.String(), Names: map[string]string{}, Machines: []MachineYear{}}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prune(now)
	for _, host := range hosts {
		key := shelfKey(host.ID(), loc.String())
		s := b.shelves[key]
		if s == nil {
			s = &shelf{Machine: host.ID(), Zone: loc.String(), Days: map[string]Day{}}
			b.shelves[key] = s
		}
		s.Looked = now
		if !host.Away {
			b.readIfDue(host.Host, s, days, now, loc, tz)
		}
		year.Machines = append(year.Machines, s.year(host, days, today, year.Names))
	}
	return year
}

// readIfDue starts a read of the days the shelf lacks, and of today once its reading is old; one at a time per shelf,
// a failed one tried again after RetryAfter.
func (b *Book) readIfDue(host shell.Host, s *shelf, days []string, now time.Time, loc *time.Location, tz string) {
	if s.busy || (s.failed != "" && now.Sub(s.failedAt) < RetryAfter) {
		return
	}
	today := days[len(days)-1]
	missing, first := 0, ""
	for _, date := range days[:len(days)-1] {
		if _, ok := s.Days[date]; !ok {
			missing++
			if first == "" {
				first = date
			}
		}
	}
	if missing == 0 && s.todayDate == today && now.Sub(s.todayAt) < TodayFresh {
		return
	}
	if first == "" {
		first = today
	}
	s.busy, s.pending = true, missing
	b.reads.Add(1)
	go func() {
		defer b.reads.Done()
		rep, err := b.Read(host, midnight(first, loc), loc, tz)
		b.shelve(s, first, today, now, rep, err)
	}()
}

// shelve keeps what a read brought: every day from its first to the one before the day it began on, as ended days, and
// that day as today's reading.
func (b *Book) shelve(s *shelf, first, today string, began time.Time, rep Report, err error) {
	b.mu.Lock()
	s.busy, s.pending = false, 0
	if err != nil {
		if s.failed == "" {
			log.Printf("usage: the year's days on %s could not be read: %v", s.Machine, err)
		}
		s.failed, s.failedAt = err.Error(), began
		b.mu.Unlock()
		return
	}
	s.failed = ""
	read := map[string]Day{}
	for _, day := range rep.Daily {
		read[day.Date] = day
	}
	ended := false
	for date := first; date < today; date = nextDay(date) {
		day, ok := read[date]
		if !ok {
			day = Day{Date: date, Folders: []DayFolder{}}
		}
		s.Days[date] = day
		ended = true
	}
	day := read[today]
	day.Date = today
	s.today, s.todayDate, s.todayAt = &day, today, began
	b.mu.Unlock()
	if ended {
		b.save()
	}
}

// nextDay is the date after a YYYY-MM-DD one.
func nextDay(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return "9999-12-31"
	}
	return t.AddDate(0, 0, 1).Format("2006-01-02")
}

// prune drops the days older than a year and the shelves nobody looked at for ShelfIdle.
func (b *Book) prune(now time.Time) {
	for key, s := range b.shelves {
		if !s.Looked.IsZero() && now.Sub(s.Looked) > ShelfIdle && !s.busy {
			delete(b.shelves, key)
			continue
		}
		loc, err := time.LoadLocation(s.Zone)
		if err != nil {
			loc = time.UTC
		}
		oldest := yearDays(now, loc)[0]
		for date := range s.Days {
			if date < oldest {
				delete(s.Days, date)
			}
		}
	}
}

// Forget drops a removed machine's days.
func (b *Book) Forget(machine string) {
	b.mu.Lock()
	for key, s := range b.shelves {
		if s.Machine == machine {
			delete(b.shelves, key)
		}
	}
	b.mu.Unlock()
	b.save()
}

// Wait is for the reads under way to end: a test's, or a shutdown's.
func (b *Book) Wait() { b.reads.Wait() }

func (b *Book) save() {
	b.saving.Lock()
	defer b.saving.Unlock()
	b.mu.Lock()
	kept := struct {
		Shelves []*shelf `json:"shelves"`
	}{Shelves: make([]*shelf, 0, len(b.shelves))}
	for _, s := range b.shelves {
		kept.Shelves = append(kept.Shelves, s)
	}
	data, err := json.Marshal(kept)
	b.mu.Unlock()
	if err == nil {
		err = state.WriteFile(b.path, data)
	}
	if err != nil {
		log.Printf("usage: %s not written: %v", b.path, err)
	}
}
