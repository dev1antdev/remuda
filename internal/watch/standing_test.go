package watch

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
)

// Whether a row stood before its watch began is the server's word when it gives one — remuda-core does, for every
// row, however late it tells it (at most 256 rows a look) — and the stream's clock only when it gives none, as herdr.
func TestARowIsFreshByTheServersWordElseByTheClock(t *testing.T) {
	yes, no := true, false
	fail := Output{Key: "tests", Pane: "t1", Match: herdr.OutputMatch{Type: "substring", Value: "FAIL"}}
	for _, c := range []struct {
		name     string
		age      time.Duration
		standing *bool
		fresh    bool
	}{
		{"stood, told a minute in", time.Minute, &yes, true},
		{"printed live as the stream opened", 0, &no, false},
		{"herdr's, as the stream opened", 0, nil, true},
		{"herdr's, a minute in", time.Minute, nil, false},
	} {
		bus := events.NewBus()
		var heard []events.OutputMatched
		events.Subscribe(bus, func(e events.OutputMatched) { heard = append(heard, e) })
		w := &Watcher{bus: bus}
		ls := &liveStream{started: time.Now().Add(-c.age), carried: map[string]bool{}, set: &streamSet{outputs: map[string][]Output{"t1": {fail}}}}
		w.heard("m", &machineState{}, ls, herdr.Event{Kind: herdr.EventOutputMatched, PaneID: "t1", MatchedLine: "x FAIL", Standing: c.standing})
		if len(heard) != 1 || heard[0].Fresh != c.fresh {
			t.Errorf("%s: heard %+v", c.name, heard)
		}
	}
}

// A row remuda-core told goes to the watch that told it alone. The person adds a rule that matches what an old rule
// does: the new rule's feed tells a row as standing, the old rule's tells the same row live — the old rule hears it
// live once, never the new rule's standing copy.
func TestARowTheServerToldGoesToTheWatchThatToldIt(t *testing.T) {
	bus := events.NewBus()
	var heard []events.OutputMatched
	events.Subscribe(bus, func(e events.OutputMatched) { heard = append(heard, e) })
	w := &Watcher{bus: bus}
	kept := Output{Key: "rule:1", Pane: "t1", Match: herdr.OutputMatch{Type: "substring", Value: "FAIL"}}
	added := Output{Key: "rule:2", Pane: "t1", Match: herdr.OutputMatch{Type: "regex", Value: "FAIL: \\w+"}}
	ls := &liveStream{started: time.Now(), carried: map[string]bool{carryKey("t1", kept): true}, set: &streamSet{outputs: map[string][]Output{"t1": {kept, added}}}}
	yes, no := true, false
	w.heard("m", &machineState{}, ls, herdr.Event{Kind: herdr.EventOutputMatched, PaneID: "t1", MatchedLine: "--- FAIL: TestLogin", Standing: &yes, Watch: &added.Match})
	w.heard("m", &machineState{}, ls, herdr.Event{Kind: herdr.EventOutputMatched, PaneID: "t1", MatchedLine: "--- FAIL: TestLogin", Standing: &no, Watch: &kept.Match})
	if len(heard) != 2 || heard[0].Watch != "rule:2" || !heard[0].Fresh || heard[0].Carried || heard[1].Watch != "rule:1" || heard[1].Fresh || !heard[1].Carried {
		t.Fatalf("heard %+v", heard)
	}
	heard = nil
	w.heard("m", &machineState{}, ls, herdr.Event{Kind: herdr.EventOutputMatched, PaneID: "t1", MatchedLine: "--- FAIL: TestLogin"})
	if len(heard) != 2 {
		t.Fatalf("herdr's row names no watch and goes to each it matches: %+v", heard)
	}
}
