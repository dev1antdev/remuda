package prstatus

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// A GitLab merge request read as a GitHub pull request is: GitLab's own API through glab on the pane's machine, as the
// person, its answers put in the same Status, Check and Review the page and the hand-offs already speak.

var mergeURL = regexp.MustCompile(`^https?://([^/\s]+)/(.+?)(?:/-)?/merge_requests/(\d+)`)

// Merge is a GitLab merge request's address taken apart: the instance, the project's path, the request's number.
type Merge struct {
	Host    string
	Project string
	IID     int
}

// MergeParts reads the host, the project and the number off a merge request's address, gitlab.com's or a company's own.
func MergeParts(address string) (Merge, bool) {
	m := mergeURL.FindStringSubmatch(address)
	if m == nil {
		return Merge{}, false
	}
	iid, _ := strconv.Atoi(m[3])
	return Merge{Host: m[1], Project: m[2], IID: iid}, true
}

// GitLab says whether the request is a GitLab merge request, read with glab.
func GitLab(address string) bool {
	_, ok := MergeParts(address)
	return ok
}

// Readable says whether remuda can read the request at all: a pull request with gh, a merge request with glab.
func Readable(address string) bool { return GitHub(address) || GitLab(address) }

// Tool names the command that reads the request on the machine.
func Tool(address string) string {
	if GitLab(address) {
		return "glab"
	}
	return "gh"
}

// path is the API path of the request, with what follows it
func (m Merge) path(rest string) string {
	return "projects/" + url.PathEscape(m.Project) + "/merge_requests/" + strconv.Itoa(m.IID) + rest
}

// glabAPI is one call of GitLab's API through glab, signed in as the person on that instance
func glabAPI(host, path string) string {
	return "glab api --hostname " + script.Quote(host) + " " + script.Quote(path) + " 2>&1"
}

// MergeScript reads the request, its approvals and its discussions, in three sections.
func MergeScript(address string) string {
	m, ok := MergeParts(address)
	if !ok {
		return ""
	}
	return script.PATH + "\n" + glabAPI(m.Host, m.path("")) + "; printf '\\036\\n'; " +
		glabAPI(m.Host, m.path("/approvals")) + "; printf '\\036\\n'; " +
		glabAPI(m.Host, m.path("/discussions?per_page=100"))
}

// JobsScript lists the jobs of the request's pipeline, the latest try of each.
func JobsScript(m Merge, pipeline int64) string {
	return script.PATH + "\n" + glabAPI(m.Host, "projects/"+url.PathEscape(m.Project)+"/pipelines/"+strconv.FormatInt(pipeline, 10)+"/jobs?per_page=100")
}

// TraceScript prints a job's log.
func TraceScript(m Merge, job int64) string {
	return script.PATH + "\n" + glabAPI(m.Host, "projects/"+url.PathEscape(m.Project)+"/jobs/"+strconv.FormatInt(job, 10)+"/trace")
}

type glabMerge struct {
	State               string `json:"state"` // opened, closed, locked, merged
	Draft               bool   `json:"draft"`
	WorkInProgress      bool   `json:"work_in_progress"`
	HasConflicts        bool   `json:"has_conflicts"`
	DetailedMergeStatus string `json:"detailed_merge_status"`
	MergeStatus         string `json:"merge_status"`
	HeadPipeline        *struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	} `json:"head_pipeline"`
}

type glabApprovals struct {
	ApprovalsLeft int `json:"approvals_left"`
	ApprovedBy    []struct {
		User struct {
			Username string `json:"username"`
		} `json:"user"`
	} `json:"approved_by"`
}

type glabNote struct {
	Body       string `json:"body"`
	System     bool   `json:"system"`
	Resolvable bool   `json:"resolvable"`
	Resolved   bool   `json:"resolved"`
	Author     struct {
		Username string `json:"username"`
	} `json:"author"`
	Position *struct {
		NewPath string `json:"new_path"`
		OldPath string `json:"old_path"`
		NewLine int    `json:"new_line"`
		OldLine int    `json:"old_line"`
	} `json:"position"`
}

type glabDiscussion struct {
	Notes []glabNote `json:"notes"`
}

