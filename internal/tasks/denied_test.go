package tasks

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/transcript"
)

func TestNewAgentSettingsDenyPromptsAndSavedOnesKeepTheirWord(t *testing.T) {
	st := open(t)
	ps, _ := st.Projects()
	inbox := ps[0]
	if !inbox.Automation.Denies("claude") {
		t.Fatalf("settings never saved start with the switch on: %+v", inbox.Automation.DenyPrompts)
	}
	p, err := st.SetAutomation(inbox.ID, Automation{Machine: "studio", Dir: "/w/app", Agent: "claude", Max: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !p.Automation.Denies("claude") || p.Automation.Denies("codex") {
		t.Fatalf("the first save keeps it on, for Claude Code alone: %+v", p.Automation.DenyPrompts)
	}
	off := false
	if p, err = st.SetAutomation(inbox.ID, Automation{Machine: "studio", Dir: "/w/app", Agent: "claude", Max: 1, DenyPrompts: &off}); err != nil || p.Automation.Denies("claude") {
		t.Fatalf("switched off: %+v %v", p.Automation.DenyPrompts, err)
	}
	if p, _ = st.SetAutomation(inbox.ID, Automation{Machine: "studio", Dir: "/w/app", Agent: "claude", Max: 1}); p.Automation.Denies("claude") {
		t.Fatal("a save that says nothing leaves it off")
	}

	before := `{"machine":"studio","dir":"/w/app","agent":"claude","queue":false,"max":1,"stages":[]}`
	if _, err := st.db.Exec(`UPDATE projects SET automation = ? WHERE id = ?`, before, inbox.ID); err != nil {
		t.Fatal(err)
	}
	saved, _ := st.Project(inbox.ID)
	if saved.Automation.DenyPrompts != nil || saved.Automation.Denies("claude") {
		t.Fatalf("settings saved before the switch read as off: %+v", saved.Automation.DenyPrompts)
	}
	if again, _ := st.SetAutomation(inbox.ID, saved.Automation); again.Automation.Denies("claude") {
		t.Fatal("saved again by a page from before, still off")
	}
}

func TestTheProjectsOwnSettingsFileIsRefusedWhileTheSwitchIsOn(t *testing.T) {
	st := open(t)
	ps, _ := st.Projects()
	on, off := true, false
	file := Automation{Machine: "studio", Dir: "/w/app", Agent: "claude", Args: "--settings /w/night.json", Max: 1, DenyPrompts: &on}
	if _, err := st.SetAutomation(ps[0].ID, file); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "night.json") {
		t.Fatalf("a settings file cannot take the setting: %v", err)
	}
	file.DenyPrompts = &off
	if _, err := st.SetAutomation(ps[0].ID, file); err != nil {
		t.Fatalf("with the switch off the file is the person's: %v", err)
	}
}

func TestTheQueueStartsClaudeCodeDenyingPrompts(t *testing.T) {
	st := open(t)
	p := queued(t, st, 2)
	queue := column(p, func(s Status) bool { return s.OnQueue })
	into(t, st, p.ID, "Fix the login page", queue)
	h := newQueue(st)
	h.q.Sweep()
	if len(h.starts) != 1 || !h.starts[0].DenyPrompts {
		t.Fatalf("the queue's Claude Code denies prompts: %+v", h.starts)
	}
	a := p.Automation
	a.Agent = "codex"
	if _, err := st.SetAutomation(p.ID, a); err != nil {
		t.Fatal(err)
	}
	into(t, st, p.ID, "Add a dark theme", queue)
	h.q.Sweep()
	if len(h.starts) != 2 || h.starts[1].DenyPrompts || h.starts[1].Agent != "codex" {
		t.Fatalf("Codex starts as it did: %+v", h.starts)
	}
}

// deniedHere is a run watch over one pane whose conversation the test writes: its last answer and record, what it was
// denied after a record, the pane's state
type deniedHere struct {
	w       *RunWatch
	state   string
	answer  string
	record  string
	denied  transcript.Denials
	afters  []string
	pushed  []push.Payload
	advance int
}

func newDenied(st *Store) *deniedHere {
	h := &deniedHere{state: "idle"}
	clock := time.Date(2026, 11, 12, 3, 0, 0, 0, time.UTC)
	h.w = &RunWatch{
		Store:      st,
		PaneState:  func(m, p string) (string, time.Time, bool) { return h.state, clock, true },
		LastAnswer: func(m, p string) (string, string) { return h.answer, h.record },
		Denied: func(m, p, after string) transcript.Denials {
			h.afters = append(h.afters, after)
			return h.denied
		},
		Advance: func(prev Run, index int, stage Stage, prompt string) error { h.advance++; return nil },
		Notify:  func(p push.Payload) { h.pushed = append(h.pushed, p) },
		Now:     func() time.Time { return clock },
	}
	return h
}

func remudaSaid(t *testing.T, st *Store, taskID int64) []string {
	t.Helper()
	cs, _ := st.Comments(taskID)
	var out []string
	for _, c := range cs {
		if c.Author == "remuda" {
			out = append(out, c.Body)
		}
	}
	return out
}

func TestARunsDeniedCallsComeBackAsAComment(t *testing.T) {
	st := open(t)
	ps, _ := st.Projects()
	tk, _ := st.Create(ps[0].ID, Patch{Title: str("Fix login")})
	run, _ := st.StartRun(Run{TaskID: tk.ID, Machine: "studio", Workspace: "w1", Pane: "w1:p1", Dir: "/w/wt", Agent: "claude"})
	_ = st.SetRun(run.ID, RunRunning, "")
	h := newDenied(st)
	h.answer, h.record = "Fixed the form; the tests could not run.", "rec-1"
	h.denied = transcript.Denials{Calls: []transcript.Denial{{Tool: "Bash", What: "npm test"}, {Tool: "Write", What: "/w/wt/notes.txt"}}}
	h.w.Sweep()
	said := remudaSaid(t, st, tk.ID)
	if len(said) != 1 || !strings.Contains(said[0], "Claude Code was denied 2 calls") || !strings.Contains(said[0], "- Bash: `npm test`") || !strings.Contains(said[0], "- Write: `/w/wt/notes.txt`") || strings.Contains(said[0], "stopped there") {
		t.Fatalf("remuda's comment: %q", said)
	}
	if len(h.afters) != 1 || h.afters[0] != "" {
		t.Fatalf("the first finish reads the whole conversation: %q", h.afters)
	}
	if len(h.pushed) != 1 || !strings.Contains(h.pushed[0].Body, "2 calls denied") || !strings.Contains(h.pushed[0].Title, "finished") {
		t.Fatalf("pushed %+v", h.pushed)
	}

	h.w.Sweep() // the same finish again: nothing new
	h.answer, h.record = "Committed.", "rec-2"
	h.denied = transcript.Denials{}
	h.w.Sweep()
	if len(remudaSaid(t, st, tk.ID)) != 1 || h.afters[len(h.afters)-1] != "rec-1" {
		t.Fatalf("a later finish reads after the last one, and with nothing denied says nothing: %q %q", remudaSaid(t, st, tk.ID), h.afters)
	}
}

func TestARunThatStoppedOnItsDeniedCallsIsReported(t *testing.T) {
	st := open(t)
	p, _ := piped(t, st)
	tk, run := startPiped(t, st, p)
	h := newDenied(st)
	h.answer, h.record = "", "rec-stop" // the turn ends on a refused call: no answer written
	h.state = "blocked"
	h.w.Sweep()
	if got, _ := st.Run(run.ID); got.State != RunRunning || len(h.afters) != 0 {
		t.Fatalf("a pane that asks is not read for denials: %+v %q", got, h.afters)
	}
	h.state = "idle"
	h.w.Sweep()
	if got, _ := st.Run(run.ID); got.State != RunRunning {
		t.Fatal("a turn ending in a call that was not denied is no finish")
	}
	h.denied = transcript.Denials{Calls: []transcript.Denial{{Tool: "Bash", What: "git push"}}, EndsTurn: true}
	was, _ := st.Get(tk.ID)
	h.w.Sweep()
	got, _ := st.Get(tk.ID)
	if got.Run.State != RunFinished || got.Run.Pipeline != "stopped: the agent stopped on calls it was denied without asking" || h.advance != 0 {
		t.Fatalf("the run and its pipeline: %+v, %d advances", got.Run, h.advance)
	}
	if got.StatusID != was.StatusID {
		t.Fatal("a run that stopped moved the card as a finish")
	}
	said := remudaSaid(t, st, tk.ID)
	if len(said) != 2 || !strings.Contains(said[0], "a call that would have waited for your permission") || !strings.Contains(said[0], "It stopped there") || !strings.Contains(said[1], "The pipeline stops") {
		t.Fatalf("remuda's comments: %q", said)
	}
	if len(h.pushed) != 1 || !strings.Contains(h.pushed[0].Title, "claude stopped") || !strings.Contains(h.pushed[0].Body, "1 call denied") {
		t.Fatalf("pushed %+v", h.pushed)
	}
	h.w.Sweep()
	if len(remudaSaid(t, st, tk.ID)) != 2 {
		t.Fatal("reported twice")
	}
}

func TestDeniedCommentFencesPastBackticks(t *testing.T) {
	got := deniedComment(Run{Agent: "claude"}, transcript.Denials{Calls: []transcript.Denial{{Tool: "Bash", What: "echo `date`"}, {Tool: "WebSearch"}}})
	if !strings.Contains(got, "- Bash: `` echo `date` ``") || !strings.Contains(got, "- WebSearch\n") {
		t.Errorf("comment: %q", got)
	}
}

// A stage starts with its agent and model alone, so the project's extra arguments — a permission mode among them —
// never reach it: the comment points at the settings files instead.
func TestDeniedCommentOfAStageNamesWhatReachesIt(t *testing.T) {
	calls := transcript.Denials{Calls: []transcript.Denial{{Tool: "Bash", What: "git push"}}}
	if first := deniedComment(Run{Agent: "claude"}, calls); !strings.Contains(first, "permission mode in the project's agent settings") {
		t.Errorf("the first run: %q", first)
	}
	stage := deniedComment(Run{Agent: "claude", Stage: 2}, calls)
	if strings.Contains(stage, "permission mode") || !strings.Contains(stage, ".claude/settings.json") {
		t.Errorf("a stage: %q", stage)
	}
}
