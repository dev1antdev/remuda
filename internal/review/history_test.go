package review

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/state"
)

// snapshotHost answers MarkSeen's look with a HEAD and no uncommitted file.
func snapshotHost() *shell.Fake {
	return &shell.Fake{Name: "studio", Answer: func(string) (string, error) { return "0123456789abcdef0123456789abcdef01234567\n\x1e\n", nil }}
}

func TestEveryVerdictIsKeptInTheHistory(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	host := snapshotHost()
	for _, verdict := range []string{VerdictChanges, "", VerdictApproved} {
		if _, err := store.MarkSeen(host, "/w/x", verdict); err != nil {
			t.Fatal(err)
		}
	}
	got := store.Verdicts(time.Time{})
	if len(got) != 2 || got[0].Verdict != VerdictChanges || got[1].Verdict != VerdictApproved || got[1].Machine != "studio" || got[1].Dir != "/w/x" {
		t.Fatalf("history: %+v (a silent look is no verdict)", got)
	}
	if again := New(dir).Verdicts(time.Time{}); len(again) != 2 {
		t.Fatalf("the history was not kept across a restart: %+v", again)
	}
	if late := store.Verdicts(time.Now().Add(time.Hour)); len(late) != 0 {
		t.Fatalf("verdicts before since were listed: %+v", late)
	}
}

func TestAStoreFromBeforeTheHistoryStartsItWithItsBaselines(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	baselines := map[string]Baseline{
		"studio|/w/a": {Head: "h", Verdict: VerdictApproved, At: at.Add(time.Hour)},
		"studio|/w/b": {Head: "h", Verdict: VerdictChanges, At: at},
		"studio|/w/c": {Head: "h", At: at},
	}
	if err := state.SaveJSON(dir, baselineFile, baselines); err != nil {
		t.Fatal(err)
	}
	got := New(dir).Verdicts(time.Time{})
	if len(got) != 2 || got[0].Dir != "/w/b" || got[0].Verdict != VerdictChanges || got[1].Dir != "/w/a" || got[1].Machine != "studio" {
		t.Fatalf("seeded history: %+v", got)
	}
	// the seed is written at once: a later silent look over /w/a must not take its approval away
	if err := state.SaveJSON(dir, baselineFile, map[string]Baseline{}); err != nil {
		t.Fatal(err)
	}
	if again := New(dir).Verdicts(time.Time{}); len(again) != 2 {
		t.Fatalf("the seed was not kept: %+v", again)
	}
}

func TestTheHistoryKeepsTheNewest(t *testing.T) {
	store := New(t.TempDir())
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	store.mu.Lock()
	for i := range HistoryKeep {
		store.history = append(store.history, Verdict{Machine: "m", Dir: "/d", Verdict: VerdictApproved, At: at.Add(time.Duration(i) * time.Minute)})
	}
	for i := HistoryKeep; i < HistoryKeep+5; i++ {
		if err := store.noteVerdict(Verdict{Machine: "m", Dir: "/d", Verdict: VerdictApproved, At: at.Add(time.Duration(i) * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	store.mu.Unlock()
	got := store.Verdicts(time.Time{})
	if len(got) != HistoryKeep || !got[0].At.Equal(at.Add(5*time.Minute)) {
		t.Fatalf("kept %d, the oldest at %v", len(got), got[0].At)
	}
}
