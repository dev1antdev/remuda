package core_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/core/coretest"
	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/jobs"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/restart"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/sleep"
	"github.com/dev1antdev/remuda/internal/transcript"
)

// The sleep and the update restart go through the whole machine on a throwaway core, here rather than in their own
// packages: the gate that holds prompts back from a core older than 0.2.1, which CI runs, is lifted only in this one.

// The conversation the stand-in starts on, and the one it moves to with /resume in the same process.
const (
	lifeSession  = "9b1c2d3e-4f50-4a6b-8c7d-0e1f2a3b4c5d"
	movedSession = "0a1b2c3d-4e5f-4061-8273-94a5b6c7d8e9"
)

// claudeOnCore is this machine on a throwaway core, as a remuda started with -core reaches it, with a stand-in Claude
// Code on its session in a workspace of its own: the machine, the workspace and the pane, once the session is known.
func claudeOnCore(t *testing.T) (*machines.Manager, *machines.Machine, string, string) {
	t.Helper()
	core.LiftPromptGate(t)
	coretest.StandInClaude(t, lifeSession)
	coretest.Server(t).Use(t)
	fleet := machines.NewManager(t.TempDir(), nil, false)
	t.Cleanup(fleet.Close)
	mc, err := fleet.Get(machines.LocalID)
	if err != nil {
		t.Fatal(err)
	}
	client, _, err := mc.Ready()
	if err != nil {
		t.Fatal(err)
	}
	folder, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range []string{lifeSession, movedSession} {
		conversation := filepath.Join(mc.Home(), ".claude", "projects", transcript.EncodePath(folder), session+".jsonl")
		if err := os.MkdirAll(filepath.Dir(conversation), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(conversation, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	made, err := client.WorkspaceCreate(folder, "api")
	if err != nil {
		t.Fatal(err)
	}
	pane := made.RootPane.PaneID
	if err := sessions.StartAgent(client, pane, "claude"); err != nil {
		t.Fatal(err)
	}
	machineSays(t, client, func(s *herdr.Snapshot) bool {
		p := s.Pane(pane)
		return p != nil && p.Agent == "claude" && p.AgentStatus == herdr.StatusIdle && restart.SessionOf(p) == lifeSession
	})
	return fleet, mc, made.Workspace.WorkspaceID, pane
}

// machineSays asks the machine until its snapshot says what want looks for.
func machineSays(t *testing.T, client herdr.API, want func(*herdr.Snapshot) bool) {
	t.Helper()
	for start := time.Now(); time.Since(start) < 20*time.Second; time.Sleep(100 * time.Millisecond) {
		if snap, err := client.SessionSnapshot(); err == nil && want(snap) {
			return
		}
	}
	snap, err := client.SessionSnapshot()
	t.Fatalf("the machine never said it: %+v %v", snap, err)
}

// screenHas waits for the pane's screen to show the text.
func screenHas(t *testing.T, client herdr.API, pane, text string) {
	t.Helper()
	for start := time.Now(); ; time.Sleep(100 * time.Millisecond) {
		screen, _ := client.PaneRead(pane)
		if strings.Contains(screen, text) {
			return
		}
		if time.Since(start) > 20*time.Second {
			t.Fatalf("the screen never said %q:\n%s", text, screen)
		}
	}
}

// finished waits for a job's end and fails the test on its error.
func finished(t *testing.T, runner *jobs.Runner, id string) jobs.Job {
	t.Helper()
	for start := time.Now(); time.Since(start) < 60*time.Second; time.Sleep(100 * time.Millisecond) {
		if job, ok := runner.Get(id); ok && job.Done {
			if !job.OK {
				t.Fatalf("%s: %s\n%s", job.Title, job.Error, job.Output)
			}
			return job
		}
	}
	t.Fatalf("job %s never ended", id)
	return jobs.Job{}
}

// movedOn has the agent /resume another conversation in the same process, as a person may right before a sleep or an
// update: the session read off the process half a minute ago is no longer the one it runs.
func movedOn(t *testing.T, client herdr.API, pane string) {
	t.Helper()
	if err := client.AgentPrompt(pane, "/resume "+movedSession); err != nil {
		t.Fatal(err)
	}
	screenHas(t, client, pane, "resumed: "+movedSession)
}

// A workspace sleeps and wakes on the conversation its agent runs at the press, though a /resume moved it there a
// moment before.
func TestAWorkspaceOnACoreSleepsAndWakesOnItsSession(t *testing.T) {
	_, mc, space, pane := claudeOnCore(t)
	client, _, _ := mc.Ready()
	s := &sleep.Service{Store: sleep.Open(""), Jobs: jobs.New(events.NewBus())}
	movedOn(t, client, pane)

	started, err := s.Sleep(mc, space, false)
	if err != nil {
		t.Fatal(err)
	}
	finished(t, s.Jobs, started.Job)
	machineSays(t, client, func(snap *herdr.Snapshot) bool { p := snap.Pane(pane); return p != nil && p.Agent == "" })
	if !s.Store.Holds(mc.ID(), space, pane) {
		t.Fatal("the agent that left is no sleeper")
	}

	started, err = s.Wake(mc, space)
	if err != nil {
		t.Fatal(err)
	}
	finished(t, s.Jobs, started.Job)
	screenHas(t, client, pane, "args: --resume "+movedSession)
	if _, asleep := s.Store.Of(mc.ID(), space); asleep {
		t.Fatal("the workspace is still asleep")
	}
}

// An agent update restarts the chat on the conversation it runs at that moment: the in-place /restart first, then,
// Claude Code saying it has none, its own exit and a start on the session in the same pane.
func TestAChatOnACoreRestartsOntoAnUpdateOnItsSession(t *testing.T) {
	fleet, mc, _, pane := claudeOnCore(t)
	client, _, _ := mc.Ready()
	movedOn(t, client, pane)
	var log []string
	if err := restart.New(fleet).Now(mc, []string{"claude"}, func(line string) { log = append(log, line) }); err != nil {
		t.Fatalf("restart: %v\n%s", err, strings.Join(log, "\n"))
	}
	screenHas(t, client, pane, "args: --resume "+movedSession)
	if said := strings.Join(log, "\n"); !strings.Contains(said, "leaving and resuming instead") {
		t.Fatalf("the restart did not try /restart before leaving:\n%s", said)
	}
}

// Agents that slept in herdr before the move carry on on a throwaway core: a workspace on their folder named as
// herdr's was, each agent of it a tab there, on its own session, and the sleep told each is back.
func TestHerdrsAgentsCarryOnOnACore(t *testing.T) {
	const second = "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"
	coretest.StandInClaude(t, lifeSession)
	coretest.Server(t).Use(t)
	fleet := machines.NewManager(t.TempDir(), nil, false)
	t.Cleanup(fleet.Close)
	mc, err := fleet.Get(machines.LocalID)
	if err != nil {
		t.Fatal(err)
	}
	client, _, err := mc.Ready()
	if err != nil {
		t.Fatal(err)
	}
	folder, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range []string{lifeSession, second} {
		conversation := filepath.Join(mc.Home(), ".claude", "projects", transcript.EncodePath(folder), session+".jsonl")
		if err := os.MkdirAll(filepath.Dir(conversation), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(conversation, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	r := restart.New(fleet)
	r.Serving = func(*machines.Machine) (bool, error) { return false, nil }
	r.Memory = restart.OpenMemory("")
	r.Memory.Core = func() bool { return true }
	slept := map[string]restart.Chat{
		"w1:p1": {Kind: "claude", Session: lifeSession, Dir: folder, Title: "api"},
		"w1:p2": {Kind: "claude", Session: second, Dir: folder, Title: "api"},
	}
	var woken []string
	r.Memory.Sleepers = func(string) map[string]restart.Chat { return slept }
	r.Memory.Woken = func(_, pane string) { woken = append(woken, pane) }
	before, err := client.SessionSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	r.Memory.Observe(events.Snapshot{Machine: mc.ID(), Snap: before, At: time.Now()})
	var log []string
	if err := r.Recover(mc, nil, func(line string) { log = append(log, line) }); err != nil {
		t.Fatalf("recover: %v\n%s", err, strings.Join(log, "\n"))
	}
	snap, err := client.SessionSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Workspaces) != 1 || snap.Workspaces[0].Label != "api" || snap.WorkspaceDir(&snap.Workspaces[0]) != folder || len(snap.Tabs) != 2 {
		t.Fatalf("one workspace named as herdr's, a tab an agent: %+v %+v", snap.Workspaces, snap.Tabs)
	}
	for i, session := range []string{lifeSession, second} {
		pane := snap.Panes[i].PaneID
		for start := time.Now(); ; time.Sleep(100 * time.Millisecond) {
			screen, _ := client.PaneRead(pane)
			if strings.Contains(screen, "args: --resume "+session) {
				break
			}
			if time.Since(start) > 20*time.Second {
				t.Fatalf("Claude Code did not carry on on %s in %s:\n%s", session, pane, screen)
			}
		}
	}
	if len(r.Memory.Stopped(mc.ID())) != 0 || len(woken) != 2 {
		t.Fatalf("still listed: %+v, the sleep told of %v", r.Memory.Stopped(mc.ID()), woken)
	}
}
