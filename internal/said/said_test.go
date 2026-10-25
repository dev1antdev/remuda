package said

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/transcript"
)

func TestFirstLineStripsTheMarks(t *testing.T) {
	cases := map[string]string{
		"## Done\n\nAdded tests.":                               "Done",
		"\n\n- **Fixed** the `parser` in [x](http://a)\n- more": "Fixed the parser in x",
		"> 1. [ ] *note* ~~old~~ <b>new</b>":                    "note old new",
		"```go\nfunc x() {}\n```\nThe code above.":              "The code above.",
		"---\n\n   \nFirst   real\tline":                        "First real line",
		"":                                                      "",
		"snake_case_name stays":                                 "snake_case_name stays",
	}
	for in, want := range cases {
		if got := FirstLine(in); got != want {
			t.Errorf("FirstLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFirstLineCutsAtAWord(t *testing.T) {
	long := strings.Repeat("word ", 40)
	got := FirstLine(long)
	if !strings.HasSuffix(got, "…") || len([]rune(got)) > Max+1 || strings.Contains(got, " …") {
		t.Fatalf("cut: %q", got)
	}
	if got := FirstLine(strings.Repeat("x", 200)); len([]rune(got)) != Max+1 {
		t.Fatalf("a word with no space is cut flat: %q", got)
	}
	if got := FirstLine("Все тесты прошли, можно мерджить"); got != "Все тесты прошли, можно мерджить" {
		t.Fatalf("short stays: %q", got)
	}
}

type fake struct {
	mu       sync.Mutex
	versions map[string]string
	answers  map[string]string
	links    map[string]string
	reads    int
}

func (f *fake) link(m, p string) (transcript.Link, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	url, ok := f.links[m+"|"+p]
	return transcript.Link{URL: url, Label: "PR #1"}, ok
}

func (f *fake) version(m, p string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.versions[m+"|"+p]
	return v, ok
}

func (f *fake) answer(m, p string) (string, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	return f.answers[m+"|"+p], "id"
}

func (f *fake) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads
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

func status(to string) events.PaneStatus {
	return events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "w1:p1"}, Kind: "claude", From: herdr.StatusWorking, To: to}
}

func TestTrackerReadsOnAFinishAndClearsOnWork(t *testing.T) {
	f := &fake{versions: map[string]string{"local|w1:p1": "10-1"}, answers: map[string]string{"local|w1:p1": "**Merged** everything.\nDetails follow."}}
	tr := New(Inputs{Version: f.version, Answer: f.answer}, nil)
	tr.OnStatus(status(herdr.StatusDone))
	waitFor(t, "the line", func() bool { return tr.Line("local", "w1:p1") == "Merged everything." })

	tr.OnStatus(status(herdr.StatusIdle))
	time.Sleep(20 * time.Millisecond)
	if f.count() != 1 {
		t.Fatalf("an unchanged file was read again: %d reads", f.count())
	}

	tr.OnStatus(status(herdr.StatusWorking))
	if tr.Line("local", "w1:p1") != "" {
		t.Fatal("a working agent still says its old line")
	}
	f.mu.Lock()
	f.versions["local|w1:p1"], f.answers["local|w1:p1"] = "20-2", "Released."
	f.mu.Unlock()
	tr.OnStatus(status(herdr.StatusDone))
	waitFor(t, "the new line", func() bool { return tr.Line("local", "w1:p1") == "Released." })

	tr.OnStatus(status(events.Gone))
	if tr.Line("local", "w1:p1") != "" {
		t.Fatal("a closed pane keeps its line")
	}
}

func TestTrackerKeepsTheLinkWhileTheAgentWorks(t *testing.T) {
	f := &fake{versions: map[string]string{"local|w1:p1": "10-1"}, answers: map[string]string{"local|w1:p1": "Opened the PR."}, links: map[string]string{"local|w1:p1": "https://github.com/o/r/pull/1"}}
	bus := events.NewBus()
	var named []events.PullRequestNamed
	var namedMu sync.Mutex
	events.Subscribe(bus, func(e events.PullRequestNamed) {
		namedMu.Lock()
		named = append(named, e)
		namedMu.Unlock()
	})
	tr := New(Inputs{Version: f.version, Answer: f.answer, Link: f.link}, bus)
	tr.OnStatus(status(herdr.StatusDone))
	waitFor(t, "the link", func() bool { _, ok := tr.Link("local", "w1:p1"); return ok })
	if link, _ := tr.Link("local", "w1:p1"); link.URL != "https://github.com/o/r/pull/1" || link.Label != "PR #1" {
		t.Fatalf("link = %+v", link)
	}
	namedMu.Lock()
	if len(named) != 1 || named[0].URL != "https://github.com/o/r/pull/1" || named[0].Label != "PR #1" || named[0].Pane != (events.PaneRef{Machine: "local", Pane: "w1:p1"}) {
		t.Fatalf("named = %+v", named)
	}
	namedMu.Unlock()

	tr.OnStatus(status(herdr.StatusWorking))
	if _, ok := tr.Link("local", "w1:p1"); !ok || tr.Line("local", "w1:p1") != "" {
		t.Fatal("working clears the line and keeps the link")
	}
	f.mu.Lock()
	f.versions["local|w1:p1"] = "11-1"
	f.mu.Unlock()
	tr.OnStatus(status(herdr.StatusDone))
	waitFor(t, "the second read", func() bool { return f.count() == 2 })
	namedMu.Lock()
	if len(named) != 1 {
		t.Fatalf("the same request was announced again: %+v", named)
	}
	namedMu.Unlock()

	tr.OnStatus(status(herdr.StatusWorking))
	f.mu.Lock()
	f.versions["local|w1:p1"] = "20-2"
	delete(f.links, "local|w1:p1")
	f.mu.Unlock()
	tr.OnStatus(status(herdr.StatusDone))
	waitFor(t, "the line again", func() bool { return tr.Line("local", "w1:p1") == "Opened the PR." })
	if _, ok := tr.Link("local", "w1:p1"); ok {
		t.Fatal("a read that finds no request drops the old one")
	}

	plain := New(Inputs{Version: f.version, Answer: f.answer}, nil)
	plain.OnStatus(status(herdr.StatusDone))
	waitFor(t, "the plain line", func() bool { return plain.Line("local", "w1:p1") != "" })
	if _, ok := plain.Link("local", "w1:p1"); ok {
		t.Fatal("no Link input, no link")
	}
}

func TestTrackerSeedsFromASnapshotOnce(t *testing.T) {
	f := &fake{versions: map[string]string{"local|w1:p1": "1-1"}, answers: map[string]string{"local|w1:p1": "Seeded."}}
	tr := New(Inputs{Version: f.version, Answer: f.answer}, nil)
	snap := &herdr.Snapshot{Panes: []herdr.PaneInfo{
		{PaneID: "w1:p1", Agent: "claude", AgentStatus: herdr.StatusDone},
		{PaneID: "w1:p2", Agent: "claude", AgentStatus: herdr.StatusWorking},
		{PaneID: "w1:p3", Agent: "", AgentStatus: herdr.StatusIdle},
		{PaneID: "w1:p4", Agent: "claude", AgentStatus: herdr.StatusIdle}, // no session file
	}}
	tr.Observe("local", snap)
	waitFor(t, "the seeded line", func() bool { return tr.Line("local", "w1:p1") == "Seeded." })
	tr.Observe("local", snap)
	tr.Observe("local", snap)
	time.Sleep(20 * time.Millisecond)
	if f.count() != 1 {
		t.Fatalf("seeding again re-read the file: %d reads", f.count())
	}
	if tr.Line("local", "w1:p2") != "" || tr.Line("local", "w1:p3") != "" || tr.Line("local", "w1:p4") != "" {
		t.Fatal("a working, shell or fileless pane got a line")
	}
	tr.Forget("local")
	if tr.Line("local", "w1:p1") != "" {
		t.Fatal("a forgotten machine keeps its lines")
	}
}
