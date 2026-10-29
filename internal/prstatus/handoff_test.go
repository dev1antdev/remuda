package prstatus

import (
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/shell"
)

const checksJSON = `[{"bucket":"skipping","link":"https://github.com/o/r/actions/runs/40693613487/job/1228","name":"end-to-end smoke","state":"SKIPPED","workflow":"CI"},` +
	`{"bucket":"fail","link":"https://github.com/o/r/actions/runs/40693613487/job/1229","name":"build, test, scan","state":"FAILURE","workflow":"CI"},` +
	`{"bucket":"fail","link":"https://github.com/o/r/actions/runs/40693613487/job/1230","name":"secret scan (whole history)","state":"FAILURE","workflow":"CI"},` +
	`{"bucket":"cancel","link":"https://github.com/o/r/actions/runs/40693613499/job/1231","name":"docs","state":"CANCELLED","workflow":"Docs"},` +
	`{"bucket":"fail","link":"https://ci.example.com/build/7","name":"external","state":"FAILURE","workflow":""},` +
	`{"bucket":"pass","link":"https://github.com/o/r/actions/runs/40693613487/job/1232","name":"lint","state":"SUCCESS","workflow":"CI"}]`

func TestChecksParseAndTheFailedRunsAreNamedOnce(t *testing.T) {
	checks, err := ParseChecks("some warning first\n" + checksJSON)
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 6 || !checks[1].Failed() || checks[0].Failed() || checks[5].Failed() || !checks[3].Failed() {
		t.Fatalf("checks = %+v", checks)
	}
	if runs := FailedRuns(checks); strings.Join(runs, ",") != "40693613487,40693613499" {
		t.Fatalf("runs = %v", runs)
	}
	if checks[4].RunID() != "" || checks[1].RunID() != "40693613487" {
		t.Fatal("run ids")
	}
	if _, err := ParseChecks("no pull requests found for branch\n"); err == nil || err.Error() != "no pull requests found for branch" {
		t.Fatalf("err = %v", err)
	}
	if s := ChecksScript("https://github.com/o/r/pull/7"); !strings.Contains(s, "gh pr checks 'https://github.com/o/r/pull/7' --json name,state,link,bucket,workflow") {
		t.Fatalf("script = %q", s)
	}
	if s := LogScript("123"); !strings.Contains(s, "gh run view '123' --log-failed") {
		t.Fatalf("script = %q", s)
	}
}

func TestTailKeepsTheEndWholeLines(t *testing.T) {
	short := "\x1b[32mok\x1b[0m line\n"
	if Tail(short) != "ok line" {
		t.Fatalf("tail = %q", Tail(short))
	}
	var b strings.Builder
	for i := 0; i < 2000; i++ {
		b.WriteString("line number ")
		b.WriteString(strings.Repeat("x", 20))
		b.WriteString("\n")
	}
	tail := Tail(b.String())
	if len(tail) > LogTail+2 || !strings.HasPrefix(tail, "…\nline number") || strings.HasSuffix(tail, "\n") {
		t.Fatalf("tail is %d bytes, starts %q", len(tail), tail[:20])
	}
}

func TestFailureMessageNamesTheChecksAndCarriesTheLogs(t *testing.T) {
	checks, _ := ParseChecks(checksJSON)
	msg, err := FailureMessage("PR #15", "https://github.com/o/r/pull/15", checks, map[string]string{"40693613487": "test\tgo test\t2026-10-29T10:00:00Z FAIL: TestX\n", "40693613499": ""})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"CI failed on PR #15 (https://github.com/o/r/pull/15): build, test, scan, secret scan (whole history), docs, external.",
		"- build, test, scan: https://github.com/o/r/actions/runs/40693613487/job/1229",
		"- external: https://ci.example.com/build/7",
		"The failed steps of run 40693613487, the end of their log:\n\n```\ntest\tgo test\t2026-10-29T10:00:00Z FAIL: TestX\n```",
		"Fix what the log shows, push the fix to the branch, and say when the checks are green again.",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message lacks %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "run 40693613499") {
		t.Fatal("a run with no log read gets no section")
	}
	green := []Check{{Name: "build", Bucket: "pass"}, {Name: "e2e", Bucket: "skipping"}, {Name: "slow", Bucket: "pending"}}
	if _, err := FailureMessage("PR #1", "u", green, nil); err != ErrNothingFailed {
		t.Fatalf("err = %v", err)
	}
}

