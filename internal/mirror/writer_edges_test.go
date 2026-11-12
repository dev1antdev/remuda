package mirror

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// writer is a machine's writer past its warm-up, the service's own as Observe makes it, the way the rig's tests meet it.
func (r *rig) writer() *writer {
	w := newWriter("gog", r.svc)
	r.svc.mu.Lock()
	r.svc.writers["gog"] = w
	r.svc.mu.Unlock()
	r.clock = r.clock.Add(warmUp)
	return w
}

func paneWrites(panes []herdr.PaneMetadata, id string) int {
	n := 0
	for _, p := range panes {
		if p.Pane == id {
			n++
		}
	}
	return n
}

// Two remuda on one herdr write under the same source: one with nothing to show on a pane clears the other's values
// once, as an earlier run's leftovers, and never again — else every snapshot is a clear, a rewrite and a snapshot more.
func TestAPlaceWithNothingToShowIsLookedAtOnceSoAnotherWriterIsNotClearedOverAndOver(t *testing.T) {
	r := newRig(t)
	if err := r.svc.Put(Settings{Figures: false}); err != nil {
		t.Fatal(err)
	}
	other := func() {
		r.herdr.mu.Lock()
		defer r.herdr.mu.Unlock()
		cost := "$1.00"
		r.herdr.apply("w1:p1", map[string]*string{TokenCost: &cost})
		r.herdr.apply("w2:p1", map[string]*string{TokenCost: &cost})
	}
	w := r.writer()
	other()
	r.sync(w, snapshot())
	panes, _ := r.herdr.take()
	if paneWrites(panes, "w1:p1") != 1 || paneWrites(panes, "w2:p1") != 1 {
		t.Fatalf("what herdr held when the writer first looked is cleared once: %+v", panes)
	}
	for i := 0; i < 3; i++ {
		other()
		r.clock = r.clock.Add(5 * time.Second)
		r.sync(w, snapshot())
		if panes, spaces := r.herdr.take(); len(panes)+len(spaces) != 0 {
			t.Fatalf("snapshot %d: the other writer's values are cleared again: %+v %+v", i, panes, spaces)
		}
	}
}

// A refusal of one pane's values (another reporter filled the pane's tokens) leaves that pane alone until its figures
// move or are due again; the rest of the machine is still written.
func TestAPaneHerdrRefusesIsLeftAloneAndTheRestAreStillWritten(t *testing.T) {
	r := newRig(t)
	snap := &herdr.Snapshot{Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1"}}}
	for _, id := range []string{"w1:p1", "w1:p2", "w1:p3"} {
		snap.Panes = append(snap.Panes, herdr.PaneInfo{PaneID: id, WorkspaceID: "w1", Agent: "claude"})
		r.costs[id] = 1
	}
	r.herdr.refuse = map[string]error{"w1:p1": &herdr.Error{Code: "metadata_token_limit", Message: "pane metadata may contain at most 32 tokens"}}
	w := r.writer()
	r.sync(w, snap)
	panes, spaces := r.herdr.take()
	if len(panes) != 3 || len(spaces) != 1 {
		t.Fatalf("one pane refused, yet %d pane writes and %d workspace writes", len(panes), len(spaces))
	}
	if !w.refused.IsZero() {
		t.Fatal("one pane's refusal holds the whole machine off")
	}
	for i := 0; i < 3; i++ {
		r.clock = r.clock.Add(5 * time.Second)
		r.sync(w, r.read(snap))
		if panes, _ := r.herdr.take(); len(panes) != 0 {
			t.Fatalf("snapshot %d: the refused pane is asked again at once: %+v", i, panes)
		}
	}
	r.costs["w1:p1"] = 2
	r.sync(w, snap)
	if panes, _ := r.herdr.take(); paneWrites(panes, "w1:p1") != 1 {
		t.Fatalf("a refused pane whose figures moved is not tried again: %+v", panes)
	}
}

// remuda's restart: the meter and the said tracker have read nothing yet, while herdr still holds what the run before
// wrote for minutes; the writer waits a moment rather than clear it all and write it again.
func TestAFreshWriterLeavesWhatHerdrHoldsUntilRemudaHasWarmedUp(t *testing.T) {
	r := newRig(t)
	snap := snapshot()
	snap.Panes[0].Tokens = map[string]string{TokenCost: "$4.20", TokenSaid: "the answer", TokenTask: "#42"}
	w := newWriter("gog", r.svc)
	w.sync(snap, r.clock)
	if panes, spaces := r.herdr.take(); len(panes)+len(spaces) != 0 {
		t.Fatalf("a writer just made cleared the run before's values: %+v %+v", panes, spaces)
	}
	r.costs["w1:p1"] = 4.2
	r.clock = r.clock.Add(warmUp)
	w.sync(snap, r.clock)
	panes, _ := r.herdr.take()
	if len(panes) != 1 || value(panes[0].Tokens, TokenCost) != "$4.20" {
		t.Fatalf("once warm the writer writes what it knows: %+v", panes)
	}
	if v, ok := panes[0].Tokens[TokenSaid]; !ok || v != nil {
		t.Errorf("a leftover remuda no longer knows is not cleared once warm: %v", panes[0].Tokens)
	}
}

// A writer ended (its machine removed, remuda stopping) writes nothing more, even in the middle of a sync.
func TestAnEndedWriterWritesNothing(t *testing.T) {
	r := newRig(t)
	r.costs["w1:p1"] = 1
	w := r.writer()
	w.end()
	r.sync(w, snapshot())
	if panes, spaces := r.herdr.take(); len(panes)+len(spaces) != 0 {
		t.Fatalf("an ended writer wrote %d times", len(panes)+len(spaces))
	}
}
