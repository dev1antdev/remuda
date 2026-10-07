// Package schedule sends prompts to agents on a timetable: "every weekday
// at 9:00, run the tests and sum up", "at 18:00, status and what is
// blocked". A schedule names a machine and a workspace folder, not a pane
// — panes come and go, the project stays — and at run time the prompt goes
// to the agent running in that folder through the same path the chat
// uses. Times are five-field cron in the server's local time. Schedules
// live in <state>/schedules.json; a run is never queued or retried: a
// minute with no agent, or an agent busy with a question, is noted and
// pushed, and the next run is the next match.
package schedule

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/herdr-ui/internal/push"
	"github.com/dev1antdev/herdr-ui/internal/state"
	"github.com/dev1antdev/herdr-ui/internal/text"
)

const file = "schedules.json"

type Schedule struct {
	ID      string `json:"id"`
	Machine string `json:"machine"`
	Dir     string `json:"dir"`    // the workspace folder; the agent in it gets the prompt
	Label   string `json:"label"`  // what the user calls it
	Cron    string `json:"cron"`   // "0 9 * * 1-5"
	Prompt  string `json:"prompt"` // what the agent gets
	Enabled bool   `json:"enabled"`

	Next       time.Time `json:"next,omitzero"` // the coming run, filled in by List
	LastRun    time.Time `json:"last_run,omitzero"`
	LastResult string    `json:"last_result,omitempty"` // "sent to <pane title>" or why not
	LastOK     bool      `json:"last_ok"`
}

// Submit delivers a prompt to the agent in a folder and names the pane it
// went to.
type Submit func(machineID, dir, text string) (pane string, err error)

type Store struct {
	dir    string
	submit Submit
	notify func(push.Payload)
	now    func() time.Time

	mu       sync.Mutex
	items    map[string]*Schedule
	inflight sync.WaitGroup
}

func New(dir string, submit Submit, notify func(push.Payload)) *Store {
	st := &Store{dir: dir, submit: submit, notify: notify, now: time.Now, items: map[string]*Schedule{}}
	var list []*Schedule
	if err := state.LoadJSON(dir, file, &list); err != nil {
		log.Printf("%s is unreadable (%v): no schedules until it is fixed", file, err)
	}
	for _, s := range list {
		st.items[s.ID] = s
	}
	return st
}

func (st *Store) saveLocked() error {
	return state.SaveJSON(st.dir, file, st.listLocked())
}

func (st *Store) listLocked() []*Schedule {
	out := make([]*Schedule, 0, len(st.items))
	for _, s := range st.items {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Label < out[j].Label || (out[i].Label == out[j].Label && out[i].ID < out[j].ID)
	})
	return out
}

// List returns copies, with the next run filled in.
func (st *Store) List() []Schedule {
	st.mu.Lock()
	defer st.mu.Unlock()
	now := st.now()
	out := make([]Schedule, 0, len(st.items))
	for _, s := range st.listLocked() {
		cp := *s
		if c, err := Parse(cp.Cron); err == nil && cp.Enabled {
			if next, ok := c.Next(now); ok {
				cp.Next = next
			}
		}
		out = append(out, cp)
	}
	return out
}

// Put creates or replaces a schedule after checking it.
func (st *Store) Put(s Schedule) (Schedule, error) {
	s.Label = strings.TrimSpace(s.Label)
	s.Prompt = strings.TrimSpace(s.Prompt)
	s.Cron = strings.Join(strings.Fields(s.Cron), " ")
	if s.Machine == "" || !strings.HasPrefix(s.Dir, "/") {
		return s, errors.New("a machine and an absolute folder are required")
	}
	if s.Prompt == "" {
		return s, errors.New("the prompt is empty")
	}
	if _, err := Parse(s.Cron); err != nil {
		return s, err
	}
	if s.Label == "" {
		s.Label = text.FirstLine(s.Prompt, 60)
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if s.ID == "" {
		s.ID = state.NewID()
	} else if old, ok := st.items[s.ID]; ok {
		s.LastRun, s.LastResult, s.LastOK = old.LastRun, old.LastResult, old.LastOK
	}
	cp := s
	st.items[s.ID] = &cp
	if err := st.saveLocked(); err != nil {
		return s, fmt.Errorf("the schedule could not be kept: %w", err)
	}
	return s, nil
}

func (st *Store) Delete(id string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.items, id)
	return st.saveLocked()
}

// Run fires one schedule now, whatever the clock says.
func (st *Store) Run(id string) (Schedule, error) {
	st.mu.Lock()
	s, ok := st.items[id]
	st.mu.Unlock()
	if !ok {
		return Schedule{}, errors.New("no such schedule")
	}
	st.fire(s)
	st.mu.Lock()
	defer st.mu.Unlock()
	if cur, ok := st.items[id]; ok {
		return *cur, nil
	}
	return *s, nil
}

