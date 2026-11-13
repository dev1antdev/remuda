package sleep

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestPlanRecordsEveryAgentWithASessionAndLeavesTheRest(t *testing.T) {
	f := newFake(
		agentPane("w1:p1", "w1:t1", "claude", sidA, herdr.StatusIdle, "/work/api"),
		agentPane("w1:p2", "w1:t2", "codex", sidB, herdr.StatusWorking, "/work/api"),
		agentPane("w1:p3", "w1:t1", "gemini", sidC, herdr.StatusBlocked, ""),
		agentPane("w1:p4", "w1:t1", "claude", "", herdr.StatusIdle, "/work/api"), // no session: nothing to resume
		agentPane("w1:p5", "w1:t1", "pi", sidA, herdr.StatusIdle, "/work/api"),   // a kind remuda cannot bring back
		shellPane("w1:p6", "w1:t1", "/work/api"),
		herdr.PaneInfo{PaneID: "w2:p1", WorkspaceID: "w2", Agent: "claude", AgentStatus: "idle", AgentSession: &herdr.AgentSession{Kind: "id", Value: sidA}},
	)
	snap, _ := f.SessionSnapshot()
	w, busy, left, err := Plan(snap, "w1")
	if err != nil {
		t.Fatal(err)
	}
	if w.Name != "api" || w.Dir != "/work/api" || len(w.Agents) != 3 {
		t.Fatalf("plan: %+v", w)
	}
	if got := w.Agents[1]; got.Pane != "w1:p2" || got.Kind != "codex" || got.Session != sidB || got.TabName != "review" || got.Tab != "w1:t2" {
		t.Fatalf("the Codex in the named tab: %+v", got)
	}
	if w.Agents[0].TabName != "" {
		t.Fatalf("a tab herdr numbers has no name of the person's: %q", w.Agents[0].TabName)
	}
	if w.Agents[2].Dir != "/work/api" {
		t.Fatalf("a pane that names no folder lives in its workspace's: %q", w.Agents[2].Dir)
	}
	if len(busy) != 2 || busy[0].Pane != "w1:p2" || busy[0].Status != "working" || busy[1].Status != "blocked" {
		t.Fatalf("busy: %+v", busy)
	}
	if len(left) != 2 || !strings.Contains(left[0], "w1:p4") || !strings.Contains(left[1], "w1:p5") {
		t.Fatalf("left running: %v", left)
	}
	if _, _, _, err := Plan(snap, "w9"); !errors.Is(err, ErrNoWorkspace) {
		t.Fatalf("a workspace that is not there: %v", err)
	}
	if msg := (&BusyError{Workspace: "api", Busy: busy}).Error(); msg != "Sleep api? Still busy: Codex is working, Gemini CLI is asking you something. Sleeping interrupts them. Waking resumes their conversations where they stopped." {
		t.Fatalf("the busy refusal: %s", msg)
	}
}

