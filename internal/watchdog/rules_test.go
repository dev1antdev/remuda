package watchdog

import (
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/radar"
)

// lastN is chat.LastToolCalls over a turn's calls: the last n of them.
func lastN(calls []string) func(string, string, int) []string {
	return func(_, _ string, n int) []string {
		if len(calls) > n {
			return calls[len(calls)-n:]
		}
		return calls
	}
}

func repeat(call string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = call
	}
	return out
}

func TestWatchdogRuleEdges(t *testing.T) {
	bash := "Bash\x00{\"command\":\"npm test\"}"
	cases := []struct {
		name  string
		state string
		age   time.Duration
		write time.Duration // since the transcript's last write; 0 for no transcript at all
		calls []string
		kind  string // the push expected, "" for none
	}{
		{"working long with no transcript at all", herdr.StatusWorking, 40 * time.Minute, 0, nil, ""},
		{"working long, written to a minute ago", herdr.StatusWorking, 40 * time.Minute, time.Minute, nil, ""},
		{"working long, silent past the limit", herdr.StatusWorking, 40 * time.Minute, 20 * time.Minute, nil, "stuck"},
		{"silent, but working for less than the limit", herdr.StatusWorking, 19 * time.Minute, 19 * time.Minute, nil, ""},
		{"the same call six times in a row", herdr.StatusWorking, 25 * time.Minute, time.Minute, repeat(bash, 6), "loop"},
		{"the same call five times, a different one before", herdr.StatusWorking, 25 * time.Minute, time.Minute, append([]string{"Read\x00{}"}, repeat(bash, 5)...), ""},
		{"the same call five times only", herdr.StatusWorking, 25 * time.Minute, time.Minute, repeat(bash, 5), ""},
		{"six alike after others", herdr.StatusWorking, 25 * time.Minute, time.Minute, append([]string{"Read\x00{}", "Grep\x00{}"}, repeat(bash, 6)...), "loop"},
		{"waiting exactly the limit", herdr.StatusBlocked, 15 * time.Minute, 0, nil, "waiting"},
		{"waiting a second short of it", herdr.StatusBlocked, 15*time.Minute - time.Second, 0, nil, ""},
		{"idle for hours", herdr.StatusIdle, 5 * time.Hour, 5 * time.Hour, nil, ""},
	}
	for _, c := range cases {
		h, w := newHarness()
		h.episodes = []radar.Episode{{Machine: "m", Pane: "p1", State: c.state, Since: h.now.Add(-c.age)}}
		if c.write > 0 {
			h.write = h.now.Add(-c.write)
		}
		w.in.LastTools = lastN(c.calls)
		w.Sweep()
		got := ""
		if len(h.pushed) == 1 {
			got = h.pushed[0].Kind
		} else if len(h.pushed) > 1 {
			got = "several"
		}
		if got != c.kind {
			t.Errorf("%s: pushed %q, want %q", c.name, got, c.kind)
		}
		if (h.notes["p1"] != "") != (c.kind != "") {
			t.Errorf("%s: note %q", c.name, h.notes["p1"])
		}
	}
}

func TestWatchdogRemindsEveryHalfHourAndForgetsAnEndedEpisode(t *testing.T) {
	h, w := newHarness()
	since := h.now.Add(-16 * time.Minute)
	h.episodes = []radar.Episode{{Machine: "m", Pane: "p1", State: herdr.StatusBlocked, Since: since}}
	w.Sweep()
	h.now = h.now.Add(29 * time.Minute)
	w.Sweep()
	if len(h.pushed) != 1 {
		t.Fatalf("a reminder before the half hour: %d pushes", len(h.pushed))
	}
	h.now = h.now.Add(time.Minute)
	w.Sweep()
	if len(h.pushed) != 2 || !strings.Contains(h.pushed[1].Body, "46 min") {
		t.Fatalf("the reminder at the half hour: %+v", h.pushed)
	}

	h.episodes = []radar.Episode{{Machine: "m", Pane: "p1", State: herdr.StatusWorking, Since: h.now}}
	w.Sweep()
	if h.notes["p1"] != "" || len(w.fired) != 0 {
		t.Fatalf("an ended episode keeps its note %q or its memory %v", h.notes["p1"], w.fired)
	}
	h.episodes = []radar.Episode{{Machine: "m", Pane: "p1", State: herdr.StatusBlocked, Since: h.now.Add(-15 * time.Minute)}}
	w.Sweep()
	if len(h.pushed) != 3 {
		t.Fatalf("a new episode is pushed at once: %d pushes", len(h.pushed))
	}
}
