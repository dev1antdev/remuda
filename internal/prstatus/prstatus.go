// Package prstatus keeps the state of every pull or merge request an agent named — its checks, its review, whether it
// merges — read on the pane's own machine as the person, a GitHub pull request with gh and a GitLab merge request with
// glab (gitlab.go), so remuda itself asks neither anything. A request is read when the agent names it, then at most
// every Every while the page keeps reading the roster, and at most every FreshFor while a hover card shows it.
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
	// Timeout is how long one gh or glab call may take.
	Timeout = 20 * time.Second
	// ComplainEvery spaces the log lines about a machine whose gh or glab cannot answer.
	ComplainEvery = time.Hour
	// FreshFor is how old a read may be before a hover card open on the request asks for another.
	FreshFor = 20 * time.Second
)

// Checks counts a request's check runs and status contexts by how they stand.
type Checks struct {
	Total   int `json:"total"`
	Pass    int `json:"pass"`
	Fail    int `json:"fail"`
	Pending int `json:"pending"`
	Skipped int `json:"skipped"`
}

// Status is a pull request as gh reports it, or a merge request as glab does, in the same words.
type Status struct {
	Title      string    `json:"title,omitempty"`
	State      string    `json:"state"` // OPEN, MERGED, CLOSED
	Draft      bool      `json:"draft"`
	Mergeable  string    `json:"mergeable"`             // MERGEABLE, CONFLICTING, UNKNOWN
	MergeState string    `json:"merge_state,omitempty"` // GitHub's mergeStateStatus (CLEAN merges now; DIRTY, BLOCKED, BEHIND, UNSTABLE), CLEAN for a merge request GitLab calls mergeable
	Review     string    `json:"review"`                // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, ""
	Threads    int       `json:"threads,omitempty"`     // a merge request's discussion threads still open
	Checks     Checks    `json:"checks"`
	Closes     []Issue   `json:"closes,omitempty"` // the issues the request closes once it merges
	At         time.Time `json:"at"`
}

// Issue is an issue a pull request closes, as gh names it: its number, its address and its repository, owner/name.
type Issue struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	Repo   string `json:"repo"`
}

// Seen is what the tracker holds of a pane's request: its status as last read, whether a read is under way, and why
// the last read failed when it did — a status from before stands beside that reason.
type Seen struct {
	Status  *Status
	Reading bool
	Reason  string
}

// Hosts finds the shell of a machine; false when it is not connected.
type Hosts func(machineID string) (shell.Shell, bool)

type entry struct {
	url     string
	status  *Status
	reason  string // why there is no status: gh or glab missing, not signed in, the machine away
	readAt  time.Time
	reading bool
	etag    string    // the last probe's, for a GitHub pull request (probe.go)
	fullAt  time.Time // when the request was last read in full
}

// reading is what one read came to: the status (nil and why when there is none), whether it was read in full, and the
// probe's ETag to keep.
type reading struct {
	status *Status
	reason string
	full   bool
	etag   string
}

