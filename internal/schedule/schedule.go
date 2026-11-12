// Package schedule runs things on a timetable: a prompt to the agent in a
// workspace folder (panes come and go, the project stays), the standup as a
// digest pushed to the person's devices, or a fresh agent started in a folder
// with a prompt and taken down once it has answered; a five-field cron in
// local time; a run is never queued or retried, and one that comes while the agent it would talk to (or the agent its
// last run started) still works or asks is skipped and noted.
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

	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/state"
	"github.com/dev1antdev/remuda/internal/text"
)

const file = "schedules.json"

// Why a run was skipped: the agent still works, waits for the person, has not taken its prompt yet, its machine has not
// said how it stands, or the run before is still on its way.
const (
	SkipWorking  = "working"
	SkipAsking   = "asking"
	SkipStarting = "starting"
	SkipUnsure   = "unsure"
	SkipRunning  = "running"
)

// The kinds of schedule: a prompt to an agent, the standup pushed as a digest, or a fresh agent started in a folder.
const (
	KindPrompt = "prompt"
	KindDigest = "digest"
	KindAgent  = "agent"
)

// What a digest covers: since local midnight, the last 24 hours, the last 7 days, or since its previous run.
const (
	SinceMidnight = "midnight"
	SinceDay      = "24h"
	SinceWeek     = "7d"
	SinceLast     = "last"
)

type Schedule struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"` // prompt (the default) | digest | agent
	Machine string `json:"machine"`
	Dir     string `json:"dir"`
	Label   string `json:"label"`
	Cron    string `json:"cron"`
	Prompt  string `json:"prompt"`
	Since   string `json:"since,omitempty"` // a digest's period: midnight | 24h | 7d | last
	Agent   string `json:"agent,omitempty"` // an agent schedule's kind of agent
	Model   string `json:"model,omitempty"` // the model as the agent's own CLI names it, "" for its default
	Args    string `json:"args,omitempty"`  // extra arguments, as the person wrote them
	Close   bool   `json:"close,omitempty"` // take the tab down once the agent has answered
	Enabled bool   `json:"enabled"`

	Next       time.Time `json:"next,omitzero"` // the coming run, filled in by List
	LastRun    time.Time `json:"last_run,omitzero"`
	LastResult string    `json:"last_result,omitempty"`
	LastOK     bool      `json:"last_ok"`
	Skipped    string    `json:"skipped,omitempty"`   // why the last run was skipped (Skip…), "" when it ran
	LastPane   *Started  `json:"last_pane,omitempty"` // the agent the last run started, until it has answered or gone
}

// Submit delivers a prompt to the agent in a folder and names the pane it went to.
type Submit func(machineID, dir, text string) (pane string, err error)

// Digest is the standup since a moment as a notification's title and body; empty when nobody did anything.
type Digest func(since time.Time) (title, body string, empty bool)

type Store struct {
	dir    string
	submit Submit
	notify func(push.Payload)
	digest Digest
	Agents Agents // the app's way to start, close and read an agent schedule's agent; set after New
	now    func() time.Time

	mu       sync.Mutex
	items    map[string]*Schedule
	firing   map[string]time.Time // the minute each schedule was last dispatched for, until its run has noted LastRun
	running  map[string]bool      // the schedules whose run is on its way now
	inflight sync.WaitGroup
}

