package watchdog

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/radar"
)

type harness struct {
	now      time.Time
	episodes []radar.Episode
	write    time.Time
	tools    []string
	notes    map[string]string
	pushed   []push.Payload
}

func newHarness() (*harness, *Watchdog) {
	h := &harness{now: time.Unix(1_800_000_000, 0), notes: map[string]string{}}
	w := New(Inputs{
		Episodes: func() []radar.Episode { return h.episodes },
		Pane: func(string, string) (PaneInfo, bool) {
			return PaneInfo{Title: "Fix auth", Workspace: "api", Agent: "claude"}, true
		},
		LastWrite: func(string, string) (time.Time, bool) { return h.write, !h.write.IsZero() },
		LastTools: func(_, _ string, n int) []string { return h.tools },
		Note:      func(_, p, note string) { h.notes[p] = note },
		Notify:    func(p push.Payload) { h.pushed = append(h.pushed, p) },
		Now:       func() time.Time { return h.now },
	})
	return h, w
}

func TestQuietWorkingIsStuckOncePerEpisodeThenReminded(t *testing.T) {
	h, w := newHarness()
	since := h.now.Add(-25 * time.Minute)
	h.episodes = []radar.Episode{{Machine: "m", Pane: "p1", State: herdr.StatusWorking, Since: since}}
	h.write = h.now.Add(-22 * time.Minute)
	w.Sweep()
	if len(h.pushed) != 1 || h.pushed[0].Kind != "stuck" || len(h.pushed[0].Actions) != 2 {
		t.Fatalf("pushed %+v", h.pushed)
	}
	if h.notes["p1"] == "" {
		t.Fatal("no note on the pane")
	}
	h.now = h.now.Add(5 * time.Minute)
	w.Sweep()
	if len(h.pushed) != 1 {
		t.Fatalf("pushed again too soon: %d", len(h.pushed))
	}
	h.now = h.now.Add(30 * time.Minute)
	w.Sweep()
	if len(h.pushed) != 2 {
		t.Fatalf("no reminder: %d", len(h.pushed))
	}
}

func TestWorkingWithRecentWritesIsFine(t *testing.T) {
	h, w := newHarness()
	h.episodes = []radar.Episode{{Machine: "m", Pane: "p1", State: herdr.StatusWorking, Since: h.now.Add(-40 * time.Minute)}}
	h.write = h.now.Add(-1 * time.Minute)
	h.tools = []string{"Bash\x00a", "Bash\x00b", "Read\x00c", "Bash\x00a", "Bash\x00b", "Read\x00c"}
	w.Sweep()
	if len(h.pushed) != 0 || h.notes["p1"] != "" {
		t.Fatalf("a busy agent was flagged: %+v %q", h.pushed, h.notes["p1"])
	}
}

func TestLoopIsFlagged(t *testing.T) {
	h, w := newHarness()
	h.episodes = []radar.Episode{{Machine: "m", Pane: "p1", State: herdr.StatusWorking, Since: h.now.Add(-30 * time.Minute)}}
	h.write = h.now.Add(-1 * time.Minute)
	h.tools = []string{"Bash\x00npm test", "Bash\x00npm test", "Bash\x00npm test", "Bash\x00npm test", "Bash\x00npm test", "Bash\x00npm test"}
	w.Sweep()
	if len(h.pushed) != 1 || h.pushed[0].Kind != "loop" {
		t.Fatalf("pushed %+v", h.pushed)
	}
}

func TestLongWaitIsReminded(t *testing.T) {
	h, w := newHarness()
	h.episodes = []radar.Episode{{Machine: "m", Pane: "p1", State: herdr.StatusBlocked, Since: h.now.Add(-10 * time.Minute)}}
	w.Sweep()
	if len(h.pushed) != 0 {
		t.Fatal("too early")
	}
	h.now = h.now.Add(6 * time.Minute)
	w.Sweep()
	if len(h.pushed) != 1 || h.pushed[0].Kind != "waiting" {
		t.Fatalf("pushed %+v", h.pushed)
	}
	// answered: the episode is gone, the memory with it
	h.episodes = []radar.Episode{{Machine: "m", Pane: "p1", State: herdr.StatusWorking, Since: h.now}}
	w.Sweep()
	if h.notes["p1"] != "" || len(w.fired) != 0 {
		t.Fatalf("note %q fired %d", h.notes["p1"], len(w.fired))
	}
}
