package prstatus

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/shell"
)

const mixed = `{"isDraft":false,"mergeable":"MERGEABLE","reviewDecision":"APPROVED","state":"OPEN","statusCheckRollup":[` +
	`{"__typename":"CheckRun","conclusion":"SUCCESS","name":"build","status":"COMPLETED"},` +
	`{"__typename":"CheckRun","conclusion":"FAILURE","name":"test","status":"COMPLETED"},` +
	`{"__typename":"CheckRun","conclusion":"","name":"smoke","status":"IN_PROGRESS"},` +
	`{"__typename":"CheckRun","conclusion":"SKIPPED","name":"e2e","status":"COMPLETED"},` +
	`{"__typename":"StatusContext","context":"ci/lint","state":"SUCCESS"},` +
	`{"__typename":"StatusContext","context":"ci/deploy","state":"PENDING"}]}`

func TestParseCountsEveryKindOfCheck(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	status, err := Parse("warning: something gh prints first\n"+mixed, now)
	if err != nil {
		t.Fatal(err)
	}
	want := Checks{Total: 6, Pass: 2, Fail: 1, Pending: 2, Skipped: 1}
	if status.Checks != want || status.State != "OPEN" || status.Review != "APPROVED" || status.Mergeable != "MERGEABLE" || status.Draft || !status.At.Equal(now) {
		t.Fatalf("status = %+v", status)
	}
}

func TestParseKeepsGhsComplaint(t *testing.T) {
	for _, out := range []string{"", "sh: gh: command not found\n", "To get started with GitHub CLI, please run:  gh auth login\nAlternatively, populate the GH_TOKEN environment variable", "HTTP 401: Bad credentials (https://api.github.com/graphql)"} {
		if _, err := Parse(out, time.Now()); err == nil {
			t.Fatalf("parsed %q", out)
		} else if out != "" && !strings.HasPrefix(err.Error(), strings.TrimSpace(strings.SplitN(out, "\n", 2)[0])) {
			t.Fatalf("reason %q for %q", err, out)
		}
	}
}

func TestScriptQuotesTheURL(t *testing.T) {
	s := Script("https://github.com/o/r/pull/7")
	if !strings.Contains(s, "gh pr view 'https://github.com/o/r/pull/7' --json state,isDraft,mergeable,reviewDecision,statusCheckRollup") || !strings.HasPrefix(s, "PATH=") {
		t.Fatalf("script = %q", s)
	}
	if GitHub("https://gitlab.com/o/r/-/merge_requests/3") || !GitHub("https://github.com/o/r/pull/7") {
		t.Fatal("GitHub")
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("waited in vain for %s", what)
}

func TestTrackerReadsOnceAndAgainOnlyWhenDue(t *testing.T) {
	var reads atomic.Int32
	host := &shell.Fake{Name: "box", Answer: func(string) (string, error) { reads.Add(1); return mixed, nil }}
	clock := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	tr := New(func(id string) (shell.Shell, bool) { return host, id == "box" })
	tr.now = func() time.Time { return clock }
	ref := events.PaneRef{Machine: "box", Pane: "w1:p1"}

	tr.OnNamed(events.PullRequestNamed{Pane: ref, URL: "https://gitlab.com/o/r/-/merge_requests/3", Label: "MR !3"})
	tr.OnNamed(events.PullRequestNamed{Pane: ref, URL: "https://github.com/o/r/pull/7", Label: "PR #7"})
	waitFor(t, "the first read", func() bool { _, ok := tr.Status("box", "w1:p1"); return ok })
	if status, _ := tr.Status("box", "w1:p1"); status.Checks.Fail != 1 || status.Review != "APPROVED" {
		t.Fatalf("status = %+v", status)
	}
	if got := host.Scripts(); len(got) != 1 || !strings.Contains(got[0], "'https://github.com/o/r/pull/7'") {
		t.Fatalf("scripts = %q", got)
	}

	tr.OnNamed(events.PullRequestNamed{Pane: ref, URL: "https://github.com/o/r/pull/7", Label: "PR #7"})
	tr.Touch()
	time.Sleep(20 * time.Millisecond)
	if reads.Load() != 1 {
		t.Fatalf("read again before it was due: %d", reads.Load())
	}

	clock = clock.Add(Every)
	tr.Touch()
	waitFor(t, "the second read", func() bool { return reads.Load() == 2 })
	tr.Touch()
	time.Sleep(20 * time.Millisecond)
	if reads.Load() != 2 {
		t.Fatalf("a fresh read was repeated: %d", reads.Load())
	}

	tr.OnStatus(events.PaneStatus{Pane: ref, To: events.Gone})
	if _, ok := tr.Status("box", "w1:p1"); ok {
		t.Fatal("a closed pane keeps its request")
	}
	clock = clock.Add(Every)
	tr.Touch()
	time.Sleep(20 * time.Millisecond)
	if reads.Load() != 2 {
		t.Fatal("a forgotten request was read")
	}
}

func TestTrackerKeepsNoStatusWhenGhCannot(t *testing.T) {
	host := &shell.Fake{Name: "box", Answer: func(string) (string, error) { return "sh: gh: command not found\n", &shell.ExitError{Code: 127} }}
	tr := New(func(string) (shell.Shell, bool) { return host, true })
	ref := events.PaneRef{Machine: "box", Pane: "w1:p1"}
	tr.OnNamed(events.PullRequestNamed{Pane: ref, URL: "https://github.com/o/r/pull/7", Label: "PR #7"})
	waitFor(t, "the read", func() bool { return len(host.Scripts()) == 1 })
	time.Sleep(20 * time.Millisecond)
	if _, ok := tr.Status("box", "w1:p1"); ok {
		t.Fatal("a failed read left a status")
	}
	tr.mu.Lock()
	reason := tr.entries[ref].reason
	tr.mu.Unlock()
	if reason != "sh: gh: command not found" {
		t.Fatalf("reason = %q", reason)
	}

	away := New(func(string) (shell.Shell, bool) { return nil, false })
	away.OnNamed(events.PullRequestNamed{Pane: ref, URL: "https://github.com/o/r/pull/7"})
	waitFor(t, "the miss", func() bool {
		away.mu.Lock()
		defer away.mu.Unlock()
		return !away.entries[ref].reading
	})
	away.mu.Lock()
	defer away.mu.Unlock()
	if away.entries[ref].reason != "the machine is not connected" || away.entries[ref].status != nil {
		t.Fatalf("entry = %+v", away.entries[ref])
	}
}