func New(dir string, submit Submit, notify func(push.Payload), digest Digest) *Store {
	st := &Store{dir: dir, submit: submit, notify: notify, digest: digest, now: time.Now, items: map[string]*Schedule{}, firing: map[string]time.Time{}, running: map[string]bool{}}
	var list []*Schedule
	if err := state.LoadJSON(dir, file, &list); err != nil {
		log.Printf("%s is unreadable (%v): no schedules until it is fixed", file, err)
	}
	for _, s := range list {
		if s.Kind == "" {
			s.Kind = KindPrompt // the records from before the kinds
		}
		if s.Kind == KindDigest && s.Since == "" {
			s.Since = SinceMidnight
		}
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

// Put creates or replaces a schedule after checking it by its kind.
func (st *Store) Put(s Schedule) (Schedule, error) {
	s.Label = strings.TrimSpace(s.Label)
	s.Prompt = strings.TrimSpace(s.Prompt)
	s.Cron = strings.Join(strings.Fields(s.Cron), " ")
	if s.Kind == "" {
		s.Kind = KindPrompt
	}
	switch s.Kind {
	case KindPrompt:
		if s.Machine == "" || !strings.HasPrefix(s.Dir, "/") {
			return s, errors.New("a machine and an absolute folder are required")
		}
		if s.Prompt == "" {
			return s, errors.New("the prompt is empty")
		}
		s.Since = ""
		if s.Label == "" {
			s.Label = text.FirstLine(s.Prompt, 60)
		}
	case KindDigest:
		if s.Machine != "" || s.Dir != "" || s.Prompt != "" {
			return s, errors.New("a digest goes to your devices: no machine, folder or prompt")
		}
		if s.Since == "" {
			s.Since = SinceMidnight
		}
		if !sinceKnown(s.Since) {
			return s, errors.New("a digest covers midnight, 24h, 7d or last")
		}
		if s.Label == "" {
			s.Label = "Standup digest"
		}
	case KindAgent:
		if err := checkAgent(&s); err != nil {
			return s, err
		}
	default:
		return s, errors.New("a schedule is a prompt, a digest or an agent")
	}
	if _, err := Parse(s.Cron); err != nil {
		return s, err
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if s.ID == "" {
		s.ID = state.NewID()
	} else if old, ok := st.items[s.ID]; ok {
		s.LastRun, s.LastResult, s.LastOK, s.Skipped, s.LastPane = old.LastRun, old.LastResult, old.LastOK, old.Skipped, old.LastPane
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
	delete(st.firing, id)
	return st.saveLocked()
}

// Run fires one schedule now, whatever the clock says; a run that would pile up on the last one is skipped all the same.
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
	st.mu.Lock()
	if st.running[s.ID] {
		st.mu.Unlock()
		st.noteSkip(s.ID, SkipRunning, "the previous run is still on its way")
		return
	}
	st.running[s.ID] = true
	st.mu.Unlock()
	defer func() {
		st.mu.Lock()
		delete(st.running, s.ID)
		st.mu.Unlock()
	}()
	switch s.Kind {
	case KindDigest:
		st.fireDigest(s)
	case KindAgent:
		st.fireAgent(s)
	default:
		st.firePrompt(s)
	}
}

// firePrompt hands the prompt to the agent in the folder; one still at work or asking something skips the run.
func (st *Store) firePrompt(s *Schedule) {
	pane, err := st.submit(s.Machine, s.Dir, s.Prompt)
	if why := skipOf(err); why != "" {
		st.noteSkip(s.ID, why, promptSkipWords(why, pane))
		return
	}
	st.mu.Lock()
	// an edit saved while the submit ran replaced the pointer, and one deleted meanwhile is gone: the result goes on the schedule as it is now
	cur, ok := st.items[s.ID]
	if !ok {
		st.mu.Unlock()
		return
	}
	cur.LastRun, cur.Skipped = st.now(), ""
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

// fireDigest pushes the standup since the period the schedule names; a quiet period sends nothing and says so.
func (st *Store) fireDigest(s *Schedule) {
	now := st.now()
	st.mu.Lock()
	since := digestSince(s.Since, s.LastRun, now)
	st.mu.Unlock()
	var title, body string
	empty := true
	err := errors.New("no standup to digest")
	if st.digest != nil {
		title, body, empty = st.digest(since)
		err = nil
	}
	st.mu.Lock()
	cur, ok := st.items[s.ID]
	if !ok {
		st.mu.Unlock()
		return
	}
	cur.LastRun, cur.Skipped = now, ""
	switch {
	case err != nil:
		cur.LastOK, cur.LastResult = false, err.Error()
	case empty:
		cur.LastOK, cur.LastResult = true, "nothing to report"
	default:
		cur.LastOK, cur.LastResult = true, "sent"
	}
	if serr := st.saveLocked(); serr != nil {
		log.Printf("schedules: %v", serr)
	}
	id := cur.ID
	st.mu.Unlock()
	if err == nil && !empty && st.notify != nil {
		st.notify(push.Payload{Kind: KindDigest, Title: title, Body: body, Tag: "digest|" + id, Panel: "standup"})
	}
}

// skipOf says why a prompt's refusal is a skipped run rather than a failed one: the agent works or asks; "" for any other.
func skipOf(err error) string {
	switch {
	case errors.Is(err, sessions.ErrAgentWorking):
		return SkipWorking
	case errors.Is(err, sessions.ErrAgentBusy):
		return SkipAsking
	}
	return ""
}

// promptSkipWords is what a skipped prompt run notes, naming the pane it would have gone to.
func promptSkipWords(why, pane string) string {
	agent := "the agent"
	if pane != "" {
		agent += " in " + pane
	}
	if why == SkipAsking {
		return agent + " waits for an answer"
	}
	return agent + " is still working"
}

// noteSkip puts a skipped run on the schedule — when, why, the words — with no push: the agent it would have reached is at
// work or waits for the person, who knows.
func (st *Store) noteSkip(id, why, words string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	cur, ok := st.items[id]
	if !ok {
		return
	}
	cur.LastRun, cur.LastOK, cur.Skipped, cur.LastResult = st.now(), true, why, "skipped: "+words
	if err := st.saveLocked(); err != nil {
		log.Printf("schedules: %v", err)
	}
}

// digestSince is the start of a digest's period at now: local midnight, 24 hours or 7 days back, or the previous run (midnight when there was none).
func digestSince(mode string, lastRun, now time.Time) time.Time {
	switch mode {
	case SinceDay:
		return now.Add(-24 * time.Hour)
	case SinceWeek:
		return now.Add(-7 * 24 * time.Hour)
	case SinceLast:
		if !lastRun.IsZero() {
			return lastRun
		}
	}
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

func sinceKnown(mode string) bool {
	switch mode {
	case SinceMidnight, SinceDay, SinceWeek, SinceLast:
		return true
	}
	return false
}

// Tick runs every enabled schedule whose cron matches this minute and was neither run nor started this minute yet — a
// delivery may outlast the half minute to the next tick, and the hour a clock falls back reads a minute it ran at
// already; call it about every half minute.
func (st *Store) Tick() {
	now := st.now()
	minute := now.Truncate(time.Minute)
	st.mu.Lock()
	var due []*Schedule
	for _, s := range st.items {
		if !s.Enabled || wallMinute(s.LastRun, now.Location()) == wallMinute(now, now.Location()) || st.firing[s.ID].Equal(minute) {
			continue
		}
		if c, err := Parse(s.Cron); err == nil && c.Matches(now) {
			st.firing[s.ID] = minute
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

// Upcoming is a cron's next n runs from now, for a form to show before it is saved; an expression Parse refuses is the error.
func (st *Store) Upcoming(expr string, n int) ([]time.Time, error) {
	c, err := Parse(expr)
	if err != nil {
		return nil, err
	}
	out := []time.Time{}
	at := st.now()
	for len(out) < n {
		next, ok := c.Next(at)
		if !ok {
			break
		}
		out = append(out, next)
		at = next
	}
	return out, nil
}

// Wait blocks until the deliveries a Tick started are done (tests).
func (st *Store) Wait() { st.inflight.Wait() }

// Loop runs Tick forever.
func (st *Store) Loop() {
	for {
		st.Tick()
		time.Sleep(30 * time.Second)
	}
}

// ---- cron ----

// Cron is a five-field expression (minute hour day-of-month month day-of-week) in local time: `*`, a number, a list, a range or a step; day-of-week 0 and 7 are both Sunday.
// As in Vixie cron, when both day fields are restricted a day matching either one fires.
type Cron struct {
	fields  [5][]bool
	anyDay  bool
	anyWeek bool
}

var ranges = [5][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 7}}

func Parse(expr string) (Cron, error) {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return Cron{}, fmt.Errorf("cron needs five fields (minute hour day month weekday), got %d", len(parts))
	}
	var c Cron
	c.anyDay, c.anyWeek = strings.HasPrefix(parts[2], "*"), strings.HasPrefix(parts[4], "*") // Vixie's DOM_STAR: "*/2" too
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

// Next is the first matching minute after t, within a year, that does not read on the wall clock as the match before it:
// the hour a clock falls back repeats a minute Tick ran at already.
func (c Cron) Next(t time.Time) (time.Time, bool) {
	from := t.Truncate(time.Minute).Add(time.Minute)
	cur := from.Add(-fallBackReach)
	last := ""
	for i := 0; i < 366*24*60+int(fallBackReach/time.Minute); i++ {
		if c.Matches(cur) {
			wall := wallMinute(cur, cur.Location())
			if !cur.Before(from) && wall != last {
				return cur, true
			}
			last = wall
		}
		cur = cur.Add(time.Minute)
	}
	return time.Time{}, false
}

// fallBackReach is how far back Next looks for a match a fall-back repeats: no zone turns its clock back further.
const fallBackReach = 3 * time.Hour

// wallMinute is a moment as the wall clock reads it in loc, to the minute: the hour a clock falls back reads the same
// minutes twice.
func wallMinute(t time.Time, loc *time.Location) string {
	return t.In(loc).Format("2006-01-02 15:04")
}
