package prstatus

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/shell"
)

func TestTheProbeReadsTheStatusAndTheETag(t *testing.T) {
	code, etag := ParseProbe("HTTP/2.0 304 Not Modified\r\nEtag: W/\"abc\"\r\nX-Ratelimit-Remaining: 4999\r\n")
	if code != 304 || etag != `W/"abc"` {
		t.Fatalf("code %d etag %q", code, etag)
	}
	if code, _ := ParseProbe("gh: Not Found (HTTP 404)\n"); code != 0 {
		t.Fatalf("gh's own line read as a status: %d", code)
	}
	s := ProbeScript("https://github.com/o/r/pull/7", `"abc"`)
	if !strings.Contains(s, `gh api -i -H 'If-None-Match: "abc"' 'repos/o/r/pulls/7'`) {
		t.Fatalf("script = %q", s)
	}
	if ProbeScript("https://gitlab.com/o/r/-/merge_requests/3", "") != "" {
		t.Fatal("a merge request was probed")
	}
}

func TestAProbeThatSaysNothingMovedKeepsTheReadWhileItStands(t *testing.T) {
	var mu sync.Mutex
	answer := "HTTP/2.0 200 OK\r\nETag: \"one\"\r\n"
	view := mixed
	host := &shell.Fake{Name: "box", Answer: func(s string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		if strings.Contains(s, "gh api -i") {
			return answer, nil
		}
		return view, nil
	}}
	clock := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	tr := New(func(string) (shell.Shell, bool) { return host, true })
	tr.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return clock }
	ref := events.PaneRef{Machine: "box", Pane: "w1:p1"}
	tr.OnNamed(events.PullRequestNamed{Pane: ref, URL: "https://github.com/o/r/pull/7", Label: "PR #7"})
	waitFor(t, "the first read", func() bool { return len(views(host)) == 1 })
	waitFor(t, "the read kept", func() bool { tr.mu.Lock(); defer tr.mu.Unlock(); return !tr.entries[ref].reading })
	if s := host.Scripts(); strings.Contains(s[0], "If-None-Match") {
		t.Fatal("the first probe named an ETag it never had")
	}

	// the mixed answer has a check still running: its read stands two minutes
	advance := func(d time.Duration, answering string) {
		mu.Lock()
		clock = clock.Add(d)
		answer = answering
		mu.Unlock()
	}
	advance(Every-time.Second, "HTTP/2.0 304 Not Modified\r\n")
	tr.TouchPane("box", "w1:p1")
	time.Sleep(20 * time.Millisecond)
	advance(time.Second, "HTTP/2.0 304 Not Modified\r\n")
	tr.TouchPane("box", "w1:p1")
	waitFor(t, "the probe", func() bool { return len(host.Scripts()) >= 4 })
	if got := host.Scripts(); !strings.Contains(got[2], `If-None-Match: "one"`) {
		t.Fatalf("the probe did not carry the ETag: %q", got[2])
	}
	if len(views(host)) != 2 {
		t.Fatalf("a running check's read stood past two minutes: %d full reads", len(views(host)))
	}

	// with every check done, an open request's read stands ten minutes
	mu.Lock()
	view = strings.NewReplacer(`"conclusion":"","name":"smoke","status":"IN_PROGRESS"`, `"conclusion":"SUCCESS","name":"smoke","status":"COMPLETED"`, `"PENDING"`, `"SUCCESS"`).Replace(mixed)
	mu.Unlock()
	advance(Every, "HTTP/2.0 200 OK\r\nETag: \"two\"\r\n")
	tr.TouchPane("box", "w1:p1")
	waitFor(t, "a moved request read again", func() bool { return len(views(host)) == 3 })
	waitFor(t, "the read kept", func() bool { tr.mu.Lock(); defer tr.mu.Unlock(); return !tr.entries[ref].reading })
	for i := 0; i < 3; i++ {
		advance(Every, "HTTP/2.0 304 Not Modified\r\n")
		tr.TouchPane("box", "w1:p1")
		waitFor(t, "the probe kept", func() bool { tr.mu.Lock(); defer tr.mu.Unlock(); return !tr.entries[ref].reading })
	}
	if len(views(host)) != 3 {
		t.Fatalf("an unchanged open request was read in full again: %d", len(views(host)))
	}
	if got := host.Scripts(); !strings.Contains(got[len(got)-1], `If-None-Match: "two"`) {
		t.Fatalf("the new ETag was not kept: %q", got[len(got)-1])
	}
	advance(4*Every, "HTTP/2.0 304 Not Modified\r\n")
	tr.TouchPane("box", "w1:p1")
	waitFor(t, "the stand run out", func() bool { return len(views(host)) == 4 })
}
