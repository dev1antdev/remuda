package scorecard

import (
	"math"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/arena"
	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/tasks"
)

var day = time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)

// at is a moment of the test's day, hours after its midnight.
func at(hours float64) time.Time { return day.Add(time.Duration(hours * float64(time.Hour))) }

// runOf is a finished work run of a task on its own worktree.
func runOf(id int64, agent, model string, started float64, dir string) tasks.RunRecord {
	return tasks.RunRecord{
		Run:   tasks.Run{ID: id, TaskID: id, Machine: "studio", Dir: dir, Agent: agent, Model: model, State: tasks.RunFinished, Started: at(started).Format(time.RFC3339), Finished: at(started + 1).Format(time.RFC3339)},
		Title: "Task", ProjectID: 1,
	}
}

func verdict(dir, word string, hours float64) review.Verdict {
	return review.Verdict{Machine: "studio", Dir: dir, Verdict: word, At: at(hours)}
}

func rowOf(t *testing.T, card Card, agent, model string) Row {
	t.Helper()
	for _, row := range card.Rows {
		if row.Agent == agent && row.Model == model {
			return row
		}
	}
	t.Fatalf("no row for %s · %s in %+v", agent, model, card.Rows)
	return Row{}
}

func TestAnEmptyStoreScoresNothing(t *testing.T) {
	card := Build(Records{}, day)
	if card.Rows == nil || len(card.Rows) != 0 || card.Verdicts != 0 || card.MinSample != MinSample {
		t.Fatalf("an empty card: %+v", card)
	}
	if empty, err := (&Service{Now: func() time.Time { return day }}).Card(30, time.UTC); err != nil || len(empty.Rows) != 0 || empty.Days != 30 {
		t.Fatalf("a service with no stores: %+v %v", empty, err)
	}
}

func TestThePeriodStartsAtLocalMidnight(t *testing.T) {
	berlin, _ := time.LoadLocation("Europe/Berlin")
	now := time.Date(2026, 11, 12, 0, 30, 0, 0, berlin)
	if got := Since(7, now); !got.Equal(time.Date(2026, 11, 6, 0, 0, 0, 0, berlin)) {
		t.Fatalf("since %v", got)
	}
	if got := Since(1, now); !got.Equal(time.Date(2026, 11, 12, 0, 0, 0, 0, berlin)) {
		t.Fatalf("today since %v", got)
	}
}

func TestEverythingBeforeThePeriodIsLeftOut(t *testing.T) {
	old := runOf(1, "claude", "Opus 4.5", -30, "/w/old")
	fresh := runOf(2, "claude", "Opus 4.5", 2, "/w/fresh")
	card := Build(Records{
		Runs:     []tasks.RunRecord{old, fresh},
		Verdicts: []review.Verdict{verdict("/w/old", review.VerdictApproved, -20), verdict("/w/fresh", review.VerdictApproved, 5)},
		Lanes:    []arena.Outcome{{Arena: 1, Lane: 1, Agent: "codex", Model: "gpt-6.1-sol", At: at(-10)}},
	}, day)
	if len(card.Rows) != 1 {
		t.Fatalf("rows: %+v", card.Rows)
	}
	row := card.Rows[0]
	if row.Runs != 1 || row.FirstApproved.Count != 1 || row.FirstApproved.Of != 1 || card.Verdicts != 1 {
		t.Fatalf("the period's row: %+v (verdicts %d)", row, card.Verdicts)
	}
}

func TestAVerdictBelongsToTheLastRunStartedOnItsWorktree(t *testing.T) {
	first := runOf(1, "claude", "Opus 4.5", 1, "/w/x")
	second := runOf(2, "codex", "gpt-6.1-sol", 4, "/w/x/") // the same folder, spelled with a slash
	elsewhere := runOf(3, "claude", "Opus 4.5", 1, "/w/y")
	card := Build(Records{
		Runs: []tasks.RunRecord{first, second, elsewhere},
		Verdicts: []review.Verdict{
			verdict("/w/x", review.VerdictChanges, 2),                                   // the first run's
			verdict("/w/x", review.VerdictApproved, 6),                                  // the second's
			verdict("/w/z", review.VerdictApproved, 6),                                  // no run worked there: the person's own folder
			{Machine: "other", Dir: "/w/y", Verdict: review.VerdictApproved, At: at(3)}, // the same path on another machine
			verdict("/w/y", review.VerdictApproved, 0.5),                                // before the run started
		},
	}, day)
	opus := rowOf(t, card, "claude", "Opus 4.5")
	codex := rowOf(t, card, "codex", "gpt-6.1-sol")
	if opus.FirstApproved.Count != 0 || opus.FirstApproved.Of != 1 || opus.Rejected != 1 || opus.Approved != 0 {
		t.Fatalf("opus: %+v", opus)
	}
	if codex.FirstApproved.Count != 1 || codex.FirstApproved.Of != 1 || codex.Rejected != 0 {
		t.Fatalf("codex: %+v", codex)
	}
	if card.Verdicts != 5 || card.Unplaced != 3 {
		t.Fatalf("verdicts %d, unplaced %d", card.Verdicts, card.Unplaced)
	}
}

