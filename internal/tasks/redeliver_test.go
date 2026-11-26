package tasks

import (
	"testing"
	"time"
)

func TestAStartingRunKeepsItsPromptAndIsHandedOverAgainAfterARestart(t *testing.T) {
	st := open(t)
	clock := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	st.now = func() time.Time { return clock }
	inbox, _ := st.Projects()
	tk, _ := st.Create(inbox[0].ID, Patch{Title: str("Ship it")})
	run, err := st.StartRun(Run{TaskID: tk.ID, Machine: "studio", Workspace: "w1", Pane: "w1:p1", Dir: "/w/wt", Agent: "claude", Prompt: "do the thing"})
	if err != nil || run.Prompt != "do the thing" {
		t.Fatalf("start %+v, %v", run, err)
	}
	if kept, err := st.Run(run.ID); err != nil || kept.Prompt != "do the thing" {
		t.Fatalf("the run on disk %+v, %v", kept, err)
	}

	var handed []Run
	w := &RunWatch{
		Store:      st,
		PaneState:  func(m, p string) (string, time.Time, bool) { return "unknown", clock, true },
		LastAnswer: func(m, p string) (string, string) { return "", "" },
		Redeliver:  func(r Run) { handed = append(handed, r) },
		Now:        func() time.Time { return clock },
	}
	w.ResumeStarting()
	if len(handed) != 1 || handed[0].ID != run.ID || handed[0].Prompt != "do the thing" {
		t.Fatalf("handed over after the restart: %+v", handed)
	}

	// the window counts from the hand-over, not from a start remuda was not there for
	clock = clock.Add(6 * time.Minute)
	w.Sweep()
	if r, _ := st.Run(run.ID); r.State != RunStarting {
		t.Fatalf("given up six minutes after the hand-over: %+v", r)
	}
	clock = clock.Add(7 * time.Minute)
	w.Sweep()
	if r, _ := st.Run(run.ID); r.State != RunFailed {
		t.Fatalf("never given up: %+v", r)
	}
}

func TestARunWithNoSavedPromptIsNotHandedOverAgain(t *testing.T) {
	st := open(t)
	inbox, _ := st.Projects()
	tk, _ := st.Create(inbox[0].ID, Patch{Title: str("x")})
	if _, err := st.StartRun(Run{TaskID: tk.ID, Machine: "studio", Workspace: "w1", Pane: "w1:p1", Agent: "claude"}); err != nil {
		t.Fatal(err)
	}
	handed := 0
	w := &RunWatch{Store: st, Redeliver: func(r Run) { handed++ }}
	w.ResumeStarting()
	if handed != 0 {
		t.Fatalf("handed over a run with no prompt: %d", handed)
	}
}
