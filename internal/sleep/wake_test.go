package sleep

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// asleep is a workspace's record as a sleep left it, its agents in these panes.
func asleep(agents ...Agent) Workspace {
	return Workspace{Name: "api", Dir: "/work/api", At: time.Now(), Agents: agents}
}

func sleeper(pane, tab, kind, session string) Agent {
	return Agent{Pane: pane, Tab: tab, Kind: kind, Session: session, Dir: "/work/api"}
}

func noChecks(Agent) error { return nil }

func TestWakeResumesEachAgentInItsShellAndSaysWhatStaysAsleep(t *testing.T) {
	w := asleep(
		sleeper("w1:p1", "w1:t1", "claude", sidA), // its shell is free: resumed there
		sleeper("w1:p2", "w1:t1", "codex", sidB),  // something runs in its shell
		sleeper("w1:p3", "w1:t1", "gemini", sidC), // its shell moved to another folder
		sleeper("w1:p4", "w1:t1", "kimi", sidA),   // the person started Claude Code there by hand
		sleeper("w1:p5", "w1:t1", "claude", sidB), // back already, resumed by hand
		sleeper("w1:p6", "w1:t1", "claude", sidC), // its session file is gone: it can never come back
	)
	f := newFake(
		shellPane("w1:p1", "w1:t1", "/work/api"),
		shellPane("w1:p2", "w1:t1", "/work/api"),
		shellPane("w1:p3", "w1:t1", "/home/me"),
		agentPane("w1:p4", "w1:t1", "claude", sidC, herdr.StatusIdle, "/work/api"),
		agentPane("w1:p5", "w1:t1", "claude", sidB, herdr.StatusWorking, "/work/api"),
		shellPane("w1:p6", "w1:t1", "/work/api"),
	)
	f.busyShell["w1:p2"] = true
	s := &Service{Store: Open("")}
	s.Store.put("local", "w1", w)
	checks := func(a Agent) error {
		if a.Pane == "w1:p6" {
			return errors.New("its session file is gone — nothing to resume")
		}
		return nil
	}
	var log []string
	err := s.wakeAll(f, "local", "w1", w, checks, func(line string) { log = append(log, line) })
	if err == nil || !strings.HasPrefix(err.Error(), "api is still partly asleep · not restarted (busy or gone): ") {
		t.Fatalf("wake: %v", err)
	}
	for _, want := range []string{"Codex in w1:p2 (something runs in its shell)", "Gemini CLI in w1:p3 (its shell is in another folder now)", "Kimi Code in w1:p4 (Claude Code runs in its pane now)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the partial wake does not name %q: %v", want, err)
		}
	}
	if len(f.started) != 1 || f.started[0] != "w1:p1 claude --resume "+sidA {
		t.Fatalf("started: %v", f.started)
	}
	record, ok := s.Store.Of("local", "w1")
	if !ok {
		t.Fatal("still partly asleep")
	}
	var panes []string
	for _, a := range record.Agents {
		panes = append(panes, a.Pane)
	}
	if strings.Join(panes, " ") != "w1:p2 w1:p3 w1:p4" {
		t.Fatalf("still asleep: %v", panes)
	}
	if !strings.Contains(strings.Join(log, "\n"), "forgotten, since they cannot come back: Claude Code in w1:p6 (its session file is gone") {
		t.Fatalf("the agent that can never come back is not said:\n%s", strings.Join(log, "\n"))
	}
}

func TestAWorkspaceWakesWholeAndIsAwake(t *testing.T) {
	w := asleep(sleeper("w1:p1", "w1:t1", "claude", sidA), sleeper("w1:p2", "w1:t1", "codex", sidB))
	f := newFake(shellPane("w1:p1", "w1:t1", "/work/api"), shellPane("w1:p2", "w1:t1", "/work/api"))
	s := &Service{Store: Open("")}
	s.Store.put("local", "w1", w)
	var log []string
	if err := s.wakeAll(f, "local", "w1", w, noChecks, func(line string) { log = append(log, line) }); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.started, []string{"w1:p1 claude --resume " + sidA, "w1:p2 codex resume " + sidB}) {
		t.Fatalf("started: %v", f.started)
	}
	if s.Store.Status("local", "w1") != nil {
		t.Fatal("awake: the record is gone")
	}
	if !strings.Contains(strings.Join(log, "\n"), "api is awake: 2 agents resumed") {
		t.Fatalf("log:\n%s", strings.Join(log, "\n"))
	}
}

func TestAGonePaneWakesInANewTabOfItsWorkspace(t *testing.T) {
	w := asleep(Agent{Pane: "w1:p2", Tab: "w1:t2", TabName: "review", Kind: "claude", Session: sidA, Dir: "/work/api"})
	f := newFake(shellPane("w1:p1", "w1:t1", "/work/api"))
	s := &Service{Store: Open("")}
	s.Store.put("local", "w1", w)
	if err := s.wakeAll(f, "local", "w1", w, noChecks, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.tabs, []string{"w1 review"}) || !slices.Equal(f.started, []string{"w1:p7 claude --resume " + sidA}) {
		t.Fatalf("tabs %v, started %v", f.tabs, f.started)
	}
}

func TestAStartThatFailsInANewTabIsTriedInThatTabNextTime(t *testing.T) {
	w := asleep(sleeper("w1:p2", "w1:t1", "claude", sidA))
	f := newFake(shellPane("w1:p1", "w1:t1", "/work/api"))
	f.refuse["w1:p7"] = &herdr.Error{Code: "agent_start_failed", Message: "no such binary"}
	s := &Service{Store: Open("")}
	s.Store.put("local", "w1", w)
	err := s.wakeAll(f, "local", "w1", w, noChecks, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "it did not start") {
		t.Fatalf("wake: %v", err)
	}
	record, _ := s.Store.Of("local", "w1")
	if len(record.Agents) != 1 || record.Agents[0].Pane != "w1:p7" || record.Agents[0].Tab != "w1:t7" {
		t.Fatalf("the agent waits in the tab made for it: %+v", record)
	}
	delete(f.refuse, "w1:p7")
	if err := s.wakeAll(f, "local", "w1", record, noChecks, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if len(f.tabs) != 1 || len(f.started) != 1 || f.started[0] != "w1:p7 claude --resume "+sidA {
		t.Fatalf("no second tab, the start in the first: tabs %v, started %v", f.tabs, f.started)
	}
}