func TestTheFirstVerdictDecidesAndALaterApprovalCountsApart(t *testing.T) {
	r := runOf(1, "claude", "Opus 4.5", 1, "/w/x")
	card := Build(Records{Runs: []tasks.RunRecord{r}, Verdicts: []review.Verdict{
		verdict("/w/x", review.VerdictApproved, 5), // given out of order: the earlier one decides
		verdict("/w/x", review.VerdictChanges, 3),
	}}, day)
	row := card.Rows[0]
	if row.FirstApproved.Count != 0 || row.Rejected != 1 || row.Approved != 1 {
		t.Fatalf("rejected, then approved: %+v", row)
	}
}

func TestAPipelinesWorkIsItsFirstAgents(t *testing.T) {
	work := runOf(1, "claude", "Opus 4.5", 1, "/w/x")
	tests := runOf(2, "codex", "gpt-6.1-sol", 3, "/w/x")
	tests.TaskID, tests.Stage = 1, 1
	request := runOf(3, "claude", "Sonnet 4.5", 4, "/w/x")
	request.TaskID, request.Stage, request.PRURL, request.PRLabel = 1, 2, "https://github.com/o/r/pull/9", "PR #9"
	again := runOf(4, "claude", "Sonnet 4.5", 4.5, "/w/x")
	again.TaskID, again.Stage, again.PRURL, again.Merged = 1, 2, "https://github.com/o/r/pull/9", at(9).Format(time.RFC3339)
	card := Build(Records{Runs: []tasks.RunRecord{work, tests, request, again}, Verdicts: []review.Verdict{verdict("/w/x", review.VerdictApproved, 6)}}, day)
	opus := rowOf(t, card, "claude", "Opus 4.5")
	if opus.FirstApproved.Count != 1 || opus.Merged.Count != 1 || opus.Merged.Of != 1 {
		t.Fatalf("the first agent's work: %+v", opus)
	}
	sonnet := rowOf(t, card, "claude", "Sonnet 4.5")
	if sonnet.Runs != 2 || sonnet.FirstApproved.Of != 0 || sonnet.Merged.Of != 0 {
		t.Fatalf("a request stage took the work's credit: %+v", sonnet)
	}
	if codex := rowOf(t, card, "codex", "gpt-6.1-sol"); codex.Runs != 1 || codex.FirstApproved.Of != 0 {
		t.Fatalf("the tests stage took a verdict: %+v", codex)
	}
}

// A request stage of the period carrying on work begun before it credits the request to nobody: the work, its verdicts
// and its request belong to the period it began in, never to the stage's own model.
func TestARequestOnWorkBegunBeforeThePeriodIsNobodys(t *testing.T) {
	work := runOf(1, "claude", "Opus 4.5", -5, "/w/x")
	request := runOf(2, "claude", "Sonnet 4.5", 2, "/w/x")
	request.TaskID, request.Stage, request.PRURL, request.Merged = 1, 2, "https://github.com/o/r/pull/9", at(4).Format(time.RFC3339)
	card := Build(Records{Runs: []tasks.RunRecord{work, request}, Verdicts: []review.Verdict{verdict("/w/x", review.VerdictApproved, 3)}}, day)
	sonnet := rowOf(t, card, "claude", "Sonnet 4.5")
	if sonnet.Runs != 1 || sonnet.Merged.Of != 0 || sonnet.FirstApproved.Of != 0 || card.Unplaced != 1 || len(card.Rows) != 1 {
		t.Fatalf("the stage took the earlier work's request: %+v (rows %d, unplaced %d)", sonnet, len(card.Rows), card.Unplaced)
	}
}

func TestAPullRequestIsCountedOnceAndMergedWhenAnyRunSawIt(t *testing.T) {
	a := runOf(1, "claude", "Opus 4.5", 1, "/w/a")
	a.PRURL = "https://github.com/o/r/pull/1"
	b := runOf(2, "claude", "Opus 4.5", 2, "/w/b")
	b.PRURL, b.Merged = "https://github.com/o/r/pull/2", at(5).Format(time.RFC3339)
	c := runOf(3, "claude", "Opus 4.5", 3, "/w/c")
	card := Build(Records{Runs: []tasks.RunRecord{a, b, c}}, day)
	if row := card.Rows[0]; row.Merged.Count != 1 || row.Merged.Of != 2 {
		t.Fatalf("requests: %+v", row.Merged)
	}
}

