package recap

import (
	"strings"
	"testing"
	"time"
)

func TestDigestCountsAndNamesTheAgents(t *testing.T) {
	since := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	now := since.Add(9 * time.Hour)
	done := Recap{Prompts: 2, Answers: 2, Cost: 1.5, Said: "Fixed the tests"}
	working := Recap{Prompts: 1, Answers: 0, Calls: 3, Cost: 0.4}
	items := []Item{
		{Machine: "local", Workspace: "remuda", State: "working", Recap: working},
		{Machine: "gog", Label: "gog", Workspace: "api", State: "blocked", Recap: Recap{Prompts: 1, Answers: 1, Cost: 2.3, Said: "Which database should the migration target, the staging one or the production one, and do you want a backup first?"}},
		{Machine: "local", Workspace: "web", State: "done", Recap: done},
		{Machine: "local", Workspace: "quiet", State: "idle", Recap: Recap{}},
		{Machine: "local", Workspace: "broken", Error: "unreadable", Recap: done},
	}
	title, body, empty := Digest(items, since, now)
	if empty {
		t.Fatal("a digest with work in it is empty")
	}
	if title != "Standup: 2 finished, 1 waiting, 1 working, $4.20" {
		t.Fatalf("title %q", title)
	}
	lines := strings.Split(body, "\n")
	if len(lines) != 3 {
		t.Fatalf("body has %d lines:\n%s", len(lines), body)
	}
	if !strings.HasPrefix(lines[0], "api on gog: Which database should the migration target") || !strings.HasSuffix(lines[0], "…") {
		t.Errorf("the waiting agent comes first, its answer cut: %q", lines[0])
	}
	if lines[1] != "web: Fixed the tests" {
		t.Errorf("the finished one next: %q", lines[1])
	}
	if lines[2] != "remuda: 1 prompt · 0 answers · $0.40" {
		t.Errorf("an agent without an answer shows its figures: %q", lines[2])
	}
}

func TestDigestCapsTheBodyAndIsEmptyWhenQuiet(t *testing.T) {
	now := time.Now()
	var items []Item
	for i := 0; i < 6; i++ {
		items = append(items, Item{Workspace: "w" + string(rune('a'+i)), State: "done", Recap: Recap{Answers: 1, Said: "ok"}})
	}
	_, body, _ := Digest(items, time.Time{}, now)
	lines := strings.Split(body, "\n")
	if len(lines) != DigestLines+1 || lines[DigestLines] != "+2 more" {
		t.Fatalf("body:\n%s", body)
	}
	title, body, empty := Digest([]Item{{Workspace: "w", State: "idle", Recap: Recap{}}}, time.Time{}, now)
	if !empty || body != "" || title != "Standup: nothing to report" {
		t.Fatalf("quiet digest: %q %q %v", title, body, empty)
	}
	title, _, empty = Digest([]Item{{Workspace: "w", State: "working", Recap: Recap{Prompts: 1, Calls: 2}}}, time.Time{}, now)
	if empty || title != "Standup: 1 working" {
		t.Fatalf("working without answers or cost: %q %v", title, empty)
	}
	title, _, empty = Digest([]Item{{Workspace: "w", State: "idle", Recap: Recap{Calls: 2}}}, time.Time{}, now)
	if empty || title != "Standup: 1 agent active" {
		t.Fatalf("calls alone: %q %v", title, empty)
	}
}
