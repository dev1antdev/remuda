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

const lifeSession = "9b1c2d3e-4f50-4a6b-8c7d-0e1f2a3b4c5d"

// claudeOnCore is this machine on a throwaway core, as a remuda started with -core reaches it, with a stand-in Claude
// Code on its session in a workspace of its own: the machine, the workspace and the pane, once the session is known.
func claudeOnCore(t *testing.T) (*machines.Manager, *machines.Machine, string, string) {
	t.Helper()
	core.LiftPromptGate(t)
	coretest.StandInClaude(t, lifeSession)
	coretest.Server(t).Use(t)
	fleet := machines.NewCoreManager(t.TempDir(), "", nil, false)
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
	conversation := filepath.Join(mc.Home(), ".claude", "projects", transcript.EncodePath(folder), lifeSession+".jsonl")
	if err := os.MkdirAll(filepath.Dir(conversation), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(conversation, nil, 0o600); err != nil {
		t.Fatal(err)
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

// resumedIn waits for Claude Code to come back in the pane on the session.
func resumedIn(t *testing.T, client herdr.API, pane string) {
	t.Helper()
	for start := time.Now(); ; time.Sleep(100 * time.Millisecond) {
		screen, _ := client.PaneRead(pane)
		if strings.Contains(screen, "args: --resume "+lifeSession) {
			return
		}
		if time.Since(start) > 20*time.Second {
			t.Fatalf("Claude Code did not come back on its session:\n%s", screen)
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

func TestAWorkspaceOnACoreSleepsAndWakesOnItsSession(t *testing.T) {
	_, mc, space, pane := claudeOnCore(t)
	client, _, _ := mc.Ready()
	s := &sleep.Service{Store: sleep.Open(""), Jobs: jobs.New(events.NewBus())}

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
	resumedIn(t, client, pane)
	if _, asleep := s.Store.Of(mc.ID(), space); asleep {
		t.Fatal("the workspace is still asleep")
	}
}

// An agent update restarts the chat on its session: the in-place /restart first, then, Claude Code saying it has none,
// its own exit and a start on the session in the same pane.
func TestAChatOnACoreRestartsOntoAnUpdateOnItsSession(t *testing.T) {
	fleet, mc, _, pane := claudeOnCore(t)
	client, _, _ := mc.Ready()
	var log []string
	if err := restart.New(fleet).Now(mc, []string{"claude"}, func(line string) { log = append(log, line) }); err != nil {
		t.Fatalf("restart: %v\n%s", err, strings.Join(log, "\n"))
	}
	resumedIn(t, client, pane)
	if said := strings.Join(log, "\n"); !strings.Contains(said, "leaving and resuming instead") {
		t.Fatalf("the restart did not try /restart before leaving:\n%s", said)
	}
}

// Agents that ran in herdr before the move carry on on a throwaway core: a workspace on their folder named as herdr's
// was, each agent of it a tab there, on its own session.
func TestHerdrsAgentsCarryOnOnACore(t *testing.T) {
	const second = "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"
	coretest.StandInClaude(t, lifeSession)
	coretest.Server(t).Use(t)
	fleet := machines.NewCoreManager(t.TempDir(), "", nil, false)
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
	r.Memory.Adopt(mc.ID(), map[string]restart.Chat{
		"w1:p1": {Kind: "claude", Session: lifeSession, Dir: folder, Title: "api"},
		"w1:p2": {Kind: "claude", Session: second, Dir: folder, Title: "api"},
	})
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
	if len(r.Memory.Stopped(mc.ID())) != 0 {
		t.Fatalf("still listed: %+v", r.Memory.Stopped(mc.ID()))
	}
}
