package sleep

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/core/coretest"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/restart"
	"github.com/dev1antdev/remuda/internal/sessions"
)

// standInClaude puts a `claude` on the PATH the next core starts with: it says what it was started with, reports
// itself through OSC 7501 as Claude Code does, and leaves on /exit.
func standInClaude(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
printf 'args: %s\n' "$*"
printf '\033]7501;state=idle:app=claude-code\033\\'
while IFS= read -r line; do
  [ "$line" = /exit ] && exit 0
  printf 'got: %s\n' "$line"
done
`
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// coreSnapshot asks the adapter until its snapshot says what want looks for.
func coreSnapshot(t *testing.T, adapter *core.Adapter, want func(*herdr.Snapshot) bool) *herdr.Snapshot {
	t.Helper()
	for start := time.Now(); time.Since(start) < 15*time.Second; time.Sleep(100 * time.Millisecond) {
		if snap, err := adapter.SessionSnapshot(); err == nil && want(snap) {
			return snap
		}
	}
	snap, err := adapter.SessionSnapshot()
	t.Fatalf("the snapshot never came: %+v %v", snap, err)
	return nil
}

// A workspace on remuda-core sleeps and wakes through the same leave and start as on herdr: Claude Code is asked to
// /exit, its shell stands, and the wake starts it there again on its session.
func TestAWorkspaceOnRemudaCoreSleepsAndWakesOnItsSession(t *testing.T) {
	poll := restart.LeavePoll
	restart.LeavePoll = 300 * time.Millisecond // the core looks at a terminal's foreground about twice a second
	t.Cleanup(func() { restart.LeavePoll = poll })
	standInClaude(t)
	client := coretest.Server(t).Client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if hello, err := client.Hello(ctx); err != nil || !hello.Has("send_foreground") {
		t.Skipf("the core does not check the agent as it types (%v): a prompt waits for remuda-core 0.2.1", err)
	}
	adapter := core.NewAdapter(client, nil, core.AdapterHooks{SessionOf: func(agent string, pid int, cwd string) *herdr.AgentSession {
		return &herdr.AgentSession{Kind: "id", Value: sidA}
	}})
	t.Cleanup(func() { adapter.Close() })
	coreSnapshot(t, adapter, func(*herdr.Snapshot) bool { return true })
	folder, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	made, err := adapter.WorkspaceCreate(folder, "api")
	if err != nil {
		t.Fatal(err)
	}
	space, pane := made.Workspace.WorkspaceID, made.RootPane.PaneID
	if err := sessions.StartAgent(adapter, pane, "claude"); err != nil {
		t.Fatal(err)
	}
	snap := coreSnapshot(t, adapter, func(s *herdr.Snapshot) bool {
		p := s.Pane(pane)
		return p != nil && p.Agent == "claude" && p.AgentStatus == herdr.StatusIdle
	})

	w, _, _, _ := Plan(snap, space)
	if len(w.Agents) != 1 || w.Agents[0].Session != sidA || w.Agents[0].Dir != folder {
		t.Fatalf("plan %+v", w)
	}
	s := &Service{Store: Open("")}
	s.Store.put("local", space, w)
	var log []string
	say := func(line string) { log = append(log, line) }
	if err := s.sleepAll(adapter, "local", space, w, say); err != nil {
		t.Fatalf("sleep: %v\n%s", err, strings.Join(log, "\n"))
	}
	if p := coreSnapshot(t, adapter, func(*herdr.Snapshot) bool { return true }).Pane(pane); p == nil || p.Agent != "" {
		t.Fatalf("asleep, the pane is %+v", p)
	}
	if !s.Store.Holds("local", space, pane) {
		t.Fatal("the agent that left is no sleeper")
	}

	if err := s.wakeAll(adapter, "local", space, w, noChecks, say); err != nil {
		t.Fatalf("wake: %v\n%s", err, strings.Join(log, "\n"))
	}
	coreSnapshot(t, adapter, func(s *herdr.Snapshot) bool {
		p := s.Pane(pane)
		return p != nil && p.Agent == "claude"
	})
	for start := time.Now(); ; time.Sleep(100 * time.Millisecond) {
		screen, _ := adapter.PaneRead(pane)
		if strings.Contains(screen, "args: --resume "+sidA) {
			break
		}
		if time.Since(start) > 10*time.Second {
			t.Fatalf("Claude Code did not come back on its session:\n%s", screen)
		}
	}
	if _, ok := s.Store.Of("local", space); ok {
		t.Fatal("the workspace is still asleep")
	}
}
