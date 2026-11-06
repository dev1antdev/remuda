package opinion

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/transcript"
)

func asked(store *Store, pane string) Opinion {
	return store.Begin(Opinion{Machine: "gog", Dir: "/w/app", Agent: "codex", Workspace: "w3", Tab: "w3:t2", Pane: pane, Head: "abc", Close: true})
}

func TestAnOpinionIsAnsweredOnceByItsOwnReviewer(t *testing.T) {
	store := NewStore(t.TempDir())
	asked(store, "w3:p2")
	if _, ok := store.Answer("gog", "w3:p1", "FINDINGS\na.go:1: x"); ok {
		t.Fatal("another pane's answer was taken for the reviewer's")
	}
	if _, ok := store.Answer("local", "w3:p2", "FINDINGS\na.go:1: x"); ok {
		t.Fatal("the same pane id on another machine answered")
	}
	op, ok := store.Answer("gog", "w3:p2", "FINDINGS\n/w/app/a.go:1: [high] x\nb.go:2: y\nSUMMARY\nok")
	if !ok || op.State != Done || op.Open() != 2 || op.Comments[0].ID != 1 || op.Comments[0].Path != "a.go" || op.Summary != "ok" || op.Answered.IsZero() {
		t.Fatalf("answered %+v", op)
	}
	if _, ok := store.Answer("gog", "w3:p2", "FINDINGS\nc.go:3: z"); ok {
		t.Fatal("a later answer of the reviewer replaced the read one")
	}
}

func TestAnUnreadableAnswerAndAGoneReviewerAreNoted(t *testing.T) {
	store := NewStore(t.TempDir())
	asked(store, "w3:p2")
	op, ok := store.Answer("gog", "w3:p2", "I could not read the diff.")
	if !ok || op.State != Unreadable || op.Note == "" || len(op.Comments) != 0 {
		t.Fatalf("unreadable %+v", op)
	}
	asked(store, "w3:p4")
	if !store.Gone("gog", "w3:p4") {
		t.Fatal("the reviewer's pane closing went unnoted")
	}
	if op, _ := store.Get("gog", "/w/app"); op.State != Gone || op.Note == "" {
		t.Fatalf("gone %+v", op)
	}
	if store.Gone("gog", "w3:p4") {
		t.Fatal("a gone opinion was noted twice")
	}
}

func TestAReviewerThatAnsweredUnreadablyIsHeardAgain(t *testing.T) {
	store := NewStore(t.TempDir())
	asked(store, "w3:p2")
	store.Answer("gog", "w3:p2", "Should I also look at the migrations?")
	if !store.Waiting("gog", "w3:p2") {
		t.Fatal("a reviewer that asked something in plain words stopped being heard")
	}
	op, ok := store.Answer("gog", "w3:p2", "FINDINGS\na.go:4: [low] x\nSUMMARY\nfine")
	if !ok || op.State != Done || op.Open() != 1 || op.Note != "" {
		t.Fatalf("its later answer: %+v", op)
	}
	if store.Waiting("gog", "w3:p2") {
		t.Fatal("a read answer kept the reviewer heard")
	}
}

func TestAFailureMarksOnlyTheOpinionAskedOfThatPane(t *testing.T) {
	store := NewStore(t.TempDir())
	asked(store, "w3:p2")
	asked(store, "w3:p5") // asked again in the same folder: the newer one stands
	store.Fail("gog", "w3:p2", "the request did not reach the reviewer")
	if op, _ := store.Get("gog", "/w/app"); op.State != Running || op.Pane != "w3:p5" {
		t.Fatalf("an older request's failure marked the newer opinion: %+v", op)
	}
	store.Fail("gog", "w3:p5", "the request did not reach the reviewer")
	if op, _ := store.Get("gog", "/w/app"); op.State != Gone || op.Note != "the request did not reach the reviewer" {
		t.Fatalf("its own failure: %+v", op)
	}
}

func TestRunningAndReviewerTellAReviewersPaneFromTheFoldersAgent(t *testing.T) {
	store := NewStore(t.TempDir())
	asked(store, "w3:p2")
	if _, ok := store.Running("gog", "/w/app"); !ok {
		t.Fatal("a request not answered yet is not running")
	}
	if !store.Reviewer("gog", "w3:p2") || store.Reviewer("gog", "w3:p1") || store.Reviewer("local", "w3:p2") {
		t.Fatal("the reviewer's pane was not told from the folder's agent")
	}
	store.Answer("gog", "w3:p2", "FINDINGS\nNONE")
	if _, ok := store.Running("gog", "/w/app"); ok {
		t.Fatal("an answered opinion still runs")
	}
	if !store.Reviewer("gog", "w3:p2") {
		t.Fatal("a reviewer that answered stopped being one: its next answer would count as the folder's agent's")
	}
}

