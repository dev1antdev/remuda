package review

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/state"
)

const historyFile = "review-verdicts.json"

// HistoryKeep is how many verdicts the history holds, the oldest dropped first: months of reviews, a few hundred KB at most.
const HistoryKeep = 1000

// Verdict is one word a review ended with: approved or changes, on a folder of a machine, when.
type Verdict struct {
	Machine string    `json:"machine"`
	Dir     string    `json:"dir"`
	Verdict string    `json:"verdict"`
	At      time.Time `json:"at"`
}

// loadHistory reads the verdicts kept in dir; a store from before the history starts it with the verdict each
// baseline holds, so the reviews already given count; a damaged file is an empty history, said once.
func (store *Store) loadHistory() {
	if _, err := os.Stat(filepath.Join(store.dir, historyFile)); errors.Is(err, os.ErrNotExist) {
		store.history = seedHistory(store.baselines)
		if err := state.SaveJSON(store.dir, historyFile, append([]Verdict{}, store.history...)); err != nil {
			log.Printf("%s could not be started: %v", historyFile, err)
		}
		return
	}
	if err := state.LoadJSON(store.dir, historyFile, &store.history); err != nil {
		log.Printf("%s is unreadable (%v): the verdicts given so far are not counted", historyFile, err)
		store.history = nil
	}
}

// seedHistory is the verdict of every baseline that holds one, the oldest first.
func seedHistory(baselines map[string]Baseline) []Verdict {
	var out []Verdict
	for storeKey, baseline := range baselines {
		if baseline.Verdict == "" {
			continue
		}
		machine, dir, _ := strings.Cut(storeKey, "|")
		out = append(out, Verdict{Machine: machine, Dir: dir, Verdict: baseline.Verdict, At: baseline.At})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return keepNewest(out)
}

func keepNewest(verdicts []Verdict) []Verdict {
	if len(verdicts) > HistoryKeep {
		return append([]Verdict(nil), verdicts[len(verdicts)-HistoryKeep:]...)
	}
	return verdicts
}

// noteVerdict adds a verdict to the history and keeps the file; the caller holds the lock. A look without a word is none.
func (store *Store) noteVerdict(verdict Verdict) error {
	if verdict.Verdict == "" {
		return nil
	}
	store.history = keepNewest(append(store.history, verdict))
	return state.SaveJSON(store.dir, historyFile, store.history)
}

// Verdicts are the verdicts given at or after since, the oldest first.
func (store *Store) Verdicts(since time.Time) []Verdict {
	store.mu.Lock()
	defer store.mu.Unlock()
	var out []Verdict
	for _, verdict := range store.history {
		if !verdict.At.Before(since) {
			out = append(out, verdict)
		}
	}
	return out
}
