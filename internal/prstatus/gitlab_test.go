package prstatus

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

const mrURL = "https://gitlab.gog.com/gog-backend/games-db/-/merge_requests/613"

// the answers GitLab's API gives through glab api, trimmed to the fields remuda reads and a few it does not
const (
	mrOpen = `{"id":91,"iid":613,"project_id":44,"title":"Refresh the connection","state":"opened","draft":false,"work_in_progress":false,` +
		`"has_conflicts":false,"detailed_merge_status":"mergeable","merge_status":"can_be_merged","blocking_discussions_resolved":false,` +
		`"head_pipeline":{"id":9001,"iid":12,"project_id":44,"sha":"abc","ref":"fix/x","status":"failed","web_url":"https://gitlab.gog.com/gog-backend/games-db/-/pipelines/9001"},` +
		`"web_url":"https://gitlab.gog.com/gog-backend/games-db/-/merge_requests/613"}`
	approvedByAnna = `{"id":91,"iid":613,"approved":true,"approvals_required":1,"approvals_left":0,"approved_by":[{"user":{"id":7,"username":"anna","name":"Anna"}}]}`
	discussions    = `[` +
		`{"id":"d1","individual_note":false,"notes":[` +
		`{"id":1,"type":"DiffNote","body":"This drops the error.","author":{"username":"anna"},"system":false,"resolvable":true,"resolved":false,` +
		`"position":{"base_sha":"a","start_sha":"b","head_sha":"c","old_path":"src/Sync.php","new_path":"src/Sync.php","position_type":"text","old_line":null,"new_line":42}},` +
		`{"id":2,"type":"DiffNote","body":"Agreed, keep it.","author":{"username":"bob"},"system":false,"resolvable":true,"resolved":false,` +
		`"position":{"old_path":"src/Sync.php","new_path":"src/Sync.php","old_line":null,"new_line":42}}]},` +
		`{"id":"d2","individual_note":false,"notes":[{"id":3,"type":"DiffNote","body":"Typo.","author":{"username":"anna"},"system":false,"resolvable":true,"resolved":true,` +
		`"position":{"old_path":"README.md","new_path":"README.md","old_line":3,"new_line":3}}]},` +
		`{"id":"d3","individual_note":true,"notes":[{"id":4,"type":null,"body":"Please add a test for the retry.","author":{"username":"carl"},"system":false,"resolvable":false,"resolved":false}]},` +
		`{"id":"d4","individual_note":true,"notes":[{"id":5,"type":null,"body":"added 1 commit","author":{"username":"bot"},"system":true,"resolvable":false,"resolved":false}]},` +
		`{"id":"d5","individual_note":false,"notes":[{"id":6,"type":"DiffNote","body":"Old line went where?","author":{"username":"dana"},"system":false,"resolvable":true,"resolved":false,` +
		`"position":{"old_path":"src/Old.php","new_path":"src/Old.php","old_line":17,"new_line":null}}]}` +
		`]`
	jobs = `[` +
		`{"id":71,"name":"lint","stage":"test","status":"success","allow_failure":false,"web_url":"https://gitlab.gog.com/gog-backend/games-db/-/jobs/71"},` +
		`{"id":77,"name":"unit","stage":"test","status":"failed","allow_failure":false,"web_url":"https://gitlab.gog.com/gog-backend/games-db/-/jobs/77"},` +
		`{"id":78,"name":"audit","stage":"test","status":"failed","allow_failure":true,"web_url":"https://gitlab.gog.com/gog-backend/games-db/-/jobs/78"},` +
		`{"id":79,"name":"e2e","stage":"test","status":"canceled","allow_failure":false,"web_url":"https://gitlab.gog.com/gog-backend/games-db/-/jobs/79"},` +
		`{"id":80,"name":"deploy-review","stage":"deploy","status":"manual","allow_failure":true,"web_url":"https://gitlab.gog.com/gog-backend/games-db/-/jobs/80"},` +
		`{"id":81,"name":"smoke","stage":"deploy","status":"running","allow_failure":false,"web_url":"https://gitlab.gog.com/gog-backend/games-db/-/jobs/81"}` +
		`]`
)

func sections(parts ...string) string { return strings.Join(parts, script.RS+"\n") }

func TestMergePartsReadsTheAddressOfEveryKind(t *testing.T) {
	for address, want := range map[string]Merge{
		mrURL: {Host: "gitlab.gog.com", Project: "gog-backend/games-db", IID: 613},
		"https://gitlab.com/group/sub/project/-/merge_requests/9#note_44": {Host: "gitlab.com", Project: "group/sub/project", IID: 9},
		"https://gitlab.example.org/team/app/merge_requests/3":            {Host: "gitlab.example.org", Project: "team/app", IID: 3},
	} {
		if got, ok := MergeParts(address); !ok || got != want {
			t.Fatalf("%s: %+v %v", address, got, ok)
		}
	}
	if _, ok := MergeParts("https://github.com/o/r/pull/7"); ok {
		t.Fatal("a pull request taken for a merge request")
	}
	if Tool(mrURL) != "glab" || Tool("https://github.com/o/r/pull/7") != "gh" || !GitLab(mrURL) || GitHub(mrURL) {
		t.Fatal("tool")
	}
}

