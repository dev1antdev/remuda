package mirror

import (
	"maps"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/radar"
	"github.com/dev1antdev/remuda/internal/usage"
)

// rig is a mirror on a fake herdr and a fake clock, its facts set by the test.
type rig struct {
	herdr *fakeHerdr
	svc   *Service
	clock time.Time
	slept time.Duration
	costs map[string]float64 // by pane: an agent the meter read
	notes map[string]string
}

func newRig(t *testing.T) *rig {
	r := &rig{herdr: &fakeHerdr{}, clock: time.Unix(1_800_000_000, 0), costs: map[string]float64{}, notes: map[string]string{}}
	r.svc = New(t.TempDir(), Inputs{
		Herdr:    func(string) (Herdr, bool) { return r.herdr, true },
		Machines: func() []Machine { return []Machine{{ID: "gog", Label: "GOG"}} },
		Radar:    func(_ string, p *herdr.PaneInfo) radar.Display { return radar.Display{Alert: r.notes[p.PaneID]} },
		Usage: func(_, pane string) (usage.Display, bool) {
			cost, ok := r.costs[pane]
			return usage.Display{Name: "Opus 4.5", Cost: cost}, ok
		},
		Review: func(_, dir string) (int, bool) { return 2, false },
		Tasks:  func(string) (map[string]int64, error) { return map[string]int64{"w1:p1": 42}, nil },
	}, "")
	r.svc.now = func() time.Time { return r.clock }
	r.svc.sleep = func(d time.Duration) { r.clock = r.clock.Add(d); r.slept += d }
	return r
}

// snapshot is two workspaces: an agent in w1, a shell in w2.
func snapshot() *herdr.Snapshot {
	return &herdr.Snapshot{
		Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1"}, {WorkspaceID: "w2"}},
		Panes: []herdr.PaneInfo{
			{PaneID: "w1:p1", WorkspaceID: "w1", Agent: "claude", AgentStatus: herdr.StatusWorking, CWD: "/src/api"},
			{PaneID: "w2:p1", WorkspaceID: "w2", CWD: "/src/web"},
		},
	}
}

// read is a snapshot as herdr would answer it now: the tokens it holds on every pane and workspace.
func (r *rig) read(snap *herdr.Snapshot) *herdr.Snapshot {
	r.herdr.mu.Lock()
	defer r.herdr.mu.Unlock()
	for i := range snap.Panes {
		snap.Panes[i].Tokens = maps.Clone(r.herdr.held[snap.Panes[i].PaneID])
	}
	for i := range snap.Workspaces {
		snap.Workspaces[i].Tokens = maps.Clone(r.herdr.held[snap.Workspaces[i].WorkspaceID])
	}
	return snap
}

// sync reads the snapshot as herdr holds it now and syncs on it.
func (r *rig) sync(w *writer, snap *herdr.Snapshot) { w.sync(r.read(snap), r.clock) }

func value(m map[string]*string, name string) string {
	if v := m[name]; v != nil {
		return *v
	}
	return ""
}

func TestAWriteOnlyWhenAFigureMovesAndAgainBeforeTheTTL(t *testing.T) {
	r := newRig(t)
	r.costs["w1:p1"] = 4.2
	w := newWriter("gog", r.svc)
	r.sync(w, snapshot())
	panes, spaces := r.herdr.take()
	if len(panes) != 1 || panes[0].Pane != "w1:p1" || panes[0].Source != Source || panes[0].TTL != TTL {
		t.Fatalf("the agent pane is written once with the TTL, the shell never: %+v", panes)
	}
	if cost, task := value(panes[0].Tokens, TokenCost), value(panes[0].Tokens, TokenTask); cost != "$4.20" || task != "#42" {
		t.Fatalf("cost %q, task %q", cost, task)
	}
	if len(spaces) != 2 {
		t.Fatalf("both workspaces carry their review: %+v", spaces)
	}

	r.clock = r.clock.Add(time.Minute)
	r.sync(w, snapshot())
	if panes, spaces := r.herdr.take(); len(panes)+len(spaces) != 0 {
		t.Fatalf("nothing moved, yet %d writes went", len(panes)+len(spaces))
	}

	r.costs["w1:p1"] = 4.31
	r.clock = r.clock.Add(time.Second)
	r.sync(w, snapshot())
	panes, spaces = r.herdr.take()
	if len(panes) != 1 || len(spaces) != 1 || spaces[0].Workspace != "w1" {
		t.Fatalf("a moved cost writes its pane and its workspace alone: %d panes, %+v", len(panes), spaces)
	}

	r.clock = r.clock.Add(Refresh)
	r.sync(w, snapshot())
	panes, spaces = r.herdr.take()
	if len(panes) != 1 || len(spaces) != 2 || panes[0].TTL != TTL {
		t.Fatalf("after %s everything is written again before its TTL runs out: %d panes, %d workspaces", Refresh, len(panes), len(spaces))
	}
	if Refresh >= TTL {
		t.Fatal("the refresh must come before the TTL")
	}
}