func TestFailureRunsTheChecksThenEachFailedRunsLog(t *testing.T) {
	host := &shell.Fake{Name: "box", Answer: func(s string) (string, error) {
		switch {
		case strings.Contains(s, "gh pr checks"):
			return checksJSON, nil
		case strings.Contains(s, "gh run view '40693613487'"):
			return "build\tTest\t2026-10-29T10:00:00Z ##[error]Process completed with exit code 1.\n", &shell.ExitError{Code: 1}
		case strings.Contains(s, "gh run view '40693613499'"):
			return "docs\tBuild\t2026-10-29T10:00:00Z cancelled\n", nil
		}
		return "", nil
	}}
	msg, err := Failure(host, "PR #15", "https://github.com/o/r/pull/15")
	if err != nil {
		t.Fatal(err)
	}
	if got := host.Scripts(); len(got) != 3 || !strings.Contains(got[1], "'40693613487' --log-failed") || !strings.Contains(got[2], "'40693613499' --log-failed") {
		t.Fatalf("scripts = %q", got)
	}
	if !strings.Contains(msg, "##[error]Process completed with exit code 1.") || !strings.Contains(msg, "docs\tBuild") {
		t.Fatalf("message:\n%s", msg)
	}
	away := &shell.Fake{Name: "box", Answer: func(string) (string, error) {
		return "", &shell.ExitError{Code: 255, Stderr: "ssh: connect to host box: no route"}
	}}
	if _, err := Failure(away, "PR #15", "u"); err == nil || !strings.Contains(err.Error(), "no route") {
		t.Fatalf("err = %v", err)
	}
	nogh := &shell.Fake{Name: "box", Answer: func(string) (string, error) { return "sh: gh: command not found\n", &shell.ExitError{Code: 127} }}
	if _, err := Failure(nogh, "PR #15", "u"); err == nil || err.Error() != "sh: gh: command not found" {
		t.Fatalf("err = %v", err)
	}
}

const reviewsJSON = `{"reviews":[{"author":{"login":"alice"},"body":"Two things before this lands.","state":"CHANGES_REQUESTED"},` +
	`{"author":{"login":"bot"},"body":"","state":"COMMENTED"},{"author":{"login":"bob"},"body":"","state":"CHANGES_REQUESTED"}]}`

const commentsJSON = `[{"user":{"login":"alice"},"path":"internal/x.go","line":42,"original_line":40,"body":"This leaks the lock.","diff_hunk":"@@ -40,3 +40,4 @@\n func a() {\n+\tmu.Lock()\n+\treturn","in_reply_to_id":0},` +
	`{"user":{"login":"carol"},"path":"internal/x.go","line":0,"original_line":12,"body":"Name it.\nTwo lines.","diff_hunk":"@@ -10,2 +12,2 @@\n-\tx := 1","in_reply_to_id":0},` +
	`{"user":{"login":"alice"},"path":"internal/x.go","line":42,"original_line":40,"body":"Still leaks.","diff_hunk":"","in_reply_to_id":77}]`

