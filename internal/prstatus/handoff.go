package prstatus

import (
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

// LogTimeout is how long reading one run's log may take: gh fetches the whole log archive first.
var LogTimeout = 90 * time.Second

// Failure reads the request's checks on the machine, then the failed runs' logs, and composes what the agent is told.
func Failure(host shell.Shell, label, url string) (string, error) {
	out, err := shell.Text(host, ChecksScript(url), Timeout)
	checks, perr := ParseChecks(out)
	if perr != nil {
		return "", ghError(out, err, perr)
	}
	logs := map[string]string{}
	for _, id := range FailedRuns(checks) {
		log, _ := shell.Text(host, LogScript(id), LogTimeout)
		logs[id] = log
	}
	return FailureMessage(label, url, checks, logs)
}

// ReviewComments reads the request's reviews and line comments on the machine and composes what the agent is told.
func ReviewComments(host shell.Shell, label, url string) (string, error) {
	out, err := shell.Text(host, ReviewScript(url), Timeout)
	review, perr := ParseReview(out)
	if perr != nil {
		return "", ghError(out, err, perr)
	}
	return ReviewMessage(label, url, review)
}

// ghError is the better of the two complaints: the shell's when gh printed nothing, else what the answer said.
func ghError(out string, run, parse error) error {
	if run != nil && strings.TrimSpace(out) == "" {
		return run
	}
	return parse
}