func (st *Store) fire(s *Schedule) {
	pane, err := st.submit(s.Machine, s.Dir, s.Prompt)
	st.mu.Lock()
	// the result goes on the schedule as it is now — an edit saved while
	// the submit ran replaced the pointer; one deleted meanwhile is gone
	cur, ok := st.items[s.ID]
	if !ok {
		st.mu.Unlock()
		return
	}
	cur.LastRun = st.now()
	if err != nil {
		cur.LastOK, cur.LastResult = false, err.Error()
	} else {
		cur.LastOK, cur.LastResult = true, "sent to "+pane
	}
	if serr := st.saveLocked(); serr != nil {
		log.Printf("schedules: %v", serr)
	}
	label, machine, dir := cur.Label, cur.Machine, cur.Dir
	st.mu.Unlock()
	if err != nil && st.notify != nil {
		st.notify(push.Payload{
			Machine: machine, Kind: "schedule",
			Title: "Schedule “" + label + "” could not run",
			Body:  err.Error() + " · " + filepath.Base(dir),
			Tag:   "schedule|" + s.ID,
		})
	}
}

// Tick runs every enabled schedule whose cron matches this minute and that
// has not run this minute yet. Call it about every half minute.
func (st *Store) Tick() {
	now := st.now()
	minute := now.Truncate(time.Minute)
	st.mu.Lock()
	var due []*Schedule
	for _, s := range st.items {
		if !s.Enabled || s.LastRun.Truncate(time.Minute).Equal(minute) {
			continue
		}
		if c, err := Parse(s.Cron); err == nil && c.Matches(now) {
			due = append(due, s)
		}
	}
	st.mu.Unlock()
	for _, s := range due {
		st.inflight.Add(1)
		go func(s *Schedule) { // one slow delivery must not hold the others past their minute
			defer st.inflight.Done()
			st.fire(s)
		}(s)
	}
}

// Wait blocks until the deliveries a Tick started are done (tests).
func (st *Store) Wait() { st.inflight.Wait() }

// Run loops Tick forever.
func (st *Store) Loop() {
	for {
		st.Tick()
		time.Sleep(30 * time.Second)
	}
}

// ---- cron ----

// Cron is a five-field expression: minute hour day-of-month month
// day-of-week, each `*`, a number, a list `1,2`, a range `1-5`, or a step
// `*/15`; day-of-week 0 and 7 are both Sunday. Local time. As in cron,
// when both the day-of-month and the day-of-week are restricted, a day
// matching either one fires.
type Cron struct {
	fields  [5][]bool // allowed values per field, indexed by value
	anyDay  bool      // day-of-month is `*`
	anyWeek bool      // day-of-week is `*`
}

var ranges = [5][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 7}}

func Parse(expr string) (Cron, error) {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return Cron{}, fmt.Errorf("cron needs five fields (minute hour day month weekday), got %d", len(parts))
	}
	var c Cron
	c.anyDay, c.anyWeek = parts[2] == "*", parts[4] == "*"
	for i, part := range parts {
		lo, hi := ranges[i][0], ranges[i][1]
		allowed := make([]bool, hi+1)
		for _, item := range strings.Split(part, ",") {
			step := 1
			if base, s, ok := strings.Cut(item, "/"); ok {
				n, err := strconv.Atoi(s)
				if err != nil || n < 1 {
					return Cron{}, fmt.Errorf("bad step in %q", item)
				}
				step, item = n, base
			}
			a, b := lo, hi
			if item != "*" {
				var err error
				if x, y, ok := strings.Cut(item, "-"); ok {
					if a, err = strconv.Atoi(x); err != nil {
						return Cron{}, fmt.Errorf("bad value in %q", item)
					}
					if b, err = strconv.Atoi(y); err != nil {
						return Cron{}, fmt.Errorf("bad value in %q", item)
					}
				} else {
					if a, err = strconv.Atoi(item); err != nil {
						return Cron{}, fmt.Errorf("bad value in %q", item)
					}
					b = a
					if step > 1 {
						b = hi
					}
				}
			}
			if a < lo || b > hi || a > b {
				return Cron{}, fmt.Errorf("%q is outside %d-%d", item, lo, hi)
			}
			for v := a; v <= b; v += step {
				allowed[v] = true
			}
		}
		if i == 4 && allowed[7] { // Sunday twice
			allowed[0] = true
		}
		c.fields[i] = allowed
	}
	return c, nil
}

func (c Cron) Matches(t time.Time) bool {
	f := c.fields
	if !f[0][t.Minute()] || !f[1][t.Hour()] || !f[3][int(t.Month())] {
		return false
	}
	day, week := f[2][t.Day()], f[4][int(t.Weekday())]
	if !c.anyDay && !c.anyWeek {
		return day || week
	}
	return day && week
}

// Next is the first matching minute after t, within a year.
func (c Cron) Next(t time.Time) (time.Time, bool) {
	cur := t.Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 366*24*60; i++ {
		if c.Matches(cur) {
			return cur, true
		}
		cur = cur.Add(time.Minute)
	}
	return time.Time{}, false
}
