package undo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/shell"
)

func snapshot(dir string, panes ...herdr.PaneInfo) *herdr.Snapshot {
	snap := &herdr.Snapshot{Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1", Label: "app"}}}
	for _, pane := range panes {
		pane.WorkspaceID, pane.CWD = "w1", dir
		snap.Panes = append(snap.Panes, pane)
	}
	return snap
}

func agentPane(id, status string) herdr.PaneInfo {
	return herdr.PaneInfo{PaneID: id, Agent: "claude", DisplayAgent: "Claude Code", AgentStatus: status}
}

// waitFor polls until done answers true, failing the test after a few seconds.
func waitFor(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("never: %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func newTracker(t *testing.T) (*Tracker, *Store) {
	store := OpenStore(t.TempDir())
	tracker := NewTracker(store, Inputs{Host: func(string) (shell.Host, error) { return shell.Local{Name: "local"}, nil }})
	return tracker, store
}

// idle waits until the tracker's readers have nothing left to do.
func idle(t *testing.T, tracker *Tracker) {
	waitFor(t, "the readers to finish", func() bool {
		tracker.mu.Lock()
		defer tracker.mu.Unlock()
		for _, running := range tracker.running {
			if running {
				return false
			}
		}
		return true
	})
}

func TestTrackerKeepsATurnFromWorkToRest(t *testing.T) {
	dir := repo(t, map[string]string{"a.txt": "one\n"})
	tracker, store := newTracker(t)
	at := time.Now()
	tracker.Observe("local", snapshot(dir, agentPane("p1", herdr.StatusWorking)), at, false)
	idle(t, tracker)
	if len(store.LoadOpen()) != 1 {
		t.Fatal("the start is not kept on the disk while the turn goes")
	}
	write(t, dir, "a.txt", "agent\n")
	tracker.Observe("local", snapshot(dir, agentPane("p1", herdr.StatusDone)), at.Add(10*time.Second), false)
	if !tracker.Going("local", dir) {
		t.Error("a turn that only just rested is not going: an undo would land in it")
	}
	tracker.Observe("local", snapshot(dir, agentPane("p1", herdr.StatusDone)), at.Add(10*time.Second+Settle), false)
	idle(t, tracker)
	tracker.Observe("local", snapshot(dir, agentPane("p1", herdr.StatusDone)), at.Add(10*time.Second+Grace), false)
	idle(t, tracker)
	turns := store.List("local", dir)
	if len(turns) != 1 || len(turns[0].Files) != 1 || turns[0].Files[0].Path != "a.txt" || turns[0].Agent != "Claude Code" || turns[0].Pane != "p1" {
		t.Fatalf("turns %+v", turns)
	}
	if len(store.LoadOpen()) != 0 || tracker.Going("local", dir) {
		t.Error("the finished turn's start stays on the disk")
	}
}

func TestTrackerTreatsABlinkAsTheSameTurn(t *testing.T) {
	dir := repo(t, map[string]string{"a.txt": "one\n"})
	tracker, store := newTracker(t)
	at := time.Now()
	tracker.Observe("local", snapshot(dir, agentPane("p1", herdr.StatusWorking)), at, false)
	idle(t, tracker)
	write(t, dir, "a.txt", "first half\n")
	tracker.Observe("local", snapshot(dir, agentPane("p1", herdr.StatusIdle)), at.Add(time.Second), false)
	tracker.Observe("local", snapshot(dir, agentPane("p1", herdr.StatusIdle)), at.Add(time.Second+Settle), false)
	idle(t, tracker)
	tracker.Observe("local", snapshot(dir, agentPane("p1", herdr.StatusWorking)), at.Add(2*time.Second+Settle), false)
	write(t, dir, "b.txt", "second half\n")
	rest := at.Add(10 * time.Second)
	tracker.Observe("local", snapshot(dir, agentPane("p1", herdr.StatusDone)), rest, false)
	tracker.Observe("local", snapshot(dir, agentPane("p1", herdr.StatusDone)), rest.Add(Settle), false)
	idle(t, tracker)
	tracker.Observe("local", snapshot(dir, agentPane("p1", herdr.StatusDone)), rest.Add(Grace), false)
	idle(t, tracker)
	turns := store.List("local", dir)
	if len(turns) != 1 || len(turns[0].Files) != 2 {
		t.Fatalf("a blink split the turn: %+v", turns)
	}
}

func TestTrackerLeavesAFolderOutsideGitAlone(t *testing.T) {
	needGit(t)
	dir := t.TempDir()
	tracker, store := newTracker(t)
	at := time.Now()
	tracker.Observe("local", snapshot(dir, agentPane("p1", herdr.StatusWorking)), at, false)
	idle(t, tracker)
	if err := os.WriteFile(filepath.Join(dir, "x"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	tracker.Observe("local", snapshot(dir, agentPane("p1", herdr.StatusDone)), at.Add(time.Second), false)
	tracker.Observe("local", snapshot(dir, agentPane("p1", herdr.StatusDone)), at.Add(time.Second+Grace), false)
	idle(t, tracker)
	if len(store.List("local", dir)) != 0 || len(store.LoadOpen()) != 0 || tracker.Going("local", dir) {
		t.Fatal("a folder outside git got a turn")
	}
}

func TestTrackerMarksTwoAgentsInOneFolderShared(t *testing.T) {
	dir := repo(t, map[string]string{"a.txt": "one\n"})
	tracker, store := newTracker(t)
	at := time.Now()
	tracker.Observe("local", snapshot(dir, agentPane("p1", herdr.StatusWorking), agentPane("p2", herdr.StatusWorking)), at, false)
	idle(t, tracker)
	write(t, dir, "a.txt", "two agents\n")
	for _, step := range []time.Duration{time.Second, time.Second + Settle, time.Second + Grace} {
		tracker.Observe("local", snapshot(dir, agentPane("p1", herdr.StatusDone), agentPane("p2", herdr.StatusDone)), at.Add(step), false)
		idle(t, tracker)
	}
	turns := store.List("local", dir)
	if len(turns) != 2 || !turns[0].Shared || !turns[1].Shared {
		t.Fatalf("turns %+v", turns)
	}
}

func TestTrackerPicksAGoingTurnUpAfterARestart(t *testing.T) {
	dir := repo(t, map[string]string{"a.txt": "one\n"})
	stateDir := t.TempDir()
	store := OpenStore(stateDir)
	host := func(string) (shell.Host, error) { return shell.Local{Name: "local"}, nil }
	first := NewTracker(store, Inputs{Host: host})
	at := time.Now()
	first.Observe("local", snapshot(dir, agentPane("p1", herdr.StatusWorking)), at, false)
	idle(t, first)
	first.Close()
	write(t, dir, "a.txt", "written while remuda restarted\n")
	again := OpenStore(stateDir)
	second := NewTracker(again, Inputs{Host: host})
	second.Observe("local", snapshot(dir, agentPane("p1", herdr.StatusWorking)), at.Add(time.Minute), true)
	for _, step := range []time.Duration{2 * time.Minute, 2*time.Minute + Settle, 2*time.Minute + Grace} {
		second.Observe("local", snapshot(dir, agentPane("p1", herdr.StatusDone)), at.Add(step), false)
		idle(t, second)
	}
	turns := again.List("local", dir)
	if len(turns) != 1 || !turns[0].Started.Equal(at) {
		t.Fatalf("the turn did not carry on from its start: %+v", turns)
	}
	if !strings.HasPrefix(turns[0].ID, at.UTC().Format("20060102T150405Z")) {
		t.Errorf("id %s", turns[0].ID)
	}
}

func TestStoreKeepsItsBounds(t *testing.T) {
	store := OpenStore(t.TempDir())
	saved, savedTotal := KeepPerFolder, MaxTotal
	t.Cleanup(func() { KeepPerFolder, MaxTotal = saved, savedTotal })
	KeepPerFolder, MaxTotal = 3, 25
	at := time.Now()
	for i := range 5 {
		turn := Turn{ID: NewID(at.Add(time.Duration(i) * time.Second)), Machine: "local", Dir: "/a", Ended: at.Add(time.Duration(i) * time.Second), Size: 1}
		if err := store.Add(turn, "s", ""); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(store.List("local", "/a")); got != 3 {
		t.Fatalf("a folder keeps %d turns, want 3", got)
	}
	big := Turn{ID: NewID(at.Add(time.Minute)), Machine: "local", Dir: "/b", Ended: at.Add(time.Minute), Size: 24}
	if err := store.Add(big, strings.Repeat("x", 24), ""); err != nil {
		t.Fatal(err)
	}
	if got := len(store.List("local", "/a")); got != 1 {
		t.Fatalf("past the total the oldest go first: %d left in /a", got)
	}
	reopened := OpenStore(filepath.Dir(store.dir))
	if len(reopened.List("local", "/a")) != 1 || len(reopened.List("local", "/b")) != 1 {
		t.Fatal("the store does not read back what it kept")
	}
	if info, err := os.Stat(filepath.Join(store.turnsDir(), big.ID+".start.patch")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("a patch is not private: %v %v", info, err)
	}
}