// open says whether a discussion is a thread still waiting: a resolvable note in it not resolved
func (d glabDiscussion) open() bool {
	for _, n := range d.Notes {
		if n.Resolvable && !n.Resolved {
			return true
		}
	}
	return false
}

// settled says whether a discussion was a thread and is resolved now
func (d glabDiscussion) settled() bool {
	resolvable := false
	for _, n := range d.Notes {
		if n.Resolvable {
			resolvable = true
			if !n.Resolved {
				return false
			}
		}
	}
	return resolvable
}

type glabJob struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Stage        string `json:"stage"`
	Status       string `json:"status"` // created, pending, running, success, failed, canceled, skipped, manual, …
	AllowFailure bool   `json:"allow_failure"`
	WebURL       string `json:"web_url"`
}

// ParseMerge reads MergeScript's answer into the request's Status, and the id of its pipeline (0 for none). The
// request's own section must parse — glab missing, not signed in, a request it cannot see are its first line — while
// the approvals and the discussions only add what they can.
func ParseMerge(out string, now time.Time) (Status, int64, error) {
	sections := script.Sections(out)
	var mr glabMerge
	if err := jsonAt(sections[0], "{", &mr); err != nil {
		return Status{}, 0, err
	}
	status := Status{State: mergeState(mr.State), Draft: mr.Draft || mr.WorkInProgress, Mergeable: "UNKNOWN", At: now}
	switch {
	case mr.HasConflicts:
		status.Mergeable = "CONFLICTING"
	case mr.DetailedMergeStatus == "mergeable" || mr.MergeStatus == "can_be_merged":
		status.Mergeable = "MERGEABLE"
	}
	var approvals glabApprovals
	approvalsRead := len(sections) > 1 && jsonAt(sections[1], "{", &approvals) == nil
	switch {
	case mr.DetailedMergeStatus == "requested_changes":
		status.Review = "CHANGES_REQUESTED"
	case approvalsRead && len(approvals.ApprovedBy) > 0 && approvals.ApprovalsLeft == 0:
		status.Review = "APPROVED"
	case approvalsRead && approvals.ApprovalsLeft > 0, mr.DetailedMergeStatus == "not_approved":
		status.Review = "REVIEW_REQUIRED"
	}
	var discussions []glabDiscussion
	if len(sections) > 2 && jsonAt(sections[2], "[", &discussions) == nil {
		for _, d := range discussions {
			if d.open() {
				status.Threads++
			}
		}
	}
	var pipeline int64
	if mr.HeadPipeline != nil {
		pipeline = mr.HeadPipeline.ID
		status.Checks = countChecks([]Check{{Name: "pipeline", Bucket: jobBucket(glabJob{Status: mr.HeadPipeline.Status})}})
	}
	return status, pipeline, nil
}

func mergeState(state string) string {
	switch state {
	case "merged":
		return "MERGED"
	case "closed":
		return "CLOSED"
	}
	return "OPEN"
}

// ParseJobs reads a pipeline's jobs as the request's checks.
func ParseJobs(out string) ([]Check, error) {
	var jobs []glabJob
	if err := jsonAt(out, "[", &jobs); err != nil {
		return nil, err
	}
	checks := make([]Check, 0, len(jobs))
	for _, j := range jobs {
		checks = append(checks, Check{Name: j.Name, State: strings.ToUpper(j.Status), Bucket: jobBucket(j), Link: j.WebURL, Workflow: j.Stage, Job: j.ID})
	}
	return checks, nil
}

// jobBucket is a job's standing in gh's words: a failure allowed to fail is skipped, a cancel needs a hand as on GitHub
func jobBucket(j glabJob) string {
	switch j.Status {
	case "success":
		return "pass"
	case "failed":
		if j.AllowFailure {
			return "skipping"
		}
		return "fail"
	case "canceled", "canceling":
		return "cancel"
	case "skipped", "manual":
		return "skipping"
	}
	return "pending"
}

// countChecks adds the checks up by their bucket.
func countChecks(checks []Check) Checks {
	var c Checks
	for _, check := range checks {
		c.Total++
		switch check.Bucket {
		case "pass":
			c.Pass++
		case "fail", "cancel":
			c.Fail++
		case "skipping":
			c.Skipped++
		default:
			c.Pending++
		}
	}
	return c
}

