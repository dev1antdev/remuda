package opinion

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
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
	svc.Read = func(machine, pane string) (string, string, bool) {
		read, ok := reads[pane]
		return read[0], read[1], ok
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
	svc.Close = func(machine, tab string) error {
		closed = append(closed, machine+" "+tab)
		return nil
	}
	svc.Begin(Opinion{Machine: "gog", Dir: "/a", Tab: "w1:t2", Pane: "w1:p2", Close: true})
	svc.Begin(Opinion{Machine: "gog", Dir: "/b", Tab: "w2:t2", Pane: "w2:p2", Close: true})
	svc.Begin(Opinion{Machine: "gog", Dir: "/c", Tab: "w3:t2", Pane: "w3:p2", Close: false})
	svc.Hear("gog", "w1:p2", "FINDINGS\nNONE")
	svc.Hear("gog", "w2:p2", "nonsense")
	svc.Hear("gog", "w3:p2", "FINDINGS\na.go:1: x")
	svc.Hear("gog", "w9:p9", "FINDINGS\na.go:1: x")
	if len(closed) != 1 || closed[0] != "gog w1:t2" {
		t.Fatalf("closed %v", closed)
	}
	store := NewStore(t.TempDir())
	store.now = func() time.Time { return time.Unix(100, 0) }
	if op := asked(store, "p"); !op.Asked.Equal(time.Unix(100, 0)) {
		t.Fatalf("asked at %v", op.Asked)
	}
}
