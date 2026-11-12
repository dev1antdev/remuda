// Package scorecard answers "which model does my work best" from remuda's own records: over a period, per agent kind
// and model, the task runs (finished or not, what they cost, how long they took), the review verdicts given on their
// worktrees, the pull requests they opened and whether those merged, and the arena lanes the person kept. Nothing is
// asked of an agent; a share with too few runs behind it is counted, never turned into a percentage.
package scorecard

import (
	"sort"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/arena"
	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/tasks"
	"github.com/dev1antdev/remuda/internal/usage"
)

// MinSample is how many runs a share or a mean needs behind it to be a figure worth comparing: under five, one run
// more or less swings a share by twenty points or more, so the page says "2 of 3" and marks no best.
const MinSample = 5

// Share is a count out of a whole: finished of the runs, approved of the reviewed.
type Share struct {
	Count int      `json:"count"`
	Of    int      `json:"of"`
	Rate  *float64 `json:"rate,omitempty"` // Count / Of, once Of reaches MinSample
}

// Mean is an average over the runs that measured it.
type Mean struct {
	Value *float64 `json:"value,omitempty"` // nil with nothing measured
	N     int      `json:"n"`               // the runs behind it
	Few   bool     `json:"few,omitempty"`   // N under MinSample: shown, never compared
}

// Row is one agent kind and model over the period.
type Row struct {
	Agent           string    `json:"agent"`
	Model           string    `json:"model"`              // the meter's name for it, "Opus 4.5"; "" when none was read
	ModelID         string    `json:"model_id,omitempty"` // what the agent's --model takes for it, "" when not known
	Runs            int       `json:"runs"`
	Finished        Share     `json:"finished"`       // runs whose agent answered, of the runs
	FirstApproved   Share     `json:"first_approved"` // approved at the first review, of the work runs reviewed
	Rejected        int       `json:"rejected"`       // rejected at the first review, approved later or not
	Approved        int       `json:"approved"`       // approved at any review
	Merged          Share     `json:"merged"`         // pull requests merged, of the ones its runs opened
	Kept            Share     `json:"kept"`           // arena lanes kept, of its lanes in decided arenas
	CostPerRun      Mean      `json:"cost_per_run"`
	CostPerApproved Mean      `json:"cost_per_approved"` // what the reviewed runs cost, over the approved ones
	MinutesPerRun   Mean      `json:"minutes_per_run"`
	Last            time.Time `json:"last"` // its newest run or decided lane
	Try             *Try      `json:"try,omitempty"`
}

// Card is the scorecard of a period.
type Card struct {
	Since     time.Time `json:"since"`
	Days      int       `json:"days"`
	MinSample int       `json:"min_sample"`
	Rows      []Row     `json:"rows"`
	Verdicts  int       `json:"verdicts"` // verdicts given in the period
	Unplaced  int       `json:"unplaced"` // of them, the ones given on a folder no run of the period worked in
}

// Records are what the scorecard reads: the runs started in the period, the verdicts given and the arenas decided in it.
type Records struct {
	Runs     []tasks.RunRecord
	Verdicts []review.Verdict
	Lanes    []arena.Outcome
}

type rowKey struct{ agent, model string }

// tally sums one row while the records are read.
type tally struct {
	key                      rowKey
	runs, finished           int
	cost                     float64
	costed                   int
	took, timed              int
	reviewed, firstApproved  int
	rejected, approved       int
	reviewedCost             float64
	reviewedCosted           int
	prs, merged, lanes, kept int
	last                     time.Time
}