func TestReviewParsesBothSectionsAndTheMessageReadsLikeTheLens(t *testing.T) {
	owner, repo, number, ok := URLParts("https://github.com/dev1antdev/remuda/pull/115?x=1")
	if !ok || owner != "dev1antdev" || repo != "remuda" || number != 115 {
		t.Fatalf("parts = %s %s %d %v", owner, repo, number, ok)
	}
	if _, _, _, ok := URLParts("https://gitlab.com/o/r/-/merge_requests/3"); ok {
		t.Fatal("a GitLab address has no parts")
	}
	s := ReviewScript("https://github.com/o/r/pull/15")
	if !strings.Contains(s, "gh pr view 'https://github.com/o/r/pull/15' --json reviews") || !strings.Contains(s, "gh api 'repos/o/r/pulls/15/comments?per_page=100'") || !strings.Contains(s, `printf '\036\n'`) {
		t.Fatalf("script = %q", s)
	}
	review, err := ParseReview(reviewsJSON + "\n\x1e\n" + commentsJSON)
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Reviews) != 2 || review.Reviews[0].Author != "alice" || review.Reviews[1].Author != "bob" || review.Reviews[1].Body != "" {
		t.Fatalf("reviews = %+v", review.Reviews)
	}
	if len(review.Comments) != 3 || review.Comments[0].Line != 42 || review.Comments[1].Line != 12 || review.Comments[0].Code != "+\treturn" || review.Comments[2].ReplyTo != 77 {
		t.Fatalf("comments = %+v", review.Comments)
	}
	msg, err := ReviewMessage("PR #15", "https://github.com/o/r/pull/15", review)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Review of PR #15 (https://github.com/o/r/pull/15):",
		"alice requested changes:\n   Two things before this lands.",
		"bob requested changes",
		"Comments (3):",
		"1. internal/x.go:42 (alice)\n   > +\treturn\n   This leaks the lock.",
		"2. internal/x.go:12 (carol)\n   > -\tx := 1\n   Name it.\n   Two lines.",
		"3. internal/x.go:42 (alice, in reply)\n   Still leaks.",
		"Work on what is asked above, push the branch, and say when it is ready for another look.",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message lacks %q:\n%s", want, msg)
		}
	}
	if _, err := ReviewMessage("PR #1", "u", Review{}); err != ErrNoReview {
		t.Fatalf("err = %v", err)
	}
	if _, err := ParseReview("gh: Not Found (HTTP 404)\n"); err == nil || !strings.Contains(err.Error(), "Not Found") {
		t.Fatalf("err = %v", err)
	}
	host := &shell.Fake{Name: "box", Answer: func(string) (string, error) { return reviewsJSON + "\n\x1e\n" + commentsJSON, nil }}
	if msg2, err := ReviewComments(host, "PR #15", "https://github.com/o/r/pull/15"); err != nil || msg2 != msg {
		t.Fatalf("over the shell: %v", err)
	}
}

func TestTrackerAnnouncesTheStateOnTheFirstReadAndOnAMove(t *testing.T) {
	state := "OPEN"
	host := &shell.Fake{Name: "box", Answer: func(string) (string, error) {
		return strings.Replace(mixed, `"state":"OPEN"`, `"state":"`+state+`"`, 1), nil
	}}
	clock := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	tr := New(func(string) (shell.Shell, bool) { return host, true })
	tr.now = func() time.Time { return clock }
	var heard []events.PullRequestState
	done := make(chan struct{}, 8)
	tr.OnState = func(e events.PullRequestState) { heard = append(heard, e); done <- struct{}{} }
	ref := events.PaneRef{Machine: "box", Pane: "w1:p1"}
	tr.OnNamed(events.PullRequestNamed{Pane: ref, URL: "https://github.com/o/r/pull/7", Label: "PR #7"})
	<-done
	if len(heard) != 1 || heard[0].State != "OPEN" || heard[0].Was != "" || heard[0].URL != "https://github.com/o/r/pull/7" || heard[0].Pane != ref {
		t.Fatalf("heard %+v", heard)
	}

	clock = clock.Add(Every)
	tr.TouchPane("box", "w1:p1")
	waitFor(t, "the second read", func() bool { return len(host.Scripts()) == 2 })
	time.Sleep(20 * time.Millisecond)
	if len(heard) != 1 {
		t.Fatalf("an unchanged state was announced: %+v", heard)
	}
	tr.TouchPane("box", "w1:p1")
	tr.TouchPane("box", "w9:p9")
	time.Sleep(20 * time.Millisecond)
	if len(host.Scripts()) != 2 {
		t.Fatal("a fresh read was repeated, or an unknown pane read")
	}

	state = "MERGED"
	clock = clock.Add(Every)
	tr.TouchPane("box", "w1:p1")
	<-done
	if len(heard) != 2 || heard[1].State != "MERGED" || heard[1].Was != "OPEN" {
		t.Fatalf("heard %+v", heard)
	}
	if status, _ := tr.Status("box", "w1:p1"); status.State != "MERGED" {
		t.Fatalf("status = %+v", status)
	}
}
