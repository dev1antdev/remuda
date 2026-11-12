package mirror

import (
	"maps"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/limits"
	"github.com/dev1antdev/remuda/internal/prstatus"
	"github.com/dev1antdev/remuda/internal/transcript"
	"github.com/dev1antdev/remuda/internal/usage"
)

func TestCostIsWrittenAsThePageWritesIt(t *testing.T) {
	for usd, want := range map[float64]string{0.042: "$0.04", 4.2: "$4.20", 9.999: "$10.00", 42.46: "$42.5", 142.5: "$143", 1234: "$1234"} {
		if got := Cost(usd); got != want {
			t.Errorf("Cost(%v) = %q, want %q", usd, got, want)
		}
	}
}

func TestContextShare(t *testing.T) {
	if got := ContextShare(usage.Display{Context: 124_000, Window: 200_000}); got != "62%" {
		t.Errorf("62%%: %q", got)
	}
	if ContextShare(usage.Display{Context: 1000}) != "" || ContextShare(usage.Display{Window: 200_000}) != "" {
		t.Error("an unknown window or no reading yet says nothing")
	}
}

func TestPullRequestWords(t *testing.T) {
	link := transcript.Link{URL: "https://github.com/o/r/pull/15", Label: "PR #15"}
	for _, c := range []struct {
		status *prstatus.Status
		want   string
	}{
		{nil, "PR #15"},
		{&prstatus.Status{State: "OPEN", Checks: prstatus.Checks{Total: 2, Pass: 2}}, "PR #15 ✓"},
		{&prstatus.Status{State: "OPEN", Checks: prstatus.Checks{Total: 3, Pass: 1, Fail: 1, Pending: 1}}, "PR #15 ✗"},
		{&prstatus.Status{State: "OPEN", Checks: prstatus.Checks{Total: 2, Pass: 1, Pending: 1}}, "PR #15 …"},
		{&prstatus.Status{State: "OPEN", Review: "APPROVED", Checks: prstatus.Checks{Total: 1, Pass: 1}}, "PR #15 ✓ approved"},
		{&prstatus.Status{State: "OPEN", Mergeable: "CONFLICTING", Review: "APPROVED"}, "PR #15 conflicts"},
		{&prstatus.Status{State: "OPEN", Draft: true, Review: "CHANGES_REQUESTED"}, "PR #15 draft"},
		{&prstatus.Status{State: "OPEN", Review: "CHANGES_REQUESTED", Checks: prstatus.Checks{Total: 1, Fail: 1}}, "PR #15 ✗ changes requested"},
		{&prstatus.Status{State: "MERGED", Checks: prstatus.Checks{Total: 1, Fail: 1}}, "PR #15 merged"},
		{&prstatus.Status{State: "CLOSED"}, "PR #15 closed"},
	} {
		if got := PullRequest(link, c.status); got != c.want {
			t.Errorf("PullRequest(%+v) = %q, want %q", c.status, got, c.want)
		}
	}
}

func TestReviewWords(t *testing.T) {
	for _, c := range []struct {
		fresh int
		ready bool
		want  string
	}{{0, false, ""}, {1, false, "1 file to review"}, {3, false, "3 files to review"}, {3, true, "ready for review"}} {
		if got := Review(c.fresh, c.ready); got != c.want {
			t.Errorf("Review(%d, %v) = %q, want %q", c.fresh, c.ready, got, c.want)
		}
	}
}

func TestPaneFiguresSayWhatTheRosterSays(t *testing.T) {
	meter := usage.Display{Name: "Opus 4.5", Cost: 4.2, Context: 124_000, Window: 200_000}
	link := transcript.Link{URL: "https://github.com/o/r/pull/15", Label: "PR #15"}
	got := PaneFigures(PaneFacts{
		Status: herdr.StatusWorking, Usage: &meter, Link: &link, Task: 42,
		PR:   &prstatus.Status{State: "OPEN", Checks: prstatus.Checks{Total: 2, Pass: 2}},
		Note: "quiet for 25 min — working, but nothing written", Said: "an answer from before this turn",
	})
	want := map[string]string{TokenCost: "$4.20", TokenContext: "62%", TokenModel: "Opus 4.5", TokenPR: "PR #15 ✓", TokenTask: "#42", TokenAlert: "! stuck 25 min"}
	if !maps.Equal(got.Tokens, want) {
		t.Errorf("tokens %v, want %v (no last answer while the agent works)", got.Tokens, want)
	}
	if !maps.Equal(got.Labels, map[string]string{herdr.StatusWorking: "stuck 25 min"}) {
		t.Errorf("the note stands as the label of the state it is about: %v", got.Labels)
	}

	hit := time.Unix(1_800_000_000, 0)
	long := strings.Repeat("word ", 40)
	unknown := usage.Display{Name: "gpt-5.5", Unknown: true, Context: 10_000, Window: 400_000}
	got = PaneFigures(PaneFacts{Status: herdr.StatusIdle, Usage: &unknown, Said: long, Limit: &limits.PaneLimit{HitAt: &hit}})
	if _, priced := got.Tokens[TokenCost]; priced {
		t.Error("a model with no price shows no cost")
	}
	if got.Tokens[TokenAlert] != "! plan limit" || got.Labels[herdr.StatusIdle] != "plan limit" || got.Labels[herdr.StatusDone] != "plan limit" {
		t.Errorf("an agent stopped on its plan limit: %v %v", got.Tokens, got.Labels)
	}
	if said := got.Tokens[TokenSaid]; utf8.RuneCountInString(said) > herdr.MaxValueRunes || !strings.HasSuffix(said, "…") {
		t.Errorf("the last answer is cut to herdr's 80 characters at a word: %q", said)
	}
	if got := PaneFigures(PaneFacts{Status: herdr.StatusIdle}); !got.empty() {
		t.Errorf("an agent remuda knows nothing of carries nothing: %+v", got)
	}
}

func TestWorkspaceFigures(t *testing.T) {
	link := transcript.Link{Label: "MR !9"}
	got := WorkspaceFigures(WorkspaceFacts{Cost: 12.34, Link: &link, Fresh: 2})
	want := map[string]string{TokenCost: "$12.3", TokenPR: "MR !9", TokenReview: "2 files to review"}
	if !maps.Equal(got.Tokens, want) || len(got.Labels) != 0 {
		t.Errorf("workspace figures %+v, want %v", got, want)
	}
	if !WorkspaceFigures(WorkspaceFacts{}).empty() {
		t.Error("a workspace with nothing to say carries nothing")
	}
}
