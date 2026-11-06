// Package backup keeps a copy of the cockpit's state dir on another machine of the fleet: the tasks, the schedules,
// the budgets, the recipes, the webhooks, the prefs, the review baselines, the devices, the agents' keys, the token, the
// push keys and the tailnet node — what an SD card that dies takes with it. A snapshot (snapshot.go) is taken nightly
// at the person's time and on demand, written into the target machine's remuda state dir under backups/<cockpit>/, the
// newest Keep kept there; `remuda restore` puts one back (restore.go). A failure is pushed once per run of failures.
package backup

import (
	"errors"
	"fmt"
	"log"
	"path"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/state"
)

const settingsFile = "backup.json"

// DefaultAt and DefaultKeep are a new cockpit's nightly time and how many backups stay.
const (
	DefaultAt   = "03:30"
	DefaultKeep = 14
	MaxKeep     = 365
)

// RetryAfter is how long a failed run waits before the next try; WaitForTarget is how long past its time a nightly
// run waits for its machine to connect (a cockpit that just started) before it counts as failed.
var (
	RetryAfter    = time.Hour
	WaitForTarget = time.Hour
)

// ErrBusy says a backup is being taken already; ErrNoTarget that no machine was picked.
var (
	ErrBusy     = errors.New("a backup is being taken already")
	ErrNoTarget = errors.New("no machine is picked for the backups")
)

// Settings is where and when the backups go.
type Settings struct {
	Machine string `json:"machine"` // the fleet machine the backups go to; "" takes none
	At      string `json:"at"`      // the nightly time, "HH:MM" in the cockpit's own zone
	Keep    int    `json:"keep"`    // how many of the newest stay on the target
}

// Result is one backup run.
type Result struct {
	At      time.Time `json:"at"`
	Machine string    `json:"machine"`
	Label   string    `json:"label,omitempty"`
	Path    string    `json:"path,omitempty"`
	Size    int64     `json:"size,omitempty"`
	Files   int       `json:"files,omitempty"`
	Took    float64   `json:"took"` // seconds
	Pruned  int       `json:"pruned,omitempty"`
	Error   string    `json:"error,omitempty"`
	Manual  bool      `json:"manual,omitempty"` // Back up now, not the nightly run
}

// Status is what the page shows.
type Status struct {
	Settings Settings  `json:"settings"`
	Last     *Result   `json:"last,omitempty"`
	LastGood *Result   `json:"last_good,omitempty"`
	Next     time.Time `json:"next,omitzero"`
	Running  bool      `json:"running"`
	Host     string    `json:"host"` // the cockpit's folder on the target
}

// record is backup.json.
type record struct {
	Settings Settings `json:"settings"`
	Last     *Result  `json:"last,omitempty"`
	LastGood *Result  `json:"last_good,omitempty"`
	Failures int      `json:"failures,omitempty"` // runs failed in a row
}

// Service takes the backups of one cockpit.
type Service struct {
	Dir     string
	Host    string // the cockpit's name, the folder its backups go to
	Version string
	Target  func(id string) (Target, error) // a machine of the fleet, connected
	Notify  func(title, body string)        // a failure, once per run of them
	Now     func() time.Time
	Zone    *time.Location

	mu      sync.Mutex
	rec     record
	running bool
}

// Open reads the settings and the last result; a damaged file is logged and read as none.
func Open(dir, host, version string) *Service {
	s := &Service{Dir: dir, Host: host, Version: version, Now: time.Now, Zone: time.Local}
	if err := state.LoadJSON(dir, settingsFile, &s.rec); err != nil {
		log.Printf("backup: %s is damaged (%v); no backups are taken until they are set again", settingsFile, err)
		s.rec = record{}
	}
	if s.rec.Settings.At == "" {
		s.rec.Settings.At = DefaultAt
	}
	if s.rec.Settings.Keep <= 0 {
		s.rec.Settings.Keep = DefaultKeep
	}
	return s
}

