package ghpr

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/dev1antdev/remuda/internal/shell"
)

// Step is one step of an Actions job.
type Step struct {
	Number    int    `json:"number"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Result    string `json:"conclusion,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
	DoneAt    string `json:"completed_at,omitempty"`
}

// Job is an Actions job with its steps.
type Job struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Result string `json:"conclusion,omitempty"`
	URL    string `json:"url"`
	Steps  []Step `json:"steps"`
}

// ReadJob reads one Actions job and its steps (a check run's id is its job's).
func ReadJob(host shell.Shell, repo Repo, job int64) (Job, error) {
	var raw struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		HTMLURL    string `json:"html_url"`
		Steps      []struct {
			Number      int    `json:"number"`
			Name        string `json:"name"`
			Status      string `json:"status"`
			Conclusion  string `json:"conclusion"`
			StartedAt   string `json:"started_at"`
			CompletedAt string `json:"completed_at"`
		} `json:"steps"`
	}
	if err := rest(host, "GET", fmt.Sprintf("repos/%s/actions/jobs/%d", repo, job), nil, &raw); err != nil {
		return Job{}, err
	}
	j := Job{ID: raw.ID, Name: raw.Name, Status: raw.Status, Result: raw.Conclusion, URL: raw.HTMLURL, Steps: []Step{}}
	for _, s := range raw.Steps {
		j.Steps = append(j.Steps, Step{Number: s.Number, Name: s.Name, Status: s.Status, Result: s.Conclusion, StartedAt: s.StartedAt, DoneAt: s.CompletedAt})
	}
	return j, nil
}

// LogRead is how much of a failed job's log the machine sends, its end; LogShown is how much of that the panel shows.
const (
	LogRead  = 60 * 1024
	LogShown = 8 * 1024
)

// Log is the end of a job's failed steps' output.
type Log struct {
	Text string `json:"text"`
	Cut  bool   `json:"cut"` // only its end is shown
}

// ReadLog reads the failed steps' output of a job, its last LogRead bytes on the machine, and keeps the last LogShown,
// gh's per-line job and step names, timestamps and the terminal's escapes stripped.
func ReadLog(host shell.Shell, repo Repo, job int64) (Log, error) {
	out, err := runKept(host, []string{"run", "view", "--job", strconv.FormatInt(job, 10), "-R", repo.String(), "--log-failed"}, "", lastBytes(LogRead), LogTimeout)
	if err != nil {
		return Log{}, err
	}
	return CleanLog(out), nil
}

var (
	logPrefix = regexp.MustCompile(`^[^\t\n]*\t[^\t\n]*\t\x{FEFF}?\S+ ?`)
	escapes   = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
)

// CleanLog is what a person wants of gh's failed output: each line without the job, the step and the timestamp gh
// starts it with, no colour codes, the last LogShown bytes from a line's start.
func CleanLog(text string) Log {
	text = strings.TrimPrefix(text, "\uFEFF")
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		lines[i] = escapes.ReplaceAllString(logPrefix.ReplaceAllString(line, ""), "")
	}
	clean := strings.Join(lines, "\n")
	cut := len(text) >= LogRead
	if len(clean) > LogShown {
		clean = clean[len(clean)-LogShown:]
		if nl := strings.IndexByte(clean, '\n'); nl >= 0 {
			clean = clean[nl+1:]
		}
		cut = true
	}
	return Log{Text: clean, Cut: cut}
}