func TestSleepAsksEachAgentToLeaveAndKeepsTheOnesThatWouldNotAwake(t *testing.T) {
	quick(t)
	f := newFake(
		agentPane("w1:p1", "w1:t1", "claude", sidA, herdr.StatusIdle, "/work/api"),
		agentPane("w1:p2", "w1:t1", "codex", sidB, herdr.StatusWorking, "/work/api"),
		agentPane("w1:p3", "w1:t1", "kimi", sidC, herdr.StatusIdle, "/work/api"),
	)
	f.stubborn["w1:p3"] = true
	snap, _ := f.SessionSnapshot()
	w, _, _, _ := Plan(snap, "w1")
	store := Open("")
	var typed []string
	s := &Service{Store: store, Typed: func(machineID, paneID, text string, keys bool) {
		typed = append(typed, paneID+" "+text)
	}}
	store.put("local", "w1", w)
	var log []string
	err := s.sleepAll(f, "local", "w1", w, func(line string) { log = append(log, line) })
	if err == nil || !strings.Contains(err.Error(), "api is asleep, but 1 agent did not stop: Kimi Code in w1:p3") {
		t.Fatalf("sleep: %v", err)
	}
	for _, want := range []string{"w1:p1 /exit", "w1:p2 esc", "w1:p2 /quit", "w1:p3 /exit", "w1:p3 ctrl+c+ctrl+c"} {
		if !slices.Contains(f.typed, want) {
			t.Errorf("herdr was not sent %q: %v", want, f.typed)
		}
	}
	if slices.Index(f.typed, "w1:p2 esc") > slices.Index(f.typed, "w1:p2 /quit") {
		t.Errorf("Codex was asked to quit before it was interrupted: %v", f.typed)
	}
	if !slices.Contains(typed, "w1:p2 Esc") || !slices.Contains(typed, "w1:p1 /exit") {
		t.Errorf("the activity did not hear what was typed: %v", typed)
	}
	if p := f.pane("w1:p1"); p == nil || p.Agent != "" {
		t.Fatalf("the shell stands where Claude Code was: %+v", p)
	}
	record, ok := store.Of("local", "w1")
	if !ok || len(record.Agents) != 2 || record.Agents[0].Pane != "w1:p1" || record.Agents[1].Pane != "w1:p2" {
		t.Fatalf("asleep: %+v", record)
	}
	if store.Holds("local", "w1", "w1:p3") || !store.Holds("local", "w1", "w1:p2") {
		t.Fatal("the agent that runs on is no sleeper")
	}
	text := strings.Join(log, "\n")
	for _, want := range []string{"Esc — it was busy", "✓ asleep, session 9b1c2d3e kept to resume", "Kimi Code did not leave"} {
		if !strings.Contains(text, want) {
			t.Errorf("the log lacks %q:\n%s", want, text)
		}
	}
}

func TestASleepNobodyLeftLeavesTheWorkspaceAwake(t *testing.T) {
	quick(t)
	f := newFake(agentPane("w1:p1", "w1:t1", "claude", sidA, herdr.StatusIdle, "/work/api"))
	f.stubborn["w1:p1"] = true
	snap, _ := f.SessionSnapshot()
	w, _, _, _ := Plan(snap, "w1")
	s := &Service{Store: Open("")}
	s.Store.put("local", "w1", w)
	err := s.sleepAll(f, "local", "w1", w, func(string) {})
	if err == nil || !strings.HasPrefix(err.Error(), "api did not go to sleep") {
		t.Fatalf("sleep: %v", err)
	}
	if s.Store.Status("local", "w1") != nil {
		t.Fatal("a workspace whose agents all run on is awake")
	}
}

func TestSleepTakesAnotherConversationAndAGonePaneAsTheyAre(t *testing.T) {
	quick(t)
	f := newFake(agentPane("w1:p1", "w1:t1", "claude", sidA, herdr.StatusIdle, "/work/api"), agentPane("w1:p2", "w1:t1", "claude", sidB, herdr.StatusIdle, "/work/api"))
	snap, _ := f.SessionSnapshot()
	w, _, _, _ := Plan(snap, "w1")
	// between the plan and the job the person resumed another conversation in one pane and closed the other
	f.snap.Panes = []herdr.PaneInfo{agentPane("w1:p1", "w1:t1", "claude", sidC, herdr.StatusIdle, "/work/api")}
	s := &Service{Store: Open("")}
	s.Store.put("local", "w1", w)
	err := s.sleepAll(f, "local", "w1", w, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "another conversation runs there now") {
		t.Fatalf("sleep: %v", err)
	}
	if len(f.typed) != 0 {
		t.Fatalf("nothing is typed into another conversation: %v", f.typed)
	}
	record, _ := s.Store.Of("local", "w1")
	if len(record.Agents) != 1 || record.Agents[0].Pane != "w1:p2" {
		t.Fatalf("the gone pane sleeps on, a wake gives it a tab: %+v", record)
	}
}
