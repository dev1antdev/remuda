package tasks

import (
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/push"
)

func TestStageDoneIsTheMarkerAtTheEnd(t *testing.T) {
	for answer, want := range map[string]bool{
		"Implemented the page.\n\nSTAGE DONE":       true,
		"Implemented.\n\n**STAGE DONE**":            true,
		"All tests pass. STAGE DONE.":               true,
		"STAGE DONE\n\nOne more thing: should I?":   false,
		"Done.\n```\nSTAGE DONE\n```":               false,
		"Should I also change the footer?":          false,
		"The marker is STAGE DONE, I will write it": false,
		"": false,
	} {
		if got := StageDone(answer); got != want {
			t.Errorf("StageDone(%q) = %v, want %v", answer, got, want)
		}
	}
	if got := withoutMarker("Implemented the page.\n\nSTAGE DONE\n"); got != "Implemented the page." {
		t.Errorf("withoutMarker = %q", got)
	}
	if got := withoutMarker("All pass. **STAGE DONE**"); got != "All pass." {
		t.Errorf("withoutMarker on one line = %q", got)
	}
}

func TestStagePromptFillsTheTaskThePreviousAnswerAndTheBranch(t *testing.T) {
	tk := Task{Title: "Fix login", Identifier: "#7"}
	got := StagePrompt(Stage{Prompt: "Review the work on {branch}.\n\nThe last stage said:\n{previous}\n\nThe task:\n{task}"}, tk, Project{Title: "Work"}, "It is fixed.", "task-7-fix-login")
	for _, want := range []string{"Review the work on task-7-fix-login.", "The last stage said:\nIt is fixed.", "Fix login\n\n(remuda task #7 in Work)", StageAsk} {
		if !strings.Contains(got, want) {
			t.Errorf("the prompt lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, PRAsk) {
		t.Error("a stage without PR asks for a pull request")
	}
	pr := StagePrompt(Stage{PR: true}, tk, Project{}, "", "b")
	if !strings.HasPrefix(pr, PRAsk) || !strings.HasSuffix(pr, StageAsk) {
		t.Errorf("a PR stage's prompt:\n%s", pr)
	}
}

// piped is the Inbox with a Review column and two stages after the first agent: Codex testing in Review, then a PR stage
func piped(t *testing.T, st *Store) (Project, Status) {
	t.Helper()
	ps, _ := st.Projects()
	inbox := ps[0]
	var cols []Status
	for _, s := range inbox.Statuses {
		if s.Kind == KindDone {
			cols = append(cols, Status{Title: "Review", Kind: KindOpen})
		}
		cols = append(cols, s)
	}
	cols, err := st.SetStatuses(inbox.ID, cols, nil)
	if err != nil {
		t.Fatal(err)
	}
	review := *statusByTitle(cols, "Review")
	p, err := st.SetAutomation(inbox.ID, Automation{Stages: []Stage{
		{Name: "Tests", Agent: "codex", Prompt: "Test {branch}: {previous}", StatusID: review.ID},
		{Name: "PR", PR: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return p, review
}

// pipeHere is a run watch over fakes: every pane idle with the answer given, every advance a new run on its own pane
type pipeHere struct {
	w        *RunWatch
	states   map[string]string
	answers  map[string]string
	advances []struct {
		index  int
		prompt string
	}
	pushed []push.Payload
}

func newPipe(st *Store) *pipeHere {
	h := &pipeHere{states: map[string]string{}, answers: map[string]string{}}
	clock := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	h.w = &RunWatch{
		Store: st,
		PaneState: func(m, p string) (string, time.Time, bool) {
			s, ok := h.states[p]
			return s, clock, ok
		},
		LastAnswer: func(m, p string) (string, string) {
			a := h.answers[p]
			if a == "" {
				return "", ""
			}
			return a, "rec-" + p + "-" + a[:3]
		},
		Advance: func(prev Run, index int, stage Stage, prompt string) error {
			h.advances = append(h.advances, struct {
				index  int
				prompt string
			}{index, prompt})
			pane := "w1:p" + string(rune('1'+index))
			r, err := st.StartStageRun(prev, index, stage, pane)
			if err == nil {
				_ = st.SetRun(r.ID, RunRunning, "")
				h.states[pane] = "working"
			}
			return err
		},
		Notify: func(p push.Payload) { h.pushed = append(h.pushed, p) },
		Now:    func() time.Time { return clock },
	}
	return h
}

func startPiped(t *testing.T, st *Store, p Project) (Task, Run) {
	t.Helper()
	tk, _ := st.Create(p.ID, Patch{Title: str("Fix login")})
	run, err := st.StartRun(Run{TaskID: tk.ID, Machine: "studio", Workspace: "w1", Pane: "w1:p1", Dir: "/w/wt", Branch: "task-1-fix-login", Agent: "claude", Pipeline: PipelineOn})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.SetRun(run.ID, RunRunning, "")
	return tk, run
}

func TestAPipelineRunsItsStagesOneAfterAnother(t *testing.T) {
	st := open(t)
	p, review := piped(t, st)
	tk, first := startPiped(t, st, p)
	h := newPipe(st)

	h.states["w1:p1"], h.answers["w1:p1"] = "idle", "Fixed the login form.\n\nSTAGE DONE"
	h.w.Sweep()
	if len(h.advances) != 1 || h.advances[0].index != 1 {
		t.Fatalf("advances %+v", h.advances)
	}
	if prompt := h.advances[0].prompt; !strings.HasPrefix(prompt, "Test task-1-fix-login: Fixed the login form.") || !strings.HasSuffix(prompt, StageAsk) {
		t.Fatalf("the Tests stage's prompt:\n%s", prompt)
	}
	got, _ := st.Get(tk.ID)
	if got.StatusID != review.ID || got.Run == nil || got.Run.Stage != 1 || got.Run.Pipeline != PipelineOn || got.Run.Agent != "codex" || got.Run.Dir != first.Dir {
		t.Fatalf("after the first stage: column %d (want %d), run %+v", got.StatusID, review.ID, got.Run)
	}
	if was, _ := st.Run(first.ID); was.Pipeline != PipelinePassed {
		t.Fatalf("the first run's pipeline %q", was.Pipeline)
	}
	if len(h.pushed) != 1 || !strings.Contains(h.pushed[0].Title, "stage 1 of 2, Tests") || !strings.Contains(h.pushed[0].Title, "→ Review") {
		t.Fatalf("pushes %+v", h.pushed)
	}

	h.w.Sweep()
	if len(h.advances) != 1 {
		t.Fatal("an answer already handled moved the pipeline again")
	}
	h.states["w1:p2"], h.answers["w1:p2"] = "idle", "All 41 tests pass.\nSTAGE DONE"
	h.w.Sweep()
	if len(h.advances) != 2 || h.advances[1].index != 2 || !strings.HasPrefix(h.advances[1].prompt, PRAsk) {
		t.Fatalf("the PR stage %+v", h.advances)
	}
	if got, _ := st.Get(tk.ID); got.StatusID != review.ID {
		t.Fatal("a stage without a column moved the card")
	}

	h.states["w1:p3"], h.answers["w1:p3"] = "idle", "Opened https://github.com/o/r/pull/7.\n\nSTAGE DONE"
	h.w.Sweep()
	got, _ = st.Get(tk.ID)
	finish := column(p, func(s Status) bool { return s.OnFinish })
	if got.Run.Pipeline != PipelineDone || got.StatusID != finish.ID {
		t.Fatalf("after the last stage: pipeline %q in column %d, want %d", got.Run.Pipeline, got.StatusID, finish.ID)
	}
	if len(h.advances) != 2 {
		t.Fatal("the pipeline went past its last stage")
	}
	cs, _ := st.Comments(tk.ID)
	var remuda []string
	for _, c := range cs {
		if c.Author == "remuda" {
			remuda = append(remuda, c.Body)
		}
	}
	if len(remuda) != 3 || !strings.Contains(remuda[0], "Stage 1 of 2, Tests: codex takes over") || !strings.Contains(remuda[2], "The pipeline is done") {
		t.Fatalf("remuda's comments %q", remuda)
	}
}

func TestThePipelineStopsForAMovedCardAQuestionOrAnAnswerWithoutTheMarker(t *testing.T) {
	for _, c := range []struct {
		name string
		set  func(st *Store, h *pipeHere, tk Task, p Project)
		want string
	}{
		{"moved", func(st *Store, h *pipeHere, tk Task, p Project) {
			landing := column(p, func(s Status) bool { return s.OnNew })
			_, _ = st.Update(tk.ID, Patch{StatusID: &landing.ID})
			h.states["w1:p1"], h.answers["w1:p1"] = "idle", "Fixed.\nSTAGE DONE"
		}, "the card was moved by hand"},
		{"asks", func(st *Store, h *pipeHere, tk Task, p Project) {
			h.states["w1:p1"] = "blocked"
		}, "the agent asked something"},
		{"no marker", func(st *Store, h *pipeHere, tk Task, p Project) {
			h.states["w1:p1"], h.answers["w1:p1"] = "idle", "Should the button be blue or green?"
		}, "the agent answered without STAGE DONE"},
	} {
		t.Run(c.name, func(t *testing.T) {
			st := open(t)
			p, _ := piped(t, st)
			tk, run := startPiped(t, st, p)
			h := newPipe(st)
			c.set(st, h, tk, p)
			h.w.Sweep()
			if len(h.advances) != 0 {
				t.Fatal("the pipeline moved on")
			}
			got, _ := st.Run(run.ID)
			if got.Pipeline != "stopped: "+c.want {
				t.Fatalf("pipeline %q, want stopped: %s", got.Pipeline, c.want)
			}
			if cs, _ := st.Comments(tk.ID); !strings.Contains(cs[len(cs)-1].Body, "The pipeline stops at the first agent's work: "+c.want) {
				t.Fatalf("comments %+v", cs)
			}
			h.w.Sweep()
			if got, _ := st.Run(run.ID); got.Pipeline != "stopped: "+c.want {
				t.Fatalf("a second sweep changed the stop: %q", got.Pipeline)
			}
		})
	}
}

func TestARunWithoutAPipelineFinishesAsBefore(t *testing.T) {
	st := open(t)
	ps, _ := st.Projects()
	tk, _ := st.Create(ps[0].ID, Patch{Title: str("Plain")})
	run, _ := st.StartRun(Run{TaskID: tk.ID, Machine: "studio", Workspace: "w1", Pane: "w1:p1", Agent: "claude"})
	_ = st.SetRun(run.ID, RunRunning, "")
	h := newPipe(st)
	h.states["w1:p1"], h.answers["w1:p1"] = "idle", "Done.\nSTAGE DONE"
	h.w.Sweep()
	got, _ := st.Get(tk.ID)
	if len(h.advances) != 0 || got.Run.Pipeline != "" || got.StatusID != column(ps[0], func(s Status) bool { return s.OnFinish }).ID {
		t.Fatalf("a plain run: advances %d, pipeline %q, column %d", len(h.advances), got.Run.Pipeline, got.StatusID)
	}
}