// fetchMerge reads a merge request with glab: the request, its approvals and discussions, then its pipeline's jobs as
// the checks; a pipeline whose jobs cannot be listed counts as its one state.
func fetchMerge(host shell.Shell, address string, now time.Time) (*Status, string) {
	out, err := shell.Text(host, MergeScript(address), Timeout)
	status, pipeline, perr := ParseMerge(out, now)
	if perr != nil {
		return nil, answerError(out, err, perr).Error()
	}
	if pipeline != 0 {
		m, _ := MergeParts(address)
		jobsOut, _ := shell.Text(host, JobsScript(m, pipeline), Timeout)
		if checks, err := ParseJobs(jobsOut); err == nil && len(checks) > 0 {
			status.Checks = countChecks(checks)
		}
	}
	return &status, ""
}

// mergeFailure reads the request's pipeline on the machine, the failed jobs' logs, and composes what the agent is told.
func mergeFailure(host shell.Shell, label, address string) (string, error) {
	m, _ := MergeParts(address)
	out, err := shell.Text(host, script.PATH+"\n"+glabAPI(m.Host, m.path("")), Timeout)
	var mr glabMerge
	if perr := jsonAt(out, "{", &mr); perr != nil {
		return "", answerError(out, err, perr)
	}
	if mr.HeadPipeline == nil {
		return "", ErrNothingFailed
	}
	jobsOut, err := shell.Text(host, JobsScript(m, mr.HeadPipeline.ID), Timeout)
	checks, perr := ParseJobs(jobsOut)
	if perr != nil {
		return "", answerError(jobsOut, err, perr)
	}
	var logs []runLog
	for _, c := range checks {
		if !c.Failed() || c.Job == 0 {
			continue
		}
		trace, _ := shell.Text(host, TraceScript(m, c.Job), LogTimeout)
		logs = append(logs, runLog{heading: fmt.Sprintf("Job %s (stage %s), the end of its log:", c.Name, c.Workflow), text: trace})
		if len(logs) == MaxRuns {
			break
		}
	}
	return failureMessage(label, address, checks, logs)
}

// DiscussionsScript reads the request's discussions.
func DiscussionsScript(address string) string {
	m, ok := MergeParts(address)
	if !ok {
		return ""
	}
	return script.PATH + "\n" + glabAPI(m.Host, m.path("/discussions?per_page=100"))
}

// ParseDiscussions reads the threads still open and the comments outside threads as the reviewers' word: a thread on
// a line becomes line comments, its replies marked, anything else a note by its author. A thread resolved is done
// with and left out, and so is what GitLab itself wrote.
func ParseDiscussions(out string) (Review, error) {
	var discussions []glabDiscussion
	if err := jsonAt(out, "[", &discussions); err != nil {
		return Review{}, err
	}
	var review Review
	for _, d := range discussions {
		if d.settled() {
			continue
		}
		first := true
		for _, n := range d.Notes {
			body := strings.TrimSpace(n.Body)
			if n.System || body == "" {
				continue
			}
			if n.Position == nil {
				review.Reviews = append(review.Reviews, ReviewNote{Author: n.Author.Username, State: "COMMENTED", Body: body})
				continue
			}
			path, line := n.Position.NewPath, n.Position.NewLine
			if line == 0 {
				path, line = n.Position.OldPath, n.Position.OldLine
			}
			comment := LineComment{Author: n.Author.Username, Path: path, Line: line, Body: body}
			if !first {
				comment.ReplyTo = 1
			}
			review.Comments = append(review.Comments, comment)
			first = false
		}
	}
	return review, nil
}

// mergeReview reads the request's open threads and comments on the machine and composes what the agent is told.
func mergeReview(host shell.Shell, label, address string) (string, error) {
	out, err := shell.Text(host, DiscussionsScript(address), Timeout)
	review, perr := ParseDiscussions(out)
	if perr != nil {
		return "", answerError(out, err, perr)
	}
	return ReviewMessage(label, address, review)
}