// Build scores every agent kind and model the records name from since on.
func Build(rec Records, since time.Time) Card {
	card := Card{Since: since, MinSample: MinSample, Rows: []Row{}}
	runs := runsFrom(rec.Runs, since)
	tallies := map[rowKey]*tally{}
	of := func(agent, model string) *tally {
		k := rowKey{agent, normalName(model)}
		t := tallies[k]
		if t == nil {
			t = &tally{key: k}
			tallies[k] = t
		}
		return t
	}
	for _, r := range runs {
		t := of(r.Agent, r.Model)
		t.runs++
		if r.Finished != "" {
			t.finished++
		}
		if r.Cost > 0 {
			t.cost += r.Cost
			t.costed++
		}
		if r.Took > 0 {
			t.took += r.Took
			t.timed++
		}
		t.last = later(t.last, r.started)
	}
	folders := workRuns(runs)
	card.Verdicts, card.Unplaced = placeVerdicts(folders, rec.Verdicts, since)
	for _, r := range runs {
		if r.Stage == 0 {
			scoreReviews(of(r.Agent, r.Model), r)
		}
	}
	for _, credit := range creditPRs(folders, runs) {
		t := of(credit.run.Agent, credit.run.Model)
		t.prs++
		if credit.merged {
			t.merged++
		}
	}
	scoreLanes(of, rec.Lanes, rec.Runs, since)
	for _, t := range tallies {
		card.Rows = append(card.Rows, t.row())
	}
	sort.SliceStable(card.Rows, func(i, j int) bool {
		a, b := card.Rows[i], card.Rows[j]
		if a.Runs+a.Kept.Of != b.Runs+b.Kept.Of {
			return a.Runs+a.Kept.Of > b.Runs+b.Kept.Of
		}
		if !a.Last.Equal(b.Last) {
			return a.Last.After(b.Last)
		}
		return a.Agent+"|"+a.Model < b.Agent+"|"+b.Model
	})
	suggest(card.Rows, runs)
	return card
}

// scoreReviews counts a work run's verdicts: the first decides approved-at-first-review or rejected before approval.
func scoreReviews(t *tally, r *run) {
	if len(r.verdicts) == 0 {
		return
	}
	t.reviewed++
	if r.Cost > 0 {
		t.reviewedCost += r.Cost
		t.reviewedCosted++
	}
	if r.verdicts[0] == review.VerdictApproved {
		t.firstApproved++
	} else {
		t.rejected++
	}
	for _, verdict := range r.verdicts {
		if verdict == review.VerdictApproved {
			t.approved++
			break
		}
	}
}

// scoreLanes counts the decided arenas' lanes of the period: a lane that was a task run goes by the model the run read.
func scoreLanes(of func(agent, model string) *tally, lanes []arena.Outcome, all []tasks.RunRecord, since time.Time) {
	runModel := map[int64]string{}
	for _, r := range all {
		runModel[r.ID] = r.Model
	}
	for _, lane := range lanes {
		if lane.At.Before(since) {
			continue
		}
		model := lane.Model
		if read := runModel[lane.RunID]; lane.RunID != 0 && read != "" {
			model = read
		}
		t := of(lane.Agent, model)
		t.lanes++
		if lane.Kept {
			t.kept++
		}
		t.last = later(t.last, lane.At)
	}
}

func (t *tally) row() Row {
	return Row{
		Agent: t.key.agent, Model: t.key.model, ModelID: cliModel(t.key.model),
		Runs:            t.runs,
		Finished:        share(t.finished, t.runs),
		FirstApproved:   share(t.firstApproved, t.reviewed),
		Rejected:        t.rejected,
		Approved:        t.approved,
		Merged:          share(t.merged, t.prs),
		Kept:            share(t.kept, t.lanes),
		CostPerRun:      mean(t.cost, t.costed),
		CostPerApproved: t.costPerApproved(),
		MinutesPerRun:   mean(float64(t.took)/60, t.timed),
		Last:            t.last,
	}
}

// costPerApproved is what the reviewed runs cost over the ones approved: the price of an approved result, the misses
// included; nothing while no reviewed run was priced or none was approved.
func (t *tally) costPerApproved() Mean {
	if t.reviewedCosted == 0 {
		return Mean{N: t.approved, Few: t.approved < MinSample}
	}
	return mean(t.reviewedCost, t.approved)
}

func share(count, of int) Share {
	s := Share{Count: count, Of: of}
	if of >= MinSample {
		rate := float64(count) / float64(of)
		s.Rate = &rate
	}
	return s
}

// mean is sum over the n runs that measured it; nothing when none did.
func mean(sum float64, n int) Mean {
	m := Mean{N: n, Few: n < MinSample}
	if n > 0 {
		value := sum / float64(n)
		m.Value = &value
	}
	return m
}

// normalName is a model as the rows name it: an id the price catalog knows by its name, anything else as it was read.
func normalName(model string) string {
	model = strings.TrimSpace(model)
	if m, ok := usage.Lookup(model); ok {
		return m.Name
	}
	return model
}

// cliModel is what an agent's --model takes for a row's model: the catalog's id for its name, an id as it was read
// (Codex names its models by their ids), "" for a name with spaces the catalog does not know.
func cliModel(name string) string {
	if m, ok := usage.ByName(name); ok {
		return m.ID
	}
	if name == "" || strings.ContainsAny(name, " \t") {
		return ""
	}
	return name
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
