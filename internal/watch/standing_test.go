package watch

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
)

// A row remuda-core says stood before the watch began is where things stood, however late it is told: a core tells
// at most 256 rows a look, so a broad rule's standing rows may come past freshFor.
func TestARowTheServerSaysStoodIsFreshWheneverItIsTold(t *testing.T) {
	bus := events.NewBus()
	w := &Watcher{bus: bus}
	var heard []events.OutputMatched
	events.Subscribe(bus, func(e events.OutputMatched) { heard = append(heard, e) })
	fail := Output{Key: "tests", Pane: "t1", Match: herdr.OutputMatch{Type: "substring", Value: "FAIL"}}
	ls := &liveStream{started: time.Now().Add(-time.Minute), carried: map[string]bool{}, set: &streamSet{outputs: map[string][]Output{"t1": {fail}}}}
	for _, standing := range []bool{true, false} {
		w.heard("m", &machineState{}, ls, herdr.Event{Kind: herdr.EventOutputMatched, PaneID: "t1", MatchedLine: "x FAIL", Standing: standing})
	}
	if len(heard) != 2 || !heard[0].Fresh || heard[1].Fresh {
		t.Fatalf("heard %+v", heard)
	}
}