func TestAnAgentThatLeftHasItsTokensClearedAndAGonePaneIsForgotten(t *testing.T) {
	r := newRig(t)
	r.costs["w1:p1"] = 1
	w := newWriter("gog", r.svc)
	r.sync(w, snapshot())
	r.herdr.take()

	snap := snapshot()
	snap.Panes[0].Agent = "" // the agent left: a shell again
	r.clock = r.clock.Add(time.Second)
	r.sync(w, snap)
	panes, _ := r.herdr.take()
	if len(panes) != 1 || panes[0].TTL != 0 || len(panes[0].Tokens) == 0 {
		t.Fatalf("one clearing write: %+v", panes)
	}
	for name, v := range panes[0].Tokens {
		if v != nil {
			t.Errorf("%s set to %q on a shell", name, *v)
		}
	}
	if _, ok := w.panes["w1:p1"]; ok {
		t.Error("a cleared pane stays in the writer's memory")
	}

	r.sync(w, snapshot())
	r.herdr.take()
	snap = snapshot()
	snap.Panes = snap.Panes[1:]
	r.sync(w, snap)
	if panes, _ := r.herdr.take(); len(panes) != 0 || len(w.panes) != 0 {
		t.Fatalf("a closed pane is forgotten without a write: %+v, %v", panes, w.panes)
	}
}

func TestAPlaceHerdrLostIsWrittenAgainAndLeftoversOfAnEarlierRunAreCleared(t *testing.T) {
	r := newRig(t)
	r.costs["w1:p1"] = 1
	w := newWriter("gog", r.svc)
	r.sync(w, snapshot())
	r.herdr.take()

	r.herdr.restart()
	r.clock = r.clock.Add(time.Minute)
	r.sync(w, snapshot())
	panes, spaces := r.herdr.take()
	if len(panes) != 1 || len(spaces) != 2 {
		t.Fatalf("what herdr lost in its restart is written again: %d panes, %d workspaces", len(panes), len(spaces))
	}

	// a snapshot read before the write landed may not show it yet: no write for that
	r.herdr.restart()
	w.sync(r.read(snapshot()), r.clock.Add(-time.Second))
	if panes, _ := r.herdr.take(); len(panes) != 0 {
		t.Fatalf("a snapshot older than the write wrote again: %d", len(panes))
	}

	// a fresh writer (remuda restarted) finds a last answer its earlier run left on a pane where nothing is known now
	fresh := newRig(t)
	snap := snapshot()
	snap.Panes[0].Tokens = map[string]string{TokenSaid: "an old answer", "someone_elses": "kept"}
	newWriter("gog", fresh.svc).sync(snap, fresh.clock)
	panes, _ = fresh.herdr.take()
	if len(panes) != 1 {
		t.Fatalf("the leftover is cleared in one write: %+v", panes)
	}
	if v, ok := panes[0].Tokens[TokenSaid]; !ok || v != nil {
		t.Errorf("remuda's leftover is not cleared: %v", panes[0].Tokens)
	}
	if _, touched := panes[0].Tokens["someone_elses"]; touched {
		t.Error("another reporter's token was touched")
	}
}

