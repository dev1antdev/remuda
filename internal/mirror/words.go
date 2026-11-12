package mirror

import (
	"fmt"
	"math"
	"strings"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/limits"
	"github.com/dev1antdev/remuda/internal/prstatus"
	"github.com/dev1antdev/remuda/internal/said"
	"github.com/dev1antdev/remuda/internal/transcript"
	"github.com/dev1antdev/remuda/internal/usage"
	"github.com/dev1antdev/remuda/internal/watchdog"
)

// The tokens remuda writes, named so a person's herdr config can tell them from another reporter's.
const (
	TokenCost    = "remuda_cost"    // "$4.20": a pane's conversation at API prices, a workspace's agents summed
	TokenContext = "remuda_context" // "62%": how full the context window is
	TokenModel   = "remuda_model"   // "Opus 4.5"
	TokenPR      = "remuda_pr"      // "PR #15 ✓": the request the agent named and how its checks stand
	TokenTask    = "remuda_task"    // "#42": the task the agent's run is for
	TokenAlert   = "remuda_alert"   // "! stuck 25 min": the watchdog's note, or the plan limit it stopped on
	TokenSaid    = "remuda_said"    // the first line of what the agent answered last
	TokenReview  = "remuda_review"  // "3 to review": a workspace's files changed since the last look
)

// PaneTokens are every name remuda may write on a pane, WorkspaceTokens on a workspace.
var (
	PaneTokens      = []string{TokenCost, TokenContext, TokenModel, TokenPR, TokenTask, TokenAlert, TokenSaid}
	WorkspaceTokens = []string{TokenCost, TokenPR, TokenReview, TokenBy}
)

// Figures are the display values of one pane or workspace: tokens by name and, for a pane, a label per herdr state.
type Figures struct {
	Tokens map[string]string
	Labels map[string]string
}

func (f Figures) empty() bool { return len(f.Tokens) == 0 && len(f.Labels) == 0 }

// PaneFacts are what remuda knows of one agent pane, gathered from its services.
type PaneFacts struct {
	Status string // herdr's agent status, the state a label stands for
	Usage  *usage.Display
	Said   string
	Link   *transcript.Link
	PR     *prstatus.Status
	Task   int64
	Note   string // the watchdog's
	Limit  *limits.PaneLimit
}

// PaneFigures turns a pane's facts into the values herdr shows: a token each for what is known, and the watchdog's
// note or the plan limit as the label of the state it stands in, so `state_text` says it too.
func PaneFigures(f PaneFacts) Figures {
	tokens := map[string]string{}
	put := func(name, value string) {
		if value = herdr.DisplayText(value, herdr.MaxValueRunes); value != "" {
			tokens[name] = value
		}
	}
	if f.Usage != nil {
		if !f.Usage.Unknown && f.Usage.Cost > 0 {
			put(TokenCost, Cost(f.Usage.Cost))
		}
		put(TokenContext, ContextShare(*f.Usage))
		put(TokenModel, f.Usage.Name)
	}
	if f.Link != nil {
		put(TokenPR, PullRequest(*f.Link, f.PR))
	}
	if f.Task > 0 {
		put(TokenTask, fmt.Sprintf("#%d", f.Task))
	}
	labels := map[string]string{}
	switch {
	case f.Note != "":
		brief := watchdog.Brief(f.Note)
		put(TokenAlert, "! "+brief)
		if f.Status != "" {
			labels[f.Status] = herdr.DisplayText(brief, herdr.MaxValueRunes)
		}
	case f.Limit != nil && f.Limit.HitAt != nil:
		put(TokenAlert, "! "+LimitWords(f.Limit))
		labels[herdr.StatusIdle] = LimitWords(f.Limit)
		labels[herdr.StatusDone] = LimitWords(f.Limit)
	}
	if f.Status != herdr.StatusWorking {
		put(TokenSaid, said.Cut(f.Said, herdr.MaxValueRunes-1))
	}
	return Figures{Tokens: tokens, Labels: labels}
}

// WorkspaceFacts are a workspace's own: its agents' costs and requests, and the review of its folder.
type WorkspaceFacts struct {
	Cost  float64
	Link  *transcript.Link
	PR    *prstatus.Status
	Fresh int  // files changed since the reviewer last looked
	Ready bool // rejected at the last look, and the agent says it is ready again
}

// WorkspaceFigures are a workspace's tokens; workspaces take no labels.
func WorkspaceFigures(f WorkspaceFacts) Figures {
	tokens := map[string]string{}
	if f.Cost > 0 {
		tokens[TokenCost] = Cost(f.Cost)
	}
	if f.Link != nil {
		tokens[TokenPR] = herdr.DisplayText(PullRequest(*f.Link, f.PR), herdr.MaxValueRunes)
	}
	if review := Review(f.Fresh, f.Ready); review != "" {
		tokens[TokenReview] = review
	}
	return Figures{Tokens: tokens}
}

// Cost is dollars as the page's usage pill writes them: two decimals under $10, one under $100, none above.
func Cost(usd float64) string {
	switch {
	case usd >= 100:
		return fmt.Sprintf("$%d", int(math.Round(usd)))
	case usd >= 10:
		return fmt.Sprintf("$%.1f", usd)
	}
	return fmt.Sprintf("$%.2f", usd)
}

// ContextShare is how full the context window is, "62%"; "" while the window or the reading is unknown.
func ContextShare(d usage.Display) string {
	if d.Window <= 0 || d.Context <= 0 {
		return ""
	}
	return fmt.Sprintf("%d%%", int(math.Round(100*float64(d.Context)/float64(d.Window))))
}

// PullRequest is the request's label and how it stands: merged or closed, else a mark for its checks — ✗ one failed,
// … some still run, ✓ every one passed — and the review's word when it stops the merge.
func PullRequest(link transcript.Link, status *prstatus.Status) string {
	label := link.Label
	if status == nil {
		return label
	}
	switch status.State {
	case "MERGED":
		return label + " merged"
	case "CLOSED":
		return label + " closed"
	}
	parts := []string{label}
	checks := status.Checks
	switch {
	case checks.Fail > 0:
		parts = append(parts, "✗")
	case checks.Pending > 0:
		parts = append(parts, "…")
	case checks.Pass > 0:
		parts = append(parts, "✓")
	}
	switch {
	case status.Draft:
		parts = append(parts, "draft")
	case status.Mergeable == "CONFLICTING":
		parts = append(parts, "conflicts")
	case status.Review == "CHANGES_REQUESTED":
		parts = append(parts, "changes requested")
	case status.Review == "APPROVED":
		parts = append(parts, "approved")
	}
	return strings.Join(parts, " ")
}

// LimitWords is an agent stopped on its plan limit, and whether the person booked the continue.
func LimitWords(limit *limits.PaneLimit) string {
	if limit.ContinueAt != nil {
		return "plan limit · continue booked"
	}
	return "plan limit"
}

// Review is a workspace's review in two or three words: ready again first, else the files to look at.
func Review(fresh int, ready bool) string {
	switch {
	case ready:
		return "ready for review"
	case fresh == 1:
		return "1 file to review"
	case fresh > 1:
		return fmt.Sprintf("%d files to review", fresh)
	}
	return ""
}
