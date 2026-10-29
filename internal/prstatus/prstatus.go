// Package prstatus keeps the state of every pull request an agent named — its checks, its review, whether it merges —
// read with gh on the pane's own machine, as the person, so remuda itself asks GitHub nothing. A request is read when
// the agent names it and then at most every Every while the page keeps reading the roster; a GitLab request is left
// alone (no gh for it).
package prstatus

import (
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

var (
	// Every is the least time between two reads of one request.
	Every = 2 * time.Minute
	// Timeout is how long one gh call may take.
	Timeout = 20 * time.Second
	// ComplainEvery spaces the log lines about a machine whose gh cannot answer.
	ComplainEvery = time.Hour
)

// Checks counts a request's check runs and status contexts by how they stand.
type Checks struct {
	Total   int `json:"total"`
	Pass    int `json:"pass"`
	Fail    int `json:"fail"`
	Pending int `json:"pending"`
	Skipped int `json:"skipped"`
}

// Status is a pull request as gh reports it.
type Status struct {
	State     string    `json:"state"` // OPEN, MERGED, CLOSED
	Draft     bool      `json:"draft"`
	Mergeable string    `json:"mergeable"` // MERGEABLE, CONFLICTING, UNKNOWN
	Review    string    `json:"review"`    // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, ""
	Checks    Checks    `json:"checks"`
	At        time.Time `json:"at"`
}

// Hosts finds the shell of a machine; false when it is not connected.
type Hosts func(machineID string) (shell.Shell, bool)

type entry struct {
	url     string
	status  *Status
	reason  string // why there is no status: gh missing, not signed in, the machine away
	readAt  time.Time
	reading bool
}

// Tracker holds one request per pane and reads one at a time per machine.
type Tracker struct {
	hosts Hosts
	// OnState hears a request's state on its first read and every move after it; the app puts it on the bus.
	OnState    func(events.PullRequestState)
	now        func() time.Time
	mu         sync.Mutex
	entries    map[events.PaneRef]*entry
	inflight   map[string]bool
	complained map[string]time.Time
}

func New(hosts Hosts) *Tracker {
	return &Tracker{hosts: hosts, now: time.Now, entries: map[events.PaneRef]*entry{}, inflight: map[string]bool{}, complained: map[string]time.Time{}}
}

// GitHub says whether gh can read the request: a pull request, not a merge request.
func GitHub(url string) bool {
	return strings.Contains(url, "/pull/")
}

// OnNamed hears a request named: a new or changed one is read now.
func (t *Tracker) OnNamed(ev events.PullRequestNamed) {
	if !GitHub(ev.URL) {
		return
	}
	t.mu.Lock()
	e := t.entries[ev.Pane]
	if e != nil && e.url == ev.URL {
		t.mu.Unlock()
		return
	}
	t.entries[ev.Pane] = &entry{url: ev.URL}
	t.mu.Unlock()
	t.start(ev.Pane)
}

// OnStatus forgets a closed pane.
func (t *Tracker) OnStatus(ev events.PaneStatus) {
	if ev.To != events.Gone {
		return
	}
	t.mu.Lock()
	delete(t.entries, ev.Pane)
	t.mu.Unlock()
}

// Forget drops a machine's requests.
func (t *Tracker) Forget(machineID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for ref := range t.entries {
		if ref.Machine == machineID {
			delete(t.entries, ref)
		}
	}
}

// Status is the request's state as last read; false while unknown.
func (t *Tracker) Status(machineID, paneID string) (Status, bool) {
	if t == nil {
		return Status{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.entries[events.PaneRef{Machine: machineID, Pane: paneID}]
	if e == nil || e.status == nil {
		return Status{}, false
	}
	return *e.status, true
}

// Touch says the roster was just read: every request whose last read is Every old is read again, one per machine
// at a time. Nothing is read for a page nobody looks at.
func (t *Tracker) Touch() {
	if t == nil {
		return
	}
	now := t.now()
	t.mu.Lock()
	var due []events.PaneRef
	for ref, e := range t.entries {
		if !e.reading && now.Sub(e.readAt) >= Every {
			due = append(due, ref)
		}
	}
	t.mu.Unlock()
	for _, ref := range due {
		t.start(ref)
	}
}

// TouchPane reads one pane's request again when its last read is Every old: the task watch asks it for the request a
// live run names, so a merge moves the task with no page open.
func (t *Tracker) TouchPane(machineID, paneID string) {
	if t == nil {
		return
	}
	ref := events.PaneRef{Machine: machineID, Pane: paneID}
	t.mu.Lock()
	e := t.entries[ref]
	due := e != nil && !e.reading && t.now().Sub(e.readAt) >= Every
	t.mu.Unlock()
	if due {
		t.start(ref)
	}
}

// start reads one request in the background when its machine has no read under way.
func (t *Tracker) start(ref events.PaneRef) {
	t.mu.Lock()
	e := t.entries[ref]
	if e == nil || e.reading || t.inflight[ref.Machine] {
		t.mu.Unlock()
		return
	}
	e.reading = true
	t.inflight[ref.Machine] = true
	url := e.url
	t.mu.Unlock()
	go t.read(ref, url)
}

func (t *Tracker) read(ref events.PaneRef, url string) {
	status, reason := t.fetch(ref.Machine, url)
	if moved, ok := t.note(ref, url, status, reason); ok && t.OnState != nil {
		t.OnState(moved)
	}
}

// note keeps what a read came to and says whether the request's state is news: its first read, or a move since.
func (t *Tracker) note(ref events.PaneRef, url string, status *Status, reason string) (events.PullRequestState, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.inflight, ref.Machine)
	e := t.entries[ref]
	if e == nil || e.url != url {
		return events.PullRequestState{}, false
	}
	e.reading = false
	e.readAt = t.now()
	e.reason = reason
	if reason != "" && t.now().Sub(t.complained[ref.Machine]) >= ComplainEvery {
		t.complained[ref.Machine] = t.now()
		log.Printf("prstatus: %s: %s cannot be read: %s", ref.Machine, url, reason)
	}
	if status == nil {
		return events.PullRequestState{}, false
	}
	was := ""
	if e.status != nil {
		was = e.status.State
	}
	e.status = status
	if status.State == was {
		return events.PullRequestState{}, false
	}
	return events.PullRequestState{Pane: ref, URL: url, State: status.State, Was: was}, true
}

// fetch runs gh on the machine and parses its answer.
func (t *Tracker) fetch(machineID, url string) (*Status, string) {
	host, ok := t.hosts(machineID)
	if !ok {
		return nil, "the machine is not connected"
	}
	out, err := shell.Text(host, Script(url), Timeout)
	status, perr := Parse(out, t.now())
	if perr == nil {
		return &status, ""
	}
	if err != nil && strings.TrimSpace(out) == "" {
		return nil, err.Error()
	}
	return nil, perr.Error()
}

// Script is the gh call for a request, run on the pane's machine.
func Script(url string) string {
	return script.PATH + "\ngh pr view " + script.Quote(url) + " --json state,isDraft,mergeable,reviewDecision,statusCheckRollup 2>&1"
}

type rollupItem struct {
	Typename   string `json:"__typename"`
	Conclusion string `json:"conclusion"` // a check run, once completed
	Status     string `json:"status"`     // a check run: COMPLETED, IN_PROGRESS, QUEUED, …
	State      string `json:"state"`      // a status context: SUCCESS, PENDING, FAILURE, ERROR, EXPECTED
}

type ghView struct {
	State          string       `json:"state"`
	IsDraft        bool         `json:"isDraft"`
	Mergeable      string       `json:"mergeable"`
	ReviewDecision string       `json:"reviewDecision"`
	Rollup         []rollupItem `json:"statusCheckRollup"`
}

// Parse reads gh's JSON; anything else — gh missing, not signed in, a refused request — is the error's text.
func Parse(out string, now time.Time) (Status, error) {
	trimmed := strings.TrimSpace(out)
	at := strings.Index(trimmed, "{")
	if at < 0 {
		return Status{}, errors.New(firstLine(trimmed, "gh answered nothing"))
	}
	var view ghView
	if err := json.Unmarshal([]byte(trimmed[at:]), &view); err != nil {
		return Status{}, errors.New(firstLine(trimmed, err.Error()))
	}
	status := Status{State: view.State, Draft: view.IsDraft, Mergeable: view.Mergeable, Review: view.ReviewDecision, At: now}
	for _, item := range view.Rollup {
		status.Checks.Total++
		switch stand(item) {
		case "pass":
			status.Checks.Pass++
		case "fail":
			status.Checks.Fail++
		case "skipped":
			status.Checks.Skipped++
		default:
			status.Checks.Pending++
		}
	}
	return status, nil
}

// stand is one rollup item as pass, fail, skipped or pending.
func stand(item rollupItem) string {
	if item.Typename == "StatusContext" || (item.Conclusion == "" && item.Status == "" && item.State != "") {
		switch item.State {
		case "SUCCESS":
			return "pass"
		case "FAILURE", "ERROR":
			return "fail"
		}
		return "pending"
	}
	if item.Status != "" && item.Status != "COMPLETED" {
		return "pending"
	}
	switch item.Conclusion {
	case "SUCCESS", "NEUTRAL":
		return "pass"
	case "SKIPPED":
		return "skipped"
	case "FAILURE", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED", "STARTUP_FAILURE", "STALE":
		return "fail"
	}
	return "pending"
}

func firstLine(text, fallback string) string {
	if text == "" {
		return fallback
	}
	line, _, _ := strings.Cut(text, "\n")
	return strings.TrimSpace(line)
}
