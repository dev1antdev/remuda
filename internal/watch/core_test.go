package watch

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core/coretest"
	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
)

// standInClaude puts a `claude` on the next core's PATH: it says it is Claude Code at rest, and reports itself as the
// lines work, ask and rest reach it — the statuses a core reads off OSC 7501.
func standInClaude(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
say() { printf '\033]7501;state=%s:app=claude-code\033\\' "$1"; }
say idle
while IFS= read -r line; do
  case "$line" in
  work) say working ;;
  ask) say blocked ;;
  rest) say idle ;;
  esac
done
`
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestTheWatcherHearsARemudaCore: the watcher on a core of the test's own, its poll slowed past the test's length, so
// only the stream can tell it: a pane made joins the stream, and its agent's statuses reach the bus at once.
func TestTheWatcherHearsARemudaCore(t *testing.T) {
	standInClaude(t)
	coretest.Server(t).Use(t)
	saved := []time.Duration{localEvery, localStreamingEvery}
	localEvery, localStreamingEvery = time.Minute, time.Minute
	t.Cleanup(func() { localEvery, localStreamingEvery = saved[0], saved[1] })
	fleet := machines.NewManager(t.TempDir(), nil, false)
	t.Cleanup(fleet.Close)
	mc, err := fleet.Get(machines.LocalID)
	if err != nil {
		t.Fatal(err)
	}
	bus := events.NewBus()
	var mu sync.Mutex
	var moves []string
	events.Subscribe(bus, func(e events.PaneStatus) {
		mu.Lock()
		moves = append(moves, e.Pane.Pane+" "+e.To)
		mu.Unlock()
	})
	heard := func(want string) bool {
		mu.Lock()
		defer mu.Unlock()
		for _, m := range moves {
			if m == want {
				return true
			}
		}
		return false
	}
	watcher := New(fleet, bus)
	done := make(chan struct{})
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		watcher.run(mc, done)
	}()
	defer func() {
		close(done)
		<-ended
	}()
	waitFor(t, "the stream", func() bool {
		watcher.mu.Lock()
		defer watcher.mu.Unlock()
		return watcher.stateFor(machines.LocalID).streaming.Load()
	})

	client, _, err := mc.Ready()
	if err != nil {
		t.Fatal(err)
	}
	made, err := client.WorkspaceCreate(t.TempDir(), "watched")
	if err != nil {
		t.Fatal(err)
	}
	defer client.WorkspaceClose(made.Workspace.WorkspaceID)
	pane := made.RootPane.PaneID
	waitFor(t, "the new pane in a snapshot", func() bool {
		snap, _ := watcher.Snapshot(machines.LocalID)
		return snap != nil && snap.Pane(pane) != nil
	})
	var started error
	for start := time.Now(); time.Since(start) < 10*time.Second; time.Sleep(200 * time.Millisecond) {
		if started = client.AgentStart(pane, "claude"); !herdr.IsCode(started, "agent_pane_busy") {
			break
		}
	}
	if started != nil {
		t.Fatal(started)
	}
	waitFor(t, "the agent's first status", func() bool { return heard(pane + " " + herdr.StatusIdle) })
	waitFor(t, "the agent at rest in a snapshot", func() bool {
		snap, _ := watcher.Snapshot(machines.LocalID)
		if snap == nil {
			return false
		}
		p := snap.Pane(pane)
		return p != nil && p.Agent == "claude" && p.AgentStatus == herdr.StatusIdle
	})
	for _, c := range []struct {
		line string
		to   string
	}{
		{"work", herdr.StatusWorking},
		{"ask", herdr.StatusBlocked},
		{"rest", herdr.StatusIdle},
	} {
		mu.Lock()
		moves = nil
		mu.Unlock()
		asked := time.Now()
		keys := make([]string, 0, len(c.line)+1)
		for _, key := range c.line {
			keys = append(keys, string(key))
		}
		keys = append(keys, "enter")
		if err := client.PaneSendKeys(pane, keys); err != nil {
			t.Fatal(err)
		}
		want := pane + " " + c.to
		waitFor(t, want, func() bool { return heard(want) })
		if took := time.Since(asked); took > 2*time.Second {
			t.Fatalf("%s took %s with the poll a minute apart", want, took)
		}
	}
}