// Tracker holds one request per pane and reads one at a time per machine.
type Tracker struct {
	hosts Hosts
	// OnState hears a request's state on its first read and every move after it; the app puts it on the bus.
	OnState func(events.PullRequestState)
	// OnChange hears a read begin and end, so an open page shows it at once.
	OnChange   func()
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
	if !Readable(ev.URL) {
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

// Seen is the pane's request as the tracker holds it, read or being read; false when the pane names none it can read.
func (t *Tracker) Seen(machineID, paneID string) (Seen, bool) {
	if t == nil {
		return Seen{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.entries[events.PaneRef{Machine: machineID, Pane: paneID}]
	if e == nil {
		return Seen{}, false
	}
	seen := Seen{Reading: e.reading, Reason: e.reason}
	if e.status != nil {
		status := *e.status
		seen.Status = &status
	}
	return seen, true
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
	t.touchPane(machineID, paneID, Every)
}

// Fresh reads one pane's request again when its last read is FreshFor old: a hover card opened on it asks, so the card
// shows the checks as they stand. It says whether a read began.
func (t *Tracker) Fresh(machineID, paneID string) bool {
	return t.touchPane(machineID, paneID, FreshFor)
}

func (t *Tracker) touchPane(machineID, paneID string, after time.Duration) bool {
	if t == nil {
		return false
	}
	ref := events.PaneRef{Machine: machineID, Pane: paneID}
	t.mu.Lock()
	e := t.entries[ref]
	due := e != nil && !e.reading && t.now().Sub(e.readAt) >= after
	t.mu.Unlock()
	return due && t.start(ref)
}

// start reads one request in the background when its machine has no read under way, and says whether it began.
func (t *Tracker) start(ref events.PaneRef) bool {
	t.mu.Lock()
	e := t.entries[ref]
	if e == nil || e.reading || t.inflight[ref.Machine] {
		t.mu.Unlock()
		return false
	}
	e.reading = true
	t.inflight[ref.Machine] = true
	url := e.url
	t.mu.Unlock()
	t.changed()
	go t.read(ref, url)
	return true
}

func (t *Tracker) read(ref events.PaneRef, url string) {
	moved, ok := t.note(ref, url, t.fetch(ref, url))
	t.changed()
	if ok && t.OnState != nil {
		t.OnState(moved)
	}
}

func (t *Tracker) changed() {
	if t.OnChange != nil {
		t.OnChange()
	}
}

// note keeps what a read came to and says whether the request's state is news: its first read, or a move since.
func (t *Tracker) note(ref events.PaneRef, url string, got reading) (events.PullRequestState, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.inflight, ref.Machine)
	e := t.entries[ref]
	if e == nil || e.url != url {
		return events.PullRequestState{}, false
	}
	status, reason := got.status, got.reason
	e.reading = false
	e.readAt = t.now()
	e.reason = reason
	if got.etag != "" {
		e.etag = got.etag
	}
	if got.full && status != nil {
		e.fullAt = t.now()
	}
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

// fetch reads the request on the machine: a GitHub pull request probed first, its last full read kept while the probe
// says nothing moved and the read still stands (probe.go), else gh's (or glab's) answer parsed.
func (t *Tracker) fetch(ref events.PaneRef, url string) reading {
	host, ok := t.hosts(ref.Machine)
	if !ok {
		return reading{reason: "the machine is not connected"}
	}
	if GitLab(url) {
		status, reason := fetchMerge(host, url, t.now())
		return reading{status: status, reason: reason, full: true}
	}
	t.mu.Lock()
	var last *Status
	var etag string
	var fullAt time.Time
	if e := t.entries[ref]; e != nil && e.url == url {
		last, etag, fullAt = e.status, e.etag, e.fullAt
	}
	t.mu.Unlock()
	code, tag := probe(host, url, etag)
	if code == 304 && last != nil && t.now().Sub(fullAt) < standFor(last) {
		kept := *last
		return reading{status: &kept}
	}
	if code != 200 {
		tag = ""
	}
	status, reason := fetchPull(host, url, t.now())
	return reading{status: status, reason: reason, full: true, etag: tag}
}

// fetchPull runs gh pr view on the machine and parses its answer.
func fetchPull(host shell.Shell, url string, now time.Time) (*Status, string) {
	out, err := shell.Text(host, Script(url), Timeout)
	status, perr := Parse(out, now)
	if perr == nil {
		return &status, ""
	}
	if err != nil && strings.TrimSpace(out) == "" {
		return nil, err.Error()
	}
	return nil, perr.Error()
}

// viewFields are what every gh names; viewMore adds the ones a gh from before closingIssuesReferences refuses whole.
const (
	viewFields = "state,isDraft,mergeable,reviewDecision,statusCheckRollup,title"
	viewMore   = viewFields + ",mergeStateStatus,closingIssuesReferences"
)

// Script is the gh call for a request, run on the pane's machine: every field, or the ones any gh knows when it
// answers that a field is unknown.
func Script(url string) string {
	view := "gh pr view " + script.Quote(url) + " --json "
	return script.PATH + "\nout=$(" + view + viewMore + " 2>&1)\ncase $out in *'Unknown JSON field'*) out=$(" + view + viewFields + " 2>&1) ;; esac\nprintf '%s\\n' \"$out\""
}

type rollupItem struct {
	Typename   string `json:"__typename"`
	Conclusion string `json:"conclusion"` // a check run, once completed
	Status     string `json:"status"`     // a check run: COMPLETED, IN_PROGRESS, QUEUED, …
	State      string `json:"state"`      // a status context: SUCCESS, PENDING, FAILURE, ERROR, EXPECTED
}

type ghIssue struct {
	Number     int    `json:"number"`
	URL        string `json:"url"`
	Repository struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

type ghView struct {
	Title          string       `json:"title"`
	State          string       `json:"state"`
	IsDraft        bool         `json:"isDraft"`
	Mergeable      string       `json:"mergeable"`
	MergeState     string       `json:"mergeStateStatus"`
	ReviewDecision string       `json:"reviewDecision"`
	Rollup         []rollupItem `json:"statusCheckRollup"`
	Closes         []ghIssue    `json:"closingIssuesReferences"`
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
	status := Status{Title: view.Title, State: view.State, Draft: view.IsDraft, Mergeable: view.Mergeable, MergeState: view.MergeState, Review: view.ReviewDecision, At: now}
	for _, issue := range view.Closes {
		repo := ""
		if issue.Repository.Owner.Login != "" && issue.Repository.Name != "" {
			repo = issue.Repository.Owner.Login + "/" + issue.Repository.Name
		}
		status.Closes = append(status.Closes, Issue{Number: issue.Number, URL: issue.URL, Repo: repo})
	}
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