func TestASharesPercentageWaitsForEnoughRuns(t *testing.T) {
	var runs []tasks.RunRecord
	var verdicts []review.Verdict
	for i := range MinSample - 1 {
		dir := "/w/" + string(rune('a'+i))
		runs = append(runs, runOf(int64(i+1), "claude", "Opus 4.5", float64(i), dir))
		verdicts = append(verdicts, verdict(dir, review.VerdictApproved, float64(i)+0.5))
	}
	few := Build(Records{Runs: runs, Verdicts: verdicts}, day).Rows[0]
	if few.FirstApproved.Rate != nil || few.FirstApproved.Count != MinSample-1 || few.Finished.Rate != nil || !few.MinutesPerRun.Few {
		t.Fatalf("a percentage from %d runs: %+v", MinSample-1, few)
	}
	runs = append(runs, runOf(9, "claude", "Opus 4.5", 8, "/w/z"))
	verdicts = append(verdicts, verdict("/w/z", review.VerdictChanges, 9))
	enough := Build(Records{Runs: runs, Verdicts: verdicts}, day).Rows[0]
	if enough.FirstApproved.Rate == nil || *enough.FirstApproved.Rate != 0.8 || enough.Finished.Rate == nil || *enough.Finished.Rate != 1 {
		t.Fatalf("no percentage from %d runs: %+v", MinSample, enough.FirstApproved)
	}
}

func TestCostAndTimePerRun(t *testing.T) {
	a := runOf(1, "claude", "Opus 4.5", 1, "/w/a")
	a.Cost, a.Took = 2, 600
	b := runOf(2, "claude", "Opus 4.5", 2, "/w/b")
	b.Cost, b.Took = 4, 1800
	live := runOf(3, "claude", "Opus 4.5", 3, "/w/c") // still at work: no cost read yet, no time
	live.Finished, live.State = "", tasks.RunRunning
	codex := runOf(4, "codex", "gpt-6.1-sol", 3, "/w/d") // no price here
	codex.Took = 120
	card := Build(Records{Runs: []tasks.RunRecord{a, b, live, codex}, Verdicts: []review.Verdict{
		verdict("/w/a", review.VerdictApproved, 4),
		verdict("/w/b", review.VerdictChanges, 4),
	}}, day)
	opus := rowOf(t, card, "claude", "Opus 4.5")
	if opus.Runs != 3 || opus.Finished.Count != 2 || *opus.CostPerRun.Value != 3 || opus.CostPerRun.N != 2 || !opus.CostPerRun.Few {
		t.Fatalf("cost per run: %+v", opus)
	}
	if *opus.CostPerApproved.Value != 6 || opus.CostPerApproved.N != 1 {
		t.Fatalf("cost per approved (both reviewed runs over the one approved): %+v", opus.CostPerApproved)
	}
	if *opus.MinutesPerRun.Value != 20 || opus.MinutesPerRun.N != 2 {
		t.Fatalf("minutes per run: %+v", opus.MinutesPerRun)
	}
	gpt := rowOf(t, card, "codex", "gpt-6.1-sol")
	if gpt.CostPerRun.Value != nil || gpt.CostPerApproved.Value != nil || *gpt.MinutesPerRun.Value != 2 {
		t.Fatalf("an unpriced model: %+v", gpt)
	}
}

// A run whose agent has not answered yet may still: it counts among the runs, but neither against the finished share
// nor in the means with what it has cost so far.
func TestARunStillAtWorkCountsInRunsAlone(t *testing.T) {
	var runs []tasks.RunRecord
	for i := range MinSample {
		r := runOf(int64(i+1), "claude", "Opus 4.5", float64(i), "/w/"+string(rune('a'+i)))
		r.Cost = 2
		runs = append(runs, r)
	}
	starting := runOf(8, "claude", "Opus 4.5", 7, "/w/s")
	starting.Finished, starting.State = "", tasks.RunStarting
	working := runOf(9, "claude", "Opus 4.5", 8, "/w/w")
	working.Finished, working.State, working.Cost = "", tasks.RunRunning, 0.1
	gone := runOf(10, "claude", "Opus 4.5", 9, "/w/g") // closed before it answered: an outcome, a run that did not finish
	gone.Finished, gone.State, gone.Cost = "", tasks.RunGone, 1
	row := Build(Records{Runs: append(runs, starting, working, gone)}, day).Rows[0]
	if row.Runs != 8 || row.UnderWay != 2 || row.Finished.Count != 5 || row.Finished.Of != 6 {
		t.Fatalf("the runs still at work weighed on the finished share: %+v", row)
	}
	if row.CostPerRun.N != 6 || math.Abs(*row.CostPerRun.Value-11.0/6) > 1e-9 {
		t.Fatalf("a run's cost so far went into the mean: %+v", row.CostPerRun)
	}
}

