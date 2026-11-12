package scorecard

import (
	"path"
	"sort"
	"time"

	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/tasks"
)

// run is a task run of the period as the scorecard reads it: its start, and the verdicts given on its work.
type run struct {
	tasks.RunRecord
	started  time.Time
	verdicts []string // the verdicts its work got, the first first
}

// underWay says the run's agent has not answered yet and may still: its prompt on its way, or delivered and unanswered.
func (r *run) underWay() bool {
	return r.Finished == "" && (r.State == tasks.RunStarting || r.State == tasks.RunRunning)
}

// runsFrom are the runs started from since on, the oldest first; a run with no readable start is left out.
func runsFrom(records []tasks.RunRecord, since time.Time) []*run {
	var out []*run
	for _, rec := range records {
		started, err := time.Parse(time.RFC3339, rec.Started)
		if err != nil || started.Before(since) {
			continue
		}
		out = append(out, &run{RunRecord: rec, started: started})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].started.Equal(out[j].started) {
			return out[i].started.Before(out[j].started)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func folderKey(machine, dir string) string {
	if dir == "" {
		return ""
	}
	return machine + "|" + path.Clean(dir)
}

// workRuns are the runs that did a worktree's work, by machine and folder, the oldest first: a pipeline's later
// stages review, test or open the request for the first agent's work, so the work and its verdicts are the first's.
func workRuns(runs []*run) map[string][]*run {
	folders := map[string][]*run{}
	for _, r := range runs {
		if k := folderKey(r.Machine, r.Dir); k != "" && r.Stage == 0 {
			folders[k] = append(folders[k], r)
		}
	}
	return folders
}

// workAt is the work run a moment on a folder belongs to: the last one started there at or before it, nil for none.
func workAt(folders map[string][]*run, machine, dir string, at time.Time) *run {
	var found *run
	for _, r := range folders[folderKey(machine, dir)] {
		if r.started.After(at) {
			break
		}
		found = r
	}
	return found
}

// placeVerdicts gives each verdict of the period to the work run whose worktree it was given on — the last run started
// on that folder before it — and answers how many there were and how many no run of the period worked in.
func placeVerdicts(folders map[string][]*run, verdicts []review.Verdict, since time.Time) (given, unplaced int) {
	ordered := append([]review.Verdict(nil), verdicts...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].At.Before(ordered[j].At) })
	for _, v := range ordered {
		if v.At.Before(since) || (v.Verdict != review.VerdictApproved && v.Verdict != review.VerdictChanges) {
			continue
		}
		given++
		r := workAt(folders, v.Machine, v.Dir, v.At)
		if r == nil {
			unplaced++
			continue
		}
		r.verdicts = append(r.verdicts, v.Verdict)
	}
	return given, unplaced
}

// prCredit is one pull request of the period: the work run it is credited to, and whether it merged.
type prCredit struct {
	run    *run
	merged bool
}

// creditPRs counts each request once, for the work run of the worktree that named it first — a pipeline's request
// stage opens it for the first agent's work, and one carrying on work begun before the period credits nobody —
// merged when any run naming it saw it merged.
func creditPRs(folders map[string][]*run, runs []*run) []*prCredit {
	byURL := map[string]*prCredit{}
	var out []*prCredit
	for _, r := range runs {
		if r.PRURL == "" {
			continue
		}
		credit := byURL[r.PRURL]
		if credit == nil {
			work := r
			if r.Stage > 0 {
				if work = workAt(folders, r.Machine, r.Dir, r.started); work == nil {
					continue
				}
			}
			credit = &prCredit{run: work}
			byURL[r.PRURL] = credit
			out = append(out, credit)
		}
		credit.merged = credit.merged || r.Merged != ""
	}
	return out
}
