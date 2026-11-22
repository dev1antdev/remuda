package sleep

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
)

func TestTheRecordOutlivesARestartAndMergesASecondSleep(t *testing.T) {
	dir := t.TempDir()
	store := Open(dir)
	at := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	store.put("gog", "w1", Workspace{Name: "api", Dir: "/work/api", At: at, Agents: []Agent{sleeper("w1:p1", "w1:t1", "claude", sidA)}})
	// a partly asleep workspace slept again: the new sleepers join, the old stay, the first moment stands
	store.put("gog", "w1", Workspace{Name: "api", Dir: "/work/api", At: at.Add(time.Hour), Agents: []Agent{sleeper("w1:p2", "w1:t1", "codex", sidB)}})
	again := Open(dir)
	w, ok := again.Of("gog", "w1")
	if !ok || len(w.Agents) != 2 || !w.At.Equal(at) {
		t.Fatalf("after a restart: %+v", w)
	}
	if status := again.Status("gog", "w1"); status == nil || status.Agents != 2 || !status.At.Equal(at) {
		t.Fatalf("status: %+v", status)
	}
	if again.Status("gog", "w2") != nil || again.Status("roofr", "w1") != nil {
		t.Fatal("another workspace, or the same id on another machine, is awake")
	}
	if !again.Holds("gog", "w1", "w1:p2") || again.Holds("gog", "w2", "w1:p2") || again.Holds("gog", "w1", "w1:p3") {
		t.Fatal("Holds")
	}
	again.keep("gog", "w1", nil)
	if _, ok := again.Of("gog", "w1"); ok {
		t.Fatal("kept with nobody: awake")
	}
	if err := os.WriteFile(filepath.Join(dir, asleepFile), []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if damaged := Open(dir); damaged.Status("gog", "w1") != nil {
		t.Fatal("a damaged file reads as nothing asleep")
	}
}

func TestAClosedWorkspaceIsForgottenOnlyByAWatchedSnapshot(t *testing.T) {
	store := Open("")
	store.put("gog", "w1", asleep(sleeper("w1:p1", "w1:t1", "claude", sidA)))
	store.put("gog", "w2", asleep(sleeper("w2:p1", "w2:t1", "claude", sidB)))
	only := &herdr.Snapshot{Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w2"}}}
	// the first snapshot after the machine came back decides nothing: herdr may still be restoring
	if store.Observe(events.Snapshot{Machine: "gog", Snap: only, First: true}) || store.Status("gog", "w1") == nil {
		t.Fatal("forgotten on a first snapshot")
	}
	if !store.Observe(events.Snapshot{Machine: "gog", Snap: only}) || store.Status("gog", "w1") != nil || store.Status("gog", "w2") == nil {
		t.Fatal("a workspace closed while watched is forgotten, the other kept")
	}
	store.Forget("gog")
	if store.Status("gog", "w2") != nil {
		t.Fatal("Forget")
	}
}

func TestTheSwitchesStartOffAndAreKept(t *testing.T) {
	dir := t.TempDir()
	store := Open(dir)
	if store.Settings() != (Settings{}) {
		t.Fatalf("off until switched on: %+v", store.Settings())
	}
	if err := store.SetSettings(Settings{WakeOnOpen: true, ResumeAutomatically: true}); err != nil {
		t.Fatal(err)
	}
	if got := Open(dir).Settings(); !got.WakeOnOpen || !got.ResumeAutomatically {
		t.Fatalf("after a restart: %+v", got)
	}
}

// TestAWorkspaceThatSleptInHerdrWaitsForTheCoresResume: once the fleet is on remuda-core, a workspace asleep in herdr
// is kept whatever the core's snapshots show, its agents offered by pane to Resume after restart, each taken off as it
// is dealt with; a workspace put to sleep on the core stays the sleep's as before. The records are kept on disk, so a
// remuda put back on herdr finds its workspaces asleep.
func TestAWorkspaceThatSleptInHerdrWaitsForTheCoresResume(t *testing.T) {
	dir := t.TempDir()
	inHerdr := Open(dir)
	inHerdr.put("gog", "w1", Workspace{Name: "api", Dir: "/work/api", Agents: []Agent{sleeper("w1:p1", "w1:t1", "claude", sidA), sleeper("w1:p2", "w1:t1", "codex", sidB)}})

	store := Open(dir)
	store.Core = func() bool { return true }
	store.put("gog", "ws-1", asleep(sleeper("t1", "tab-1", "claude", sidB)))
	coreSnap := &herdr.Snapshot{Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "ws-9"}}}
	if !store.Observe(events.Snapshot{Machine: "gog", Snap: coreSnap}) {
		t.Fatal("the core's own workspace, closed, is not forgotten")
	}
	if _, ok := store.Of("gog", "w1"); !ok {
		t.Fatal("herdr's workspace is no core snapshot's to forget")
	}
	if _, ok := store.Of("gog", "ws-1"); ok {
		t.Fatal("a workspace put to sleep on the core and closed since is forgotten as ever")
	}
	slept := store.SleptInHerdr("gog")
	if c := slept["w1:p2"]; len(slept) != 2 || c.Kind != "codex" || c.Session != sidB || c.Dir != "/work/api" || c.Title != "api" || c.Core {
		t.Fatalf("herdr's sleepers: %+v", slept)
	}
	store.Woke("gog", "w1:p1")
	if w, ok := Open(dir).Of("gog", "w1"); !ok || len(w.Agents) != 1 || w.Agents[0].Pane != "w1:p2" {
		t.Fatalf("one dealt with, the other still asleep on disk: %+v", w)
	}
	store.Woke("gog", "w1:p2")
	store.Woke("gog", "w9:p9")
	if _, ok := store.Of("gog", "w1"); ok || len(store.SleptInHerdr("gog")) != 0 {
		t.Fatal("its last agent dealt with, herdr's workspace wakes")
	}
}

// A workspace that slept in herdr is not woken on the core, where a wake would start its agents with herdr's server
// perhaps still running them: Resume after restart is its way back.
func TestAWorkspaceThatSleptInHerdrIsNotWokenOnTheCore(t *testing.T) {
	fleet := machines.NewManager(t.TempDir(), nil, true)
	t.Cleanup(fleet.Close)
	mc, _ := fleet.Get(machines.LocalID)
	store := Open("")
	store.put(mc.ID(), "w1", asleep(sleeper("w1:p1", "w1:t1", "claude", sidA)))
	store.Core = func() bool { return true }
	if _, err := (&Service{Store: store}).Wake(mc, "w1"); !errors.Is(err, ErrSleptInHerdr) {
		t.Fatalf("woken: %v", err)
	}
	if _, ok := store.Of(mc.ID(), "w1"); !ok {
		t.Fatal("still asleep, for Resume after restart")
	}
}
