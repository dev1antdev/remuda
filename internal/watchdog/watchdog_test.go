package watchdog

import (
	"strconv"
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
	// the tag names the episode: a reminder replaces the note it reminds of, a later episode's note is its own
	if want := "m|p1|stuck|" + strconv.FormatInt(since.Unix(), 10); h.pushed[0].Tag != want || h.pushed[1].Tag != want {
		t.Fatalf("tags %q, %q; want both %q", h.pushed[0].Tag, h.pushed[1].Tag, want)
	}
	h.episodes[0].Since = h.now
	h.now = h.now.Add(25 * time.Minute)
	w.Sweep()
	if len(h.pushed) != 3 || h.pushed[2].Tag == h.pushed[0].Tag {
		t.Fatalf("a later episode's note: %+v", h.pushed[len(h.pushed)-1])
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
	h.episodes = []radar.Episode{{Machine: "m", Pane: "p1", State: herdr.StatusWorking, Since: h.now}}
	w.Sweep()
	if h.notes["p1"] != "" || len(w.fired) != 0 {
		t.Fatalf("note %q fired %d", h.notes["p1"], len(w.fired))
	}
}

// TestBriefReadsEveryNoteTheSweepWrites keeps the short words in step with the notes: each kind is swept for real.
func TestBriefReadsEveryNoteTheSweepWrites(t *testing.T) {
	h, w := newHarness()
	h.episodes = []radar.Episode{{Machine: "m", Pane: "stuck", State: herdr.StatusWorking, Since: h.now.Add(-30 * time.Minute)}}
	h.write = h.now.Add(-25 * time.Minute)
	w.Sweep()
	h.write = h.now
	h.tools = []string{"Bash\x00{}", "Bash\x00{}", "Bash\x00{}", "Bash\x00{}", "Bash\x00{}", "Bash\x00{}"}
	h.episodes = []radar.Episode{{Machine: "m", Pane: "loop", State: herdr.StatusWorking, Since: h.now.Add(-30 * time.Minute)}}
	w.Sweep()
	h.episodes = []radar.Episode{{Machine: "m", Pane: "waiting", State: herdr.StatusBlocked, Since: h.now.Add(-80 * time.Minute)}}
	w.Sweep()
	for pane, want := range map[string]string{"stuck": "stuck 25 min", "loop": "loop: Bash", "waiting": "waiting 1h 20m"} {
		if got := Brief(h.notes[pane]); got != want {
			t.Errorf("Brief(%q) = %q, want %q", h.notes[pane], got, want)
		}
	}
	if Brief("") != "" || Brief("something else") != "something else" {
		t.Error("a note Brief does not know must stay as it is")
	}
}
