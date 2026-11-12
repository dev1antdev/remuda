package tasks

import (
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/placement"
)

type offered struct {
	task           int64
	machine, agent string
	why            string
}

// anywhere is the queued Inbox with its queue free to use any machine, Place answering what the test sets
func anywhere(t *testing.T, st *Store, h *queueHere, res *placement.Result) (Project, *[]offered) {
	t.Helper()
	p := queued(t, st, 2)
	a := p.Automation
	a.Anywhere = true
	p, err := st.SetAutomation(p.ID, a)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Automation.Anywhere {
		t.Fatal("the switch was not kept")
	}
	var offers []offered
	h.q.Place = func(a Automation, agent string) placement.Result { return *res }
	h.q.Offer = func(p Project, tk Task, machine, agent, why string) {
		offers = append(offers, offered{tk.ID, machine, agent, why})
	}
	return p, &offers
}

func TestAQueueFreeToUseAnyMachineStartsWhereThereIsRoom(t *testing.T) {
	st := open(t)
	h := newQueue(st)
	res := placement.Result{Pick: &placement.Machine{ID: "gog", Label: "GOG", Dir: "/home/me/app", Workspace: "w7", PerCore: 0.2}}
	p, _ := anywhere(t, st, h, &res)
	card := into(t, st, p.ID, "Fix the login page", column(p, func(s Status) bool { return s.OnQueue }))
	h.q.Sweep()
	if len(h.starts) != 1 || h.on[0] != "gog" || h.starts[0].Workspace != "w7" || h.starts[0].Branch != BranchFor(card) || !h.starts[0].Queued {
		t.Fatalf("started %+v on %v", h.starts, h.on)
	}
	n := note(t, st, p.ID)
	if n.State != "started" || n.Reason != "started "+card.Identifier+" on GOG" || n.Fleet == nil || n.Fleet.Pick.ID != "gog" {
		t.Fatalf("note %+v", n)
	}
}

func TestNoRoomAnywhereWaitsAndOffersTheOtherVendorOnlyForThePlan(t *testing.T) {
	st := open(t)
	h := newQueue(st)
	hot := placement.Machine{ID: "pi", Label: "Pi", TempC: 90, Out: placement.OutHot}
	held := placement.Machine{ID: "gog", Label: "GOG", Held: "Claude's 5 h window is at 97 % until Nov 12 16:50", Out: placement.OutPlan}
	res := placement.Result{Others: []placement.Machine{hot, held}}
	p, offers := anywhere(t, st, h, &res)
	card := into(t, st, p.ID, "Fix the login page", column(p, func(s Status) bool { return s.OnQueue }))
	h.q.Sweep()
	if len(h.starts) != 0 || len(*offers) != 0 {
		t.Fatalf("started %v, offered %v with a hot machine among them", h.starts, *offers)
	}
	if n := note(t, st, p.ID); n.State != "waiting" || !strings.HasPrefix(n.Reason, "no machine has room: Pi: hot at 90 °C; GOG: Claude's") || n.Fleet == nil {
		t.Fatalf("note %+v", n)
	}
	res.Others = []placement.Machine{held}
	h.q.Sweep()
	if len(*offers) != 1 || (*offers)[0] != (offered{card.ID, "", "claude", held.Held}) {
		t.Fatalf("offers %+v", *offers)
	}
}

func TestAFixedMachineHeldOnThePlanOffersTheOtherVendor(t *testing.T) {
	st := open(t)
	h := newQueue(st)
	p := queued(t, st, 1)
	var offers []offered
	placed := false
	h.q.Place = func(Automation, string) placement.Result { placed = true; return placement.Result{} }
	h.q.Offer = func(p Project, tk Task, machine, agent, why string) {
		offers = append(offers, offered{tk.ID, machine, agent, why})
	}
	card := into(t, st, p.ID, "Fix the login page", column(p, func(s Status) bool { return s.OnQueue }))
	h.spent = "Claude's 5 h window is at 96 % until Nov 12 16:50"
	h.q.Sweep()
	if len(h.starts) != 0 || len(offers) != 1 || offers[0] != (offered{card.ID, "studio", "claude", h.spent}) {
		t.Fatalf("starts %v, offers %+v", h.starts, offers)
	}
	if placed {
		t.Fatal("a project on its own machine asked where there is room")
	}
}

func TestAnOfferedCardRunsOnTheOtherVendorOnce(t *testing.T) {
	st := open(t)
	h := newQueue(st)
	p := queued(t, st, 1)
	queue := column(p, func(s Status) bool { return s.OnQueue })
	card := into(t, st, p.ID, "Fix the login page", queue)
	words, err := h.q.RunOffered(p.ID, card.ID, "codex", "gpt-6.1")
	if err != nil || len(h.starts) != 1 || h.starts[0].Agent != "codex" || h.starts[0].Model != "gpt-6.1" || h.on[0] != "studio" || h.starts[0].Workspace != "w1" {
		t.Fatalf("%q %v: %+v", words, err, h.starts)
	}
	if !strings.Contains(words, "Codex") {
		t.Fatalf("words %q", words)
	}
	if _, err := h.q.RunOffered(p.ID, card.ID, "codex", ""); err == nil {
		t.Fatal("an offer ran a card that had left Queued")
	}
	other := into(t, st, p.ID, "Add a dark theme", queue)
	h.spent = "Codex's 5 h window is at 99 % until Nov 12 16:50"
	if _, err := h.q.RunOffered(p.ID, other.ID, "codex", ""); err == nil || !strings.Contains(err.Error(), "Codex's") {
		t.Fatalf("ran with the other vendor held too: %v", err)
	}
	if saved, _ := st.Project(p.ID); saved.Automation.Agent != "claude" {
		t.Fatalf("the project's agent changed to %q", saved.Automation.Agent)
	}
}
