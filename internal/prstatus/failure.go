package prstatus

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dev1antdev/remuda/internal/script"
)

// A red check handed to the agent: the request's checks read with gh, the failed ones' run logs, one message.

// LogTail is how much of a failed run's log goes to the agent: its end, where the failure is.
const LogTail = 8 * 1024

// MaxRuns is how many failed runs' logs one message carries.
const MaxRuns = 3

// Check is one of a request's checks as gh pr checks lists it.
type Check struct {
	Name     string `json:"name"`
	State    string `json:"state"`
	Bucket   string `json:"bucket"` // pass, fail, pending, skipping, cancel
	Link     string `json:"link"`
	Workflow string `json:"workflow"`
}

// Failed says whether the check needs a hand: failed or cancelled.
func (c Check) Failed() bool { return c.Bucket == "fail" || c.Bucket == "cancel" }

var runLink = regexp.MustCompile(`/actions/runs/(\d+)`)

// RunID is the GitHub Actions run a check's link points into, "" for a check from elsewhere.
func (c Check) RunID() string {
	m := runLink.FindStringSubmatch(c.Link)
	if m == nil {
		return ""
	}
	return m[1]
}

// ChecksScript lists a request's checks on the pane's machine.
func ChecksScript(url string) string {
	return script.PATH + "\ngh pr checks " + script.Quote(url) + " --json name,state,link,bucket,workflow 2>&1"
}

// ParseChecks reads gh pr checks' JSON; anything else is the error's text.
func ParseChecks(out string) ([]Check, error) {
	trimmed := strings.TrimSpace(out)
	at := strings.Index(trimmed, "[")
	if at < 0 {
		return nil, errors.New(firstLine(trimmed, "gh answered nothing"))
	}
	var checks []Check
	if err := json.Unmarshal([]byte(trimmed[at:]), &checks); err != nil {
		return nil, errors.New(firstLine(trimmed, err.Error()))
	}
	return checks, nil
}

// FailedRuns is every GitHub Actions run a failed check points into, each once, in the checks' order, at most MaxRuns.
func FailedRuns(checks []Check) []string {
	var runs []string
	seen := map[string]bool{}
	for _, c := range checks {
		id := c.RunID()
		if !c.Failed() || id == "" || seen[id] {
			continue
		}
		seen[id] = true
		runs = append(runs, id)
		if len(runs) == MaxRuns {
			break
		}
	}
	return runs
}

// LogScript prints the log of a run's failed steps.
func LogScript(runID string) string {
	return script.PATH + "\ngh run view " + script.Quote(runID) + " --log-failed 2>&1"
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// Tail is the end of a log, at most LogTail bytes, starting at a line, the escapes stripped.
func Tail(log string) string {
	log = ansi.ReplaceAllString(log, "")
	log = strings.TrimRight(log, "\n")
	if len(log) <= LogTail {
		return log
	}
	cut := log[len(log)-LogTail:]
	if nl := strings.IndexByte(cut, '\n'); nl >= 0 {
		cut = cut[nl+1:]
	}
	return "…\n" + cut
}

// ErrNothingFailed says every check of the request passed or still runs.
var ErrNothingFailed = errors.New("no check of the request has failed")

// FailureMessage is what the agent is told: which checks failed, where, and the end of each failed run's log.
func FailureMessage(label, url string, checks []Check, logs map[string]string) (string, error) {
	var failed []Check
	for _, c := range checks {
		if c.Failed() {
			failed = append(failed, c)
		}
	}
	if len(failed) == 0 {
		return "", ErrNothingFailed
	}
	names := make([]string, 0, len(failed))
	for _, c := range failed {
		names = append(names, c.Name)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "CI failed on %s (%s): %s.\n", label, url, strings.Join(names, ", "))
	for _, c := range failed {
		if c.Link != "" {
			fmt.Fprintf(&b, "- %s: %s\n", c.Name, c.Link)
		}
	}
	runs := make([]string, 0, len(logs))
	for id := range logs {
		runs = append(runs, id)
	}
	sort.Strings(runs)
	for _, id := range runs {
		tail := Tail(logs[id])
		if tail == "" {
			continue
		}
		fmt.Fprintf(&b, "\nThe failed steps of run %s, the end of their log:\n\n```\n%s\n```\n", id, tail)
	}
	b.WriteString("\nFix what the log shows, push the fix to the branch, and say when the checks are green again.")
	return b.String(), nil
}