func TestCommentsOnFilesChangedSinceTheRequestAreMoved(t *testing.T) {
	op := Opinion{
		Blobs: map[string]string{"a.go": "1111", "b.go": "2222", "d.go": "4444"},
		Comments: []Comment{
			{ID: 1, Finding: Finding{Path: "a.go", Line: 3}},
			{ID: 2, Finding: Finding{Path: "b.go", Line: 4}},
			{ID: 3, Finding: Finding{Path: "b.go", Line: 9}},
			{ID: 4, Finding: Finding{Path: "c.go", Line: 1}},
			{ID: 5, Finding: Finding{Path: "d.go", Line: 1}},
			{ID: 6, Finding: Finding{Path: "e.go", Line: 1}, Dismissed: true},
		},
	}
	now := map[string]string{"a.go": "1111", "b.go": "2299", "c.go": "3333", "e.go": "5555"}
	if got := Moved(op, now); len(got) != 2 || got[0] != "b.go" || got[1] != "c.go" {
		t.Fatalf("moved %v: an unchanged file, a file committed since and a dismissed comment's file are not moved", got)
	}
}

func TestASnapshotReadsAReviewerAtRestAndNotesOneGone(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.retry, svc.readEvery = time.Millisecond, time.Hour
	op := svc.Begin(Opinion{Machine: "gog", Dir: "/a", Pane: "w1:p2", Tab: "w1:t2"})
	gone := svc.Begin(Opinion{Machine: "gog", Dir: "/b", Pane: "w2:p2", Tab: "w2:t2"})
	reads := 0
	svc.Read = func(asked Opinion) (string, bool) {
		reads++
		return Answered(turnsOf(Prompt(Scope{}, op.Ref), "FINDINGS\na.go:1: x"), asked)
	}
	snap := &herdr.Snapshot{Panes: []herdr.PaneInfo{{PaneID: "w1:p2", TabID: "w1:t2", AgentStatus: "working"}}}
	svc.Observe(events.Snapshot{Machine: "gog", Snap: snap, At: gone.Asked.Add(time.Second)})
	if b, _ := svc.Get("gog", "/b"); reads != 0 || b.State != Running {
		t.Fatal("a reviewer at work was read, or a pane missing from a snapshot older than its tab was taken for gone")
	}
	snap.Panes[0].AgentStatus = "done"
	svc.Observe(events.Snapshot{Machine: "gog", Snap: snap, At: gone.Asked.Add(time.Minute)})
	deadline := time.Now().Add(2 * time.Second)
	for svc.Waiting("gog", "w1:p2") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got, _ := svc.Get("gog", "/a"); got.State != Done || got.Open() != 1 {
		t.Fatalf("a finish no status move told of: %+v", got)
	}
	if got, _ := svc.Get("gog", "/b"); got.State != Gone {
		t.Fatalf("a reviewer's pane missing from the machine: %+v", got)
	}
}

// turnsOf is a conversation of a prompt and the answer to it, "" for none yet
func turnsOf(prompt, answer string) []transcript.Turn {
	turns := []transcript.Turn{{Role: "user", Parts: []transcript.Part{{Type: "text", Text: prompt}}}}
	if answer != "" {
		turns = append(turns, transcript.Turn{Role: "assistant", Parts: []transcript.Part{{Type: "text", Text: answer}}})
	}
	return turns
}

func TestCommentsAreDismissedAndOpinionsKeptAcrossARestart(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	first := asked(store, "w3:p2")
	store.Answer("gog", "w3:p2", "FINDINGS\na.go:1: x\nb.go:2: y\nc.go:3: z")
	op, err := store.Dismiss("gog", "/w/app", []int{2})
	if err != nil || op.Open() != 2 || !op.Comments[1].Dismissed {
		t.Fatalf("dismissed one %+v %v", op, err)
	}
	again := NewStore(dir)
	kept, ok := again.Get("gog", "/w/app")
	if !ok || kept.Open() != 2 || kept.Agent != "codex" || kept.Pane != first.Pane || !kept.Close {
		t.Fatalf("after a restart %+v", kept)
	}
	if op, _ := again.Dismiss("gog", "/w/app", nil); op.Open() != 0 {
		t.Fatalf("dismissed all %+v", op)
	}
	if _, err := again.Dismiss("gog", "/w/other", nil); err != ErrNone {
		t.Fatalf("a folder without an opinion: %v", err)
	}
	asked(again, "w3:p9")
	if op, _ := again.Get("gog", "/w/app"); op.State != Running || len(op.Comments) != 0 || op.Pane != "w3:p9" {
		t.Fatalf("a second run did not replace the first: %+v", op)
	}
	again.Forget("gog", "/w/app")
	if _, ok := again.Get("gog", "/w/app"); ok {
		t.Fatal("a forgotten opinion stayed")
	}
	again.Begin(Opinion{Machine: "roofr", Dir: "/r"})
	again.ForgetMachine("roofr")
	if _, ok := again.Get("roofr", "/r"); ok {
		t.Fatal("a removed machine's opinion stayed")
	}
}

