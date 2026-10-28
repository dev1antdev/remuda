package review

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

func TestReadyWords(t *testing.T) {
	cases := map[string]bool{
		"Fixed the three comments.\n\nREADY FOR REVIEW": true,
		"**READY FOR REVIEW**":                          true,
		"- READY FOR REVIEW":                            true, // a list mark is stripped
		"All done and pushed. READY FOR REVIEW.":        true, // the last line ends with it
		"I will write READY FOR REVIEW when done.\n\nStill working on the parser.": false,
		"Done; I'll say when it is ready for review.":                              false,
		"The changes are ready for another look.":                                  true,
		"Ready For Review": true, // the phrase case-folds; only the marker is case-sensitive
		"Готово к ревью, посмотри ещё раз.":                           true,
		"It is not ready for review yet: the tests fail.":             false,
		"```\nREADY FOR REVIEW\n```\nThat is what the script prints.": false,
		"":      false,
		"Done.": false,
	}
	for text, want := range cases {
		if got := ReadyWords(text); got != want {
			t.Errorf("ReadyWords(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestOnAnswerMarksARejectedFolderOnceUntilTheNextLook(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	if store.OnAnswer("local", "/x", "READY FOR REVIEW", now) {
		t.Fatal("a folder never reviewed was marked")
	}
	store.mu.Lock()
	store.baselines[key("local", "/x")] = Baseline{Head: "h", Blobs: map[string]string{}, Verdict: VerdictChanges, At: now}
	store.baselines[key("local", "/ok")] = Baseline{Head: "h", Blobs: map[string]string{}, Verdict: VerdictApproved, At: now}
	store.mu.Unlock()
	if store.OnAnswer("local", "/x", "Still working on it.", now) || store.Ready("local", "/x") {
		t.Fatal("an answer that says nothing marked the folder")
	}
	if store.OnAnswer("local", "/ok", "READY FOR REVIEW", now) || store.Ready("local", "/ok") {
		t.Fatal("an approved folder was marked")
	}
	if !store.OnAnswer("local", "/x", "Fixed both.\n\nREADY FOR REVIEW", now.Add(time.Minute)) || !store.Ready("local", "/x") {
		t.Fatal("the rejected folder was not marked")
	}
	if store.OnAnswer("local", "/x", "READY FOR REVIEW", now.Add(2*time.Minute)) {
		t.Fatal("a second answer marked it again")
	}
	again := New(dir)
	if kept, _ := again.baseline("local", "/x"); !again.Ready("local", "/x") || !kept.Ready.Equal(now.Add(time.Minute)) || kept.Verdict != VerdictChanges {
		t.Fatalf("the mark was not kept on disk: %+v", kept)
	}
	host := &shell.Fake{Name: "local", Dir: "/x", Answer: func(string) (string, error) { return "newhead\n\x1e\n", nil }}
	if _, err := again.MarkSeen(host, "/x", ""); err != nil {
		t.Fatal(err)
	}
	if again.Ready("local", "/x") {
		t.Fatal("a look left the mark")
	}
}

func TestReadyNotice(t *testing.T) {
	p := ReadyNotice("gog", "w3:p1", "remuda", "## Done\n\nFixed the two comments.\n\nREADY FOR REVIEW")
	if p.Kind != "review" || p.Machine != "gog" || p.Pane != "w3:p1" || p.Tag != "review|gog|w3:p1" {
		t.Fatalf("payload: %+v", p)
	}
	if p.Title != "remuda is ready for another look" || p.Body != "Done" {
		t.Fatalf("words: %q / %q", p.Title, p.Body)
	}
}