func TestTurningTheFiguresOffClearsEverythingRemudaWrote(t *testing.T) {
	r := newRig(t)
	r.costs["w1:p1"] = 2
	r.notes["w1:p1"] = "waiting for your answer for 20 min"
	w := newWriter("gog", r.svc)
	r.sync(w, snapshot())
	panes, _ := r.herdr.take()
	if len(panes) != 1 || panes[0].Labels[herdr.StatusWorking] != "waiting 20 min" {
		t.Fatalf("the note stands as a label: %+v", panes)
	}
	if err := r.svc.Put(Settings{Figures: false}); err != nil {
		t.Fatal(err)
	}
	r.clock = r.clock.Add(time.Second)
	r.sync(w, snapshot())
	panes, spaces := r.herdr.take()
	if len(panes) != 1 || !panes[0].ClearLabels || len(panes[0].Labels) != 0 || len(spaces) != 2 {
		t.Fatalf("off clears the pane's tokens and labels and both workspaces': %+v %+v", panes, spaces)
	}
	for _, m := range spaces {
		for name, v := range m.Tokens {
			if v != nil {
				t.Errorf("workspace %s keeps %s=%q", m.Workspace, name, *v)
			}
		}
	}
	r.clock = r.clock.Add(Refresh)
	r.sync(w, snapshot())
	if panes, spaces := r.herdr.take(); len(panes)+len(spaces) != 0 {
		t.Fatal("with the figures off nothing more is written")
	}
	if settings := New(r.svc.dir, Inputs{}, "").Settings(); settings.Figures {
		t.Error("the switch is not kept")
	}
	if settings := New(t.TempDir(), Inputs{}, "").Settings(); !settings.Figures || settings.Toasts {
		t.Errorf("a new cockpit shows its figures in herdr and keeps its toasts out: %+v", settings)
	}
}

func TestWritesArePacedAndARefusalHoldsTheMachineOff(t *testing.T) {
	r := newRig(t)
	snap := &herdr.Snapshot{Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1"}}}
	for _, id := range []string{"w1:p1", "w1:p2", "w1:p3", "w1:p4", "w1:p5", "w1:p6"} {
		snap.Panes = append(snap.Panes, herdr.PaneInfo{PaneID: id, WorkspaceID: "w1", Agent: "claude"})
		r.costs[id] = 1
	}
	w := newWriter("gog", r.svc)
	r.sync(w, snap)
	panes, spaces := r.herdr.take()
	if len(panes) != 6 || len(spaces) != 1 {
		t.Fatalf("six panes and their workspace, yet %d and %d writes", len(panes), len(spaces))
	}
	if want := 6 * time.Second / WritesPerSecond; r.slept != want {
		t.Errorf("seven writes waited %s, want %s", r.slept, want)
	}

	r.herdr.err = &herdr.Error{Code: "pane_not_found", Message: "gone"}
	r.costs["w1:p1"] = 2
	r.sync(w, snap)
	if _, ok := w.panes["w1:p1"]; ok {
		t.Error("a pane herdr no longer has is still remembered")
	}

	r.herdr.err = &herdr.Error{Code: "unknown_method", Message: "pane.report_metadata"}
	r.sync(w, snap)
	r.herdr.err = nil
	r.herdr.take()
	r.clock = r.clock.Add(time.Minute)
	r.sync(w, snap)
	if panes, _ := r.herdr.take(); len(panes) != 0 {
		t.Fatalf("a herdr that refused is asked again within %s: %d writes", refusedPause, len(panes))
	}
	r.clock = r.clock.Add(refusedPause)
	r.sync(w, snap)
	if panes, _ := r.herdr.take(); len(panes) == 0 {
		t.Fatal("the pause never ends")
	}
}

func TestObserveHandsTheSnapshotToTheMachinesOwnWriter(t *testing.T) {
	r := newRig(t)
	r.svc.sleep = func(time.Duration) {}
	r.costs["w1:p1"] = 1
	r.svc.Observe(events.Snapshot{Machine: "gog", Snap: snapshot(), At: r.clock})
	deadline := time.Now().Add(5 * time.Second)
	for {
		r.herdr.mu.Lock()
		n := len(r.herdr.panes)
		r.herdr.mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the writer never wrote")
		}
		time.Sleep(10 * time.Millisecond)
	}
	r.svc.Forget("gog")
	r.svc.Close()
	r.svc.Observe(events.Snapshot{Machine: "gog", Snap: snapshot(), At: r.clock})
	if len(r.svc.writers) != 0 {
		t.Error("a closed mirror starts a writer")
	}

	closed := New(t.TempDir(), Inputs{Herdr: func(string) (Herdr, bool) { return r.herdr, true }}, "a scratch instance")
	closed.Observe(events.Snapshot{Machine: "gog", Snap: snapshot(), At: r.clock})
	if len(closed.writers) != 0 {
		t.Error("a scratch instance writes into herdr")
	}
}