func TestADamagedFileIsAnEmptyStore(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, file), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(dir)
	if _, ok := store.Get("gog", "/w/app"); ok {
		t.Fatal("a damaged file read as an opinion")
	}
	asked(store, "w3:p2")
	if _, ok := NewStore(dir).Get("gog", "/w/app"); !ok {
		t.Fatal("the store did not write over a damaged file")
	}
}

func TestAReviewerAtRestIsReadWhenItsConversationIsTheRequest(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.retry = time.Millisecond
	refs := map[string]string{}
	for _, pane := range []string{"w1:p2", "w2:p2", "w3:p2", "w4:p2"} {
		refs[pane] = svc.Begin(Opinion{Machine: "gog", Dir: "/" + pane, Pane: pane, Tab: pane}).Ref
	}
	reads := map[string][2]string{
		"w1:p2": {Prompt(Scope{}, refs["w1:p2"]), "FINDINGS\na.go:1: [high] x"},
		"w2:p2": {"Fix the login page", "FINDINGS\nb.go:2: y"},              // the person's own conversation in the folder
		"w3:p2": {Prompt(Scope{}, refs["w3:p2"]), ""},                       // not answered yet
		"w4:p2": {Prompt(Scope{}, "an-earlier-one"), "FINDINGS\nc.go:3: z"}, // an earlier request's conversation in the folder
	}
	svc.Read = func(op Opinion) (string, bool) {
		read, ok := reads[op.Pane]
		if !ok {
			return "", false
		}
		return Answered(turnsOf(read[0], read[1]), op)
	}
	done := func(pane, to string) {
		svc.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "gog", Pane: pane}, To: to})
	}
	done("w1:p2", "working")
	if !svc.Waiting("gog", "w1:p2") {
		t.Fatal("a reviewer at work was read")
	}
	done("w1:p2", "done")
	done("w2:p2", "idle")
	done("w3:p2", "done")
	done("w4:p2", "done")
	deadline := time.Now().Add(2 * time.Second)
	for svc.Waiting("gog", "w1:p2") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	if op, _ := svc.Get("gog", "/w1:p2"); op.State != Done || op.Open() != 1 {
		t.Fatalf("the reviewer's answer: %+v", op)
	}
	if !svc.Waiting("gog", "w2:p2") || !svc.Waiting("gog", "w3:p2") || !svc.Waiting("gog", "w4:p2") {
		t.Fatal("another conversation, an earlier request's, or none yet, was taken for the review")
	}
	done("w3:p2", events.Gone)
	if op, _ := svc.Get("gog", "/w3:p2"); op.State != Gone {
		t.Fatalf("a reviewer's pane closing: %+v", op)
	}
}

func TestTheReviewersTabClosesOnlyAfterAReadableAnswer(t *testing.T) {
	svc := NewService(t.TempDir())
	var closed []string
	svc.Close = func(machine, tab, pane string) error {
		closed = append(closed, machine+" "+tab+" "+pane)
		return nil
	}
	svc.Begin(Opinion{Machine: "gog", Dir: "/a", Tab: "w1:t2", Pane: "w1:p2", Close: true})
	svc.Begin(Opinion{Machine: "gog", Dir: "/b", Tab: "w2:t2", Pane: "w2:p2", Close: true})
	svc.Begin(Opinion{Machine: "gog", Dir: "/c", Tab: "w3:t2", Pane: "w3:p2", Close: false})
	svc.Hear("gog", "w1:p2", "FINDINGS\nNONE")
	svc.Hear("gog", "w2:p2", "nonsense")
	svc.Hear("gog", "w3:p2", "FINDINGS\na.go:1: x")
	svc.Hear("gog", "w9:p9", "FINDINGS\na.go:1: x")
	if len(closed) != 1 || closed[0] != "gog w1:t2 w1:p2" {
		t.Fatalf("closed %v", closed)
	}
	store := NewStore(t.TempDir())
	store.now = func() time.Time { return time.Unix(100, 0) }
	if op := asked(store, "p"); !op.Asked.Equal(time.Unix(100, 0)) {
		t.Fatalf("asked at %v", op.Asked)
	}
}

func TestADismissLeavesACopyHandedOutBeforeAlone(t *testing.T) {
	store := NewStore(t.TempDir())
	asked(store, "w3:p2")
	store.Answer("gog", "w3:p2", "FINDINGS\na.go:1: x\nb.go:2: y")
	before, _ := store.Get("gog", "/w/app") // being written out to a page while the dismiss runs
	if _, err := store.Dismiss("gog", "/w/app", nil); err != nil {
		t.Fatal(err)
	}
	if before.Comments[0].Dismissed || before.Comments[1].Dismissed {
		t.Fatal("a dismiss wrote into a copy handed out before it, outside the lock")
	}
}