func TestMergeScriptAsksTheInstanceTheRequestIsOn(t *testing.T) {
	s := MergeScript(mrURL)
	for _, want := range []string{
		"glab api --hostname 'gitlab.gog.com' 'projects/gog-backend%2Fgames-db/merge_requests/613' 2>&1; printf '\\036\\n';",
		"'projects/gog-backend%2Fgames-db/merge_requests/613/approvals'",
		"'projects/gog-backend%2Fgames-db/merge_requests/613/discussions?per_page=100'",
	} {
		if !strings.Contains(s, want) || !strings.HasPrefix(s, "PATH=") {
			t.Fatalf("script lacks %q:\n%s", want, s)
		}
	}
	m, _ := MergeParts(mrURL)
	if s := JobsScript(m, 9001); !strings.Contains(s, "'projects/gog-backend%2Fgames-db/pipelines/9001/jobs?per_page=100'") {
		t.Fatalf("jobs = %q", s)
	}
	if s := TraceScript(m, 77); !strings.Contains(s, "'projects/gog-backend%2Fgames-db/jobs/77/trace'") {
		t.Fatalf("trace = %q", s)
	}
}

func TestParseMergeSpeaksGhsWords(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	status, pipeline, err := ParseMerge(sections(mrOpen, approvedByAnna, discussions), now)
	if err != nil {
		t.Fatal(err)
	}
	want := Status{State: "OPEN", Mergeable: "MERGEABLE", Review: "APPROVED", Threads: 2, Checks: Checks{Total: 1, Fail: 1}, At: now}
	if status != want || pipeline != 9001 {
		t.Fatalf("status = %+v, pipeline %d", status, pipeline)
	}

	merged := strings.Replace(mrOpen, `"state":"opened"`, `"state":"merged"`, 1)
	if s, _, _ := ParseMerge(sections(merged, approvedByAnna, "[]"), now); s.State != "MERGED" || s.Threads != 0 {
		t.Fatalf("merged = %+v", s)
	}
	closedDraft := strings.NewReplacer(`"state":"opened"`, `"state":"closed"`, `"draft":false`, `"draft":true`).Replace(mrOpen)
	if s, _, _ := ParseMerge(sections(closedDraft, approvedByAnna, "[]"), now); s.State != "CLOSED" || !s.Draft {
		t.Fatalf("closed draft = %+v", s)
	}
	conflicts := strings.NewReplacer(`"has_conflicts":false`, `"has_conflicts":true`, `"detailed_merge_status":"mergeable"`, `"detailed_merge_status":"broken_status"`).Replace(mrOpen)
	waiting := `{"approvals_required":2,"approvals_left":1,"approved_by":[{"user":{"username":"anna"}}]}`
	if s, _, _ := ParseMerge(sections(conflicts, waiting, "[]"), now); s.Mergeable != "CONFLICTING" || s.Review != "REVIEW_REQUIRED" {
		t.Fatalf("conflicts = %+v", s)
	}
	changes := strings.Replace(mrOpen, `"detailed_merge_status":"mergeable"`, `"detailed_merge_status":"requested_changes"`, 1)
	if s, _, _ := ParseMerge(sections(changes, approvedByAnna, "[]"), now); s.Review != "CHANGES_REQUESTED" {
		t.Fatalf("changes = %+v", s)
	}
	nobodyNeeded := `{"approved":true,"approvals_required":0,"approvals_left":0,"approved_by":[]}`
	noPipeline := strings.Replace(mrOpen, mrOpen[strings.Index(mrOpen, `"head_pipeline"`):strings.Index(mrOpen, `"web_url":"https://gitlab.gog.com/gog-backend/games-db/-/merge_requests`)], `"head_pipeline":null,`, 1)
	s, pipeline, err := ParseMerge(sections(noPipeline, nobodyNeeded, "glab: 403 Forbidden"), now)
	if err != nil || s.Review != "" || s.Checks != (Checks{}) || pipeline != 0 || s.Threads != 0 {
		t.Fatalf("no approvals needed, no pipeline, discussions refused = %+v %d %v", s, pipeline, err)
	}
}

func TestParseMergeKeepsGlabsComplaint(t *testing.T) {
	for _, out := range []string{"sh: glab: command not found\n", "glab: 401 Unauthorized (HTTP 401)\n", "glab: 404 Project Not Found (HTTP 404)"} {
		_, _, err := ParseMerge(out, time.Now())
		if err == nil || err.Error() != strings.TrimSpace(strings.SplitN(out, "\n", 2)[0]) {
			t.Fatalf("%q: %v", out, err)
		}
	}
}

func TestJobsCountAsChecks(t *testing.T) {
	checks, err := ParseJobs(jobs)
	if err != nil {
		t.Fatal(err)
	}
	want := Checks{Total: 6, Pass: 1, Fail: 2, Pending: 1, Skipped: 2}
	if got := countChecks(checks); got != want {
		t.Fatalf("checks = %+v", got)
	}
	if !checks[1].Failed() || checks[2].Failed() || !checks[3].Failed() || checks[1].Job != 77 || checks[1].Workflow != "test" {
		t.Fatalf("checks = %+v", checks)
	}
}

