package sleep

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
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
