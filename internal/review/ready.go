package review

import (
	"log"
	"strings"
	"time"
	"unicode"

	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/said"
)

// Marker is the line the rejected verdict asks the agent to end its answer with once the work is ready again.
const Marker = "READY FOR REVIEW"

var (
	readyPhrases = []string{"ready for review", "ready for another look", "ready for you to look", "ready for your review", "готово к ревью", "готово для ревью"}
	notReady     = []string{"not ready", "not yet ready", "isn't ready", "isn’t ready", "не готово", "will ", "i'll", "i’ll", "once "} // a promise for later is not the word
)

// ReadyWords reports whether an answer says the work is ready for another look: the Marker on a line of its own, or
// it or one of a few plain phrases on a line that promises nothing for later and whose sentence does not say "not"
// before it ("NOT READY FOR REVIEW", "won't be ready for review") — never inside a code fence.
func ReadyWords(text string) bool {
	inFence := false
	var lines []string
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || line == "" {
			continue
		}
		lines = append(lines, strings.TrimRight(said.Plain(line), " .!"))
	}
	for _, line := range lines {
		if line == Marker {
			return true
		}
		folded := strings.ToLower(line)
		if containsAny(folded, notReady) {
			continue
		}
		if at := firstIndex(folded, readyPhrases); at >= 0 && !negated(folded[:at]) {
			return true
		}
	}
	return false
}

// firstIndex is where the first of the phrases stands in the line, -1 when none does.
func firstIndex(line string, phrases []string) int {
	first := -1
	for _, phrase := range phrases {
		if at := strings.Index(line, phrase); at >= 0 && (first < 0 || at < first) {
			first = at
		}
	}
	return first
}

// negated says whether the sentence leading up to a ready phrase takes it back: "not quite", "won't be", "не".
func negated(before string) bool {
	if cut := strings.LastIndexAny(before, ".!?;"); cut >= 0 {
		before = before[cut+1:]
	}
	words := strings.FieldsFunc(before, func(r rune) bool { return !unicode.IsLetter(r) && r != '\'' && r != '’' })
	for _, word := range words {
		if word == "not" || word == "не" || strings.HasSuffix(word, "n't") || strings.HasSuffix(word, "n’t") {
			return true
		}
	}
	return false
}

func containsAny(line string, phrases []string) bool {
	for _, phrase := range phrases {
		if strings.Contains(line, phrase) {
			return true
		}
	}
	return false
}

// OnAnswer hears what an agent answered in a folder: when the folder was rejected at the last look and the answer
// says the work is ready again, the baseline notes it and the call answers true — once per rejection; a folder never
// reviewed, approved, or already noted is left alone.
func (store *Store) OnAnswer(machineID, dir, text string, now time.Time) bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	storeKey := key(machineID, dir)
	baseline, ok := store.baselines[storeKey]
	if !ok || baseline.Verdict != VerdictChanges || !baseline.Ready.IsZero() || !ReadyWords(text) {
		return false
	}
	baseline.Ready = now
	store.baselines[storeKey] = baseline
	if err := store.save(); err != nil {
		log.Printf("%s: the ready mark could not be kept: %v", dir, err)
	}
	return true
}

// Ready says whether the folder, rejected at the last look, has since said it is ready for another.
func (store *Store) Ready(machineID, dir string) bool {
	baseline, ok := store.baseline(machineID, dir)
	return ok && !baseline.Ready.IsZero()
}

// ReadyNotice is the push for a folder ready again: the workspace by name, the answer's first line, the pane to open.
func ReadyNotice(machineID, paneID, workspace, text string) push.Payload {
	return push.Payload{
		Machine: machineID, Pane: paneID, Kind: "review", Tag: "review|" + machineID + "|" + paneID,
		Title: workspace + " is ready for another look",
		Body:  said.FirstLine(text),
	}
}