func TestArenaLanesKeptOfTheLanesRun(t *testing.T) {
	asked := runOf(7, "claude", "Opus 4.5", 1, "/w/lane")
	card := Build(Records{
		Runs: []tasks.RunRecord{asked},
		Lanes: []arena.Outcome{
			{Arena: 1, Lane: 1, Agent: "claude", Model: "opus", RunID: 7, Kept: true, At: at(3)}, // a task's lane: the run's model
			{Arena: 1, Lane: 2, Agent: "codex", Model: "gpt-6.1-sol", At: at(3)},
			{Arena: 2, Lane: 1, Agent: "claude", Model: "claude-opus-4-5", At: at(4)}, // an id, read as its name
			{Arena: 2, Lane: 2, Agent: "codex", Model: "gpt-6.1-sol", Kept: true, At: at(4)},
		},
	}, day)
	opus := rowOf(t, card, "claude", "Opus 4.5")
	if opus.Kept.Count != 1 || opus.Kept.Of != 2 || opus.Runs != 1 || opus.ModelID != "claude-opus-4-5" {
		t.Fatalf("opus lanes: %+v", opus)
	}
	gpt := rowOf(t, card, "codex", "gpt-6.1-sol")
	if gpt.Kept.Count != 1 || gpt.Kept.Of != 2 || gpt.Runs != 0 || gpt.ModelID != "gpt-6.1-sol" || !gpt.Last.Equal(at(4)) {
		t.Fatalf("an arena-only row: %+v", gpt)
	}
}

func TestARowIsOfferedAPastTaskToTryItOn(t *testing.T) {
	older := runOf(1, "claude", "Opus 4.5", 1, "/w/a")
	newer := runOf(2, "claude", "Fable 5.1", 3, "/w/b")
	newer.Title = "Fix the login page"
	unfinished := runOf(3, "codex", "gpt-6.1-sol", 5, "/w/c")
	unfinished.Finished = ""
	card := Build(Records{Runs: []tasks.RunRecord{older, newer, unfinished}}, day)

	opus := rowOf(t, card, "claude", "Opus 4.5")
	if opus.Try == nil || opus.Try.TaskID != 2 || opus.Try.Title != "Fix the login page" || opus.Try.Identifier != "#2" || len(opus.Try.Lanes) != 2 ||
		opus.Try.Lanes[0].Model != "Fable 5.1" || opus.Try.Lanes[0].ModelID != "claude-fable-5-1" || opus.Try.Lanes[1].Model != "Opus 4.5" {
		t.Fatalf("opus on the newest task another model did: %+v", opus.Try)
	}
	fable := rowOf(t, card, "claude", "Fable 5.1")
	if fable.Try == nil || fable.Try.TaskID != 1 || fable.Try.Lanes[0].Model != "Opus 4.5" || fable.Try.Lanes[1].Model != "Fable 5.1" {
		t.Fatalf("fable on the newest task another model did: %+v", fable.Try)
	}
	gpt := rowOf(t, card, "codex", "gpt-6.1-sol")
	if gpt.Try == nil || gpt.Try.TaskID != 2 || gpt.Try.Lanes[1].Agent != "codex" {
		t.Fatalf("a model whose only run is unfinished: %+v", gpt.Try)
	}

	alone := Build(Records{Runs: []tasks.RunRecord{older}}, day).Rows[0]
	if alone.Try == nil || alone.Try.TaskID != 1 || len(alone.Try.Lanes) != 1 {
		t.Fatalf("the only model, on its own task with nobody beside it: %+v", alone.Try)
	}
	if none := Build(Records{Runs: []tasks.RunRecord{unfinished}}, day).Rows[0]; none.Try != nil {
		t.Fatalf("a task never finished was offered: %+v", none.Try)
	}
	if hidden := card.WithoutTasks(); hidden.Rows[0].Try != nil || card.Rows[0].Try == nil {
		t.Fatal("a viewer's card names a task, or the person's lost it")
	}
}

func TestRowsGoByHowMuchTheyDid(t *testing.T) {
	card := Build(Records{Runs: []tasks.RunRecord{
		runOf(1, "codex", "gpt-6.1-sol", 1, "/w/a"),
		runOf(2, "claude", "Opus 4.5", 2, "/w/b"),
		runOf(3, "claude", "Opus 4.5", 3, "/w/c"),
		runOf(4, "claude", "", 4, "/w/d"), // the meter never read it
	}}, day)
	if len(card.Rows) != 3 || card.Rows[0].Model != "Opus 4.5" || card.Rows[1].Model != "" || card.Rows[2].Model != "gpt-6.1-sol" || card.Rows[1].ModelID != "" {
		t.Fatalf("order: %+v", card.Rows)
	}
}