var clock = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d)$`)

// Check says what is wrong with settings, nil when nothing; the machine is the caller's to know.
func Check(settings Settings) error {
	if !clock.MatchString(settings.At) {
		return fmt.Errorf("the time is HH:MM, 00:00 to 23:59, not %q", settings.At)
	}
	if settings.Keep < 1 || settings.Keep > MaxKeep {
		return fmt.Errorf("keep 1 to %d backups, not %d", MaxKeep, settings.Keep)
	}
	return nil
}

// Put replaces the settings.
func (s *Service) Put(settings Settings) error {
	if err := Check(settings); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec.Settings = settings
	return state.SaveJSON(s.Dir, settingsFile, s.rec)
}

// Status is the settings, the last runs and the next nightly one.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := Status{Settings: s.rec.Settings, Last: s.rec.Last, LastGood: s.rec.LastGood, Running: s.running, Host: HostFolder(s.Host)}
	if s.rec.Settings.Machine != "" {
		status.Next = Next(s.rec.Settings.At, s.Now(), s.Zone)
	}
	return status
}

// scheduled is the newest moment the nightly time came at or before now.
func scheduled(at string, now time.Time, zone *time.Location) time.Time {
	m := clock.FindStringSubmatch(at)
	if m == nil {
		m = clock.FindStringSubmatch(DefaultAt)
	}
	hour, _ := strconv.Atoi(m[1])
	minute, _ := strconv.Atoi(m[2])
	local := now.In(zone)
	moment := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, zone)
	if moment.After(now) {
		moment = time.Date(local.Year(), local.Month(), local.Day()-1, hour, minute, 0, 0, zone)
	}
	return moment
}

// Next is the first moment the nightly time comes after now.
func Next(at string, now time.Time, zone *time.Location) time.Time {
	last := scheduled(at, now, zone)
	return time.Date(last.Year(), last.Month(), last.Day()+1, last.Hour(), last.Minute(), 0, 0, zone)
}

// Due says whether the nightly run should go now: the time came since the last run, or the last run failed and has
// waited RetryAfter; the moment it was due for is the answer's second part.
func Due(settings Settings, last *Result, now time.Time, zone *time.Location) (bool, time.Time) {
	if settings.Machine == "" {
		return false, time.Time{}
	}
	moment := scheduled(settings.At, now, zone)
	if last == nil || last.At.Before(moment) {
		return true, moment
	}
	return last.Error != "" && now.Sub(last.At) >= RetryAfter, moment
}

// Run takes a backup now, the person's or the night's; the result is kept either way.
func (s *Service) Run(manual bool) (Result, error) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return Result{}, ErrBusy
	}
	settings := s.rec.Settings
	if settings.Machine == "" {
		s.mu.Unlock()
		return Result{}, ErrNoTarget
	}
	s.running = true
	s.mu.Unlock()
	start := s.Now()
	result := Result{At: start, Machine: settings.Machine, Manual: manual}
	target, err := s.Target(settings.Machine)
	if err == nil {
		result.Label = target.Label()
		err = s.take(target, settings.Keep, start, &result)
	}
	result.Took = s.Now().Sub(start).Seconds()
	if err != nil {
		result.Error = err.Error()
	}
	s.finish(result)
	return result, err
}

// take snapshots the state dir and puts it on the target, then prunes the target's folder.
func (s *Service) take(target Target, keep int, at time.Time, result *Result) error {
	data, manifest, err := Snapshot(s.Dir, s.Host, s.Version, at)
	if err != nil {
		return err
	}
	result.Size, result.Files = int64(len(data)), manifest.Files
	folder, err := folderOn(target, s.Dir, s.Host, true)
	if err != nil {
		return err
	}
	name := NameAt(at)
	if err := target.PutFile(path.Join(folder, name+".partial"), data, Timeout); err != nil {
		return fmt.Errorf("writing it on %s: %w", target.Label(), err)
	}
	out, err := shell.Text(target, FinishScript(folder, name, keep), Timeout)
	if err != nil {
		return fmt.Errorf("putting it in place on %s: %s", target.Label(), firstLine(out, err))
	}
	result.Path, result.Pruned = path.Join(folder, name), pruned(out)
	return nil
}

// finish keeps a run's result and says a failure once per run of them.
func (s *Service) finish(result Result) {
	s.mu.Lock()
	s.running = false
	s.rec.Last = &result
	first := false
	if result.Error == "" {
		s.rec.LastGood = &result
		s.rec.Failures = 0
	} else {
		s.rec.Failures++
		first = s.rec.Failures == 1
	}
	err := state.SaveJSON(s.Dir, settingsFile, s.rec)
	s.mu.Unlock()
	if err != nil {
		log.Printf("backup: keeping the result: %v", err)
	}
	if result.Error != "" {
		log.Printf("backup: to %s failed: %s", result.Machine, result.Error)
		if first && s.Notify != nil {
			s.Notify("The backup failed", failureWords(result))
		}
	}
}

func failureWords(result Result) string {
	where := result.Label
	if where == "" {
		where = result.Machine
	}
	return fmt.Sprintf("To %s: %s", where, result.Error)
}

// List is the backups on the target, newest first.
func (s *Service) List() (string, []File, error) {
	s.mu.Lock()
	machine := s.rec.Settings.Machine
	s.mu.Unlock()
	if machine == "" {
		return "", nil, ErrNoTarget
	}
	target, err := s.Target(machine)
	if err != nil {
		return "", nil, err
	}
	folder, err := folderOn(target, s.Dir, s.Host, false)
	if err != nil {
		return "", nil, err
	}
	out, err := shell.Text(target, ListScript(folder), Timeout)
	if err != nil {
		return folder, nil, fmt.Errorf("listing %s on %s: %s", folder, target.Label(), firstLine(out, err))
	}
	return folder, ParseList(out), nil
}

// Tick takes the nightly backup when it is due; a machine not connected yet is waited for up to WaitForTarget.
func (s *Service) Tick() {
	s.mu.Lock()
	settings, last, running := s.rec.Settings, s.rec.Last, s.running
	s.mu.Unlock()
	now := s.Now()
	due, moment := Due(settings, last, now, s.Zone)
	if !due || running {
		return
	}
	if _, err := s.Target(settings.Machine); err != nil && now.Sub(moment) < WaitForTarget {
		return
	}
	_, _ = s.Run(false)
}

// Loop ticks every so often until done.
func (s *Service) Loop(every time.Duration, done <-chan struct{}) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			s.Tick()
		}
	}
}