// glabFake answers each glab call by the API path it asks for, as GitLab would
func glabFake(answers map[string]string) *shell.Fake {
	return &shell.Fake{Name: "gog", Remote: true, Answer: func(s string) (string, error) {
		if strings.Contains(s, "/approvals'") {
			out := []string{}
			for _, path := range []string{"merge_requests/613'", "/approvals'", "/discussions?per_page=100'"} {
				out = append(out, answers[path])
			}
			return sections(out...), nil
		}
		for path, answer := range answers {
			if strings.Contains(s, path) {
				return answer, nil
			}
		}
		return "glab: 404 Not Found (HTTP 404)", &shell.ExitError{Code: 1}
	}}
}

var gogAnswers = map[string]string{
	"merge_requests/613'":                mrOpen,
	"/approvals'":                        approvedByAnna,
	"/discussions?per_page=100'":         discussions,
	"/pipelines/9001/jobs?per_page=100'": jobs,
	"/jobs/77/trace'":                    "\x1b[0KRunning tests\nFAIL: TestRefresh (0.01s)\n    sync_test.go:42: connection gone\n",
	"/jobs/79/trace'":                    "",
}

func TestTrackerReadsAMergeRequestWithItsJobs(t *testing.T) {
	host := glabFake(gogAnswers)
	tr := New(func(id string) (shell.Shell, bool) { return host, id == "gog" })
	ref := events.PaneRef{Machine: "gog", Pane: "w1P:p1"}
	var heard []events.PullRequestState
	done := make(chan struct{}, 1)
	tr.OnState = func(ev events.PullRequestState) { heard = append(heard, ev); done <- struct{}{} }
	tr.OnNamed(events.PullRequestNamed{Pane: ref, URL: mrURL, Label: "MR !613"})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("no read")
	}
	status, ok := tr.Status("gog", "w1P:p1")
	if !ok || status.Checks != (Checks{Total: 6, Pass: 1, Fail: 2, Pending: 1, Skipped: 2}) || status.Review != "APPROVED" || status.Threads != 2 {
		t.Fatalf("status = %+v", status)
	}
	if len(heard) != 1 || heard[0].State != "OPEN" || heard[0].URL != mrURL {
		t.Fatalf("heard = %+v", heard)
	}
	if got := host.Scripts(); len(got) != 2 || !strings.Contains(got[1], "/pipelines/9001/jobs") {
		t.Fatalf("scripts = %q", got)
	}
}

func TestFailureHandsThePipelinesFailedJobsToTheAgent(t *testing.T) {
	msg, err := Failure(glabFake(gogAnswers), "MR !613", mrURL)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"CI failed on MR !613 (" + mrURL + "): unit, e2e.",
		"- unit: https://gitlab.gog.com/gog-backend/games-db/-/jobs/77",
		"Job unit (stage test), the end of its log:\n\n```\nRunning tests\nFAIL: TestRefresh (0.01s)\n    sync_test.go:42: connection gone\n```",
		"Fix what the log shows, push the fix to the branch, and say when the checks are green again.",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message lacks %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "audit") || strings.Contains(msg, "Job e2e") {
		t.Fatalf("a failure allowed to fail, or a job without a log, got a section:\n%s", msg)
	}

	green := map[string]string{"merge_requests/613'": strings.Replace(mrOpen, `"status":"failed"`, `"status":"success"`, 1), "/pipelines/9001/jobs?per_page=100'": `[{"id":71,"name":"lint","stage":"test","status":"success"}]`}
	if _, err := Failure(glabFake(green), "MR !613", mrURL); !errors.Is(err, ErrNothingFailed) {
		t.Fatalf("green pipeline: %v", err)
	}
	if _, err := Failure(&shell.Fake{Name: "gog", Answer: func(string) (string, error) { return "sh: glab: command not found\n", &shell.ExitError{Code: 127} }}, "MR !613", mrURL); err == nil || err.Error() != "sh: glab: command not found" {
		t.Fatalf("no glab: %v", err)
	}
}

func TestReviewHandsTheOpenThreadsToTheAgent(t *testing.T) {
	msg, err := ReviewComments(glabFake(gogAnswers), "MR !613", mrURL)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Review of MR !613 (" + mrURL + "):",
		"carl wrote:\n   Please add a test for the retry.",
		"Comments (3):",
		"1. src/Sync.php:42 (anna)\n   This drops the error.",
		"2. src/Sync.php:42 (bob, in reply)\n   Agreed, keep it.",
		"3. src/Old.php:17 (dana)\n   Old line went where?",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message lacks %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "Typo") || strings.Contains(msg, "added 1 commit") {
		t.Fatalf("a resolved thread or GitLab's own note went to the agent:\n%s", msg)
	}
	if _, err := ReviewComments(glabFake(map[string]string{"/discussions?per_page=100'": "[]"}), "MR !613", mrURL); !errors.Is(err, ErrNoReview) {
		t.Fatalf("no threads: %v", err)
	}
}
