package schedule

import (
	"strings"
	"testing"
)

func TestAScoutHeldOnThePlanSkipsAndOffersTheOtherVendor(t *testing.T) {
	st, door := scoutStore(t, t.TempDir())
	held := map[string]string{"codex": "Codex's 5 h window is at 97 % until Nov 12 16:50"}
	var offers []string
	st.Agents.Hold = func(s Schedule) string { return held[s.Agent] }
	st.Agents.Offer = func(s Schedule, why string) { offers = append(offers, s.Agent+": "+why) }
	s, err := st.Put(scout)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := st.Run(s.ID)
	if len(door.starts) != 0 || got.Skipped != SkipLimit || got.LastResult != "skipped: "+held["codex"] || len(offers) != 1 || offers[0] != "codex: "+held["codex"] {
		t.Fatalf("starts %v, schedule %+v, offers %v", door.starts, got, offers)
	}
	if len(door.pushed) != 0 {
		t.Fatalf("a skipped run pushed %+v", door.pushed)
	}

	ran, err := st.RunAs(s.ID, Swap{Agent: "claude", Model: "claude-sonnet-4-6"})
	if err != nil || len(door.starts) != 1 {
		t.Fatalf("run as Claude Code: %v, starts %v", err, door.starts)
	}
	if start := door.starts[0]; start.Agent != "claude" || start.Model != "claude-sonnet-4-6" || start.Args != "" || start.Prompt != scout.Prompt {
		t.Fatalf("started %+v", start)
	}
	if !strings.HasSuffix(ran.LastResult, "with Claude Code, for the plan limit") || len(offers) != 1 {
		t.Fatalf("after the swap %+v, offers %v", ran, offers)
	}
	if saved := st.List()[0]; saved.Agent != "codex" || saved.Model != "gpt-5" || saved.Args != "--full-auto" {
		t.Fatalf("the saved schedule changed: %+v", saved)
	}
}

func TestASwapHeldTooIsSkippedWithoutAnotherOffer(t *testing.T) {
	st, door := scoutStore(t, t.TempDir())
	offers := 0
	st.Agents.Hold = func(Schedule) string { return "spent" }
	st.Agents.Offer = func(Schedule, string) { offers++ }
	s, _ := st.Put(scout)
	if _, err := st.RunAs(s.ID, Swap{Agent: "claude"}); err == nil || err.Error() != "spent" || len(door.starts) != 0 || offers != 0 {
		t.Fatalf("%v, starts %v, offers %d", err, door.starts, offers)
	}
	if _, err := st.RunAs(s.ID, Swap{Agent: "shell"}); err == nil {
		t.Fatal("ran as a shell")
	}
}
