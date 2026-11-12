package mirror

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/push"
)

// onOneHerdr is a cockpit reaching GOG over ssh and GOG's own remuda, both writing into GOG's one herdr; the
// cockpit knows a task on the agent, the machine's own remuda knows none.
func onOneHerdr(t *testing.T) (cockpit, own *rig) {
	cockpit, own = newRig(t), newRig(t)
	own.herdr = cockpit.herdr
	cockpit.svc.in.Self = "rp-orange:7317"
	own.svc.in.Self = "gog:7317"
	own.svc.in.Local = func(string) bool { return true }
	own.svc.in.Tasks = func(string) (map[string]int64, error) { return nil, nil }
	cockpit.costs["w1:p1"], own.costs["w1:p1"] = 1, 1
	for _, r := range []*rig{cockpit, own} {
		r.svc.in.Snapshot = func(string) *herdr.Snapshot { return r.read(snapshot()) } // the watcher's latest
	}
	return cockpit, own
}

func claims(r *rig) map[string]string {
	r.herdr.mu.Lock()
	defer r.herdr.mu.Unlock()
	return map[string]string{"w1": r.herdr.held["w1"][TokenBy], "w2": r.herdr.held["w2"][TokenBy]}
}

func TestOneHerdrHasOneWriterTheCockpitOverTheMachinesOwn(t *testing.T) {
	cockpit, own := onOneHerdr(t)
	for _, r := range []*rig{cockpit, own} {
		if err := r.svc.Put(Settings{Figures: true, Toasts: true}); err != nil {
			t.Fatal(err)
		}
	}
	cw, ow := cockpit.writer(), own.writer()
	own.sync(ow, snapshot()) // the machine's own was there first
	if got := claims(own); got["w1"] != "local gog:7317" {
		t.Fatalf("the machine's own remuda claims its herdr: %v", got)
	}
	cockpit.sync(cw, snapshot())
	if got := claims(cockpit); got["w1"] != "remote rp-orange:7317" || got["w2"] != "remote rp-orange:7317" {
		t.Fatalf("the cockpit takes the herdr over: %v", got)
	}
	cockpit.herdr.take()

	for i := 0; i < 3; i++ {
		own.clock = own.clock.Add(5 * time.Second)
		own.sync(ow, snapshot())
		cockpit.clock = cockpit.clock.Add(5 * time.Second)
		cockpit.sync(cw, snapshot())
	}
	if panes, spaces := own.herdr.take(); len(panes)+len(spaces) != 0 {
		t.Fatalf("someone still writes with nothing moved: %+v %+v", panes, spaces)
	}
	cockpit.herdr.mu.Lock()
	task := cockpit.herdr.held["w1:p1"][TokenTask]
	cockpit.herdr.mu.Unlock()
	if task != "#42" {
		t.Errorf("the cockpit's task stands on the pane: %q", task)
	}
	if st := own.svc.Status(); len(st.Elsewhere) != 1 || st.Elsewhere[0].Writer != "rp-orange:7317" || st.Elsewhere[0].Label != "GOG" {
		t.Errorf("the machine's own remuda says who writes there: %+v", st.Elsewhere)
	}
	if st := cockpit.svc.Status(); len(st.Elsewhere) != 0 {
		t.Errorf("the cockpit stays out of nowhere: %+v", st.Elsewhere)
	}
	question := push.Payload{Machine: "gog", Pane: "w1:p1", Kind: "input", Title: "api needs you"}
	if _, ok := own.svc.toastOf(question, own.clock); ok {
		t.Error("the machine's own remuda pops the toast the cockpit pops too")
	}
	if _, ok := cockpit.svc.toastOf(question, cockpit.clock); !ok {
		t.Error("the cockpit pops no toast")
	}
	restarted := newRig(t) // the machine's own just restarted: no writer has synced, its warm-up still runs
	restarted.herdr = cockpit.herdr
	restarted.svc.in.Self, restarted.svc.in.Local = "gog:7317", own.svc.in.Local
	restarted.svc.in.Snapshot = func(string) *herdr.Snapshot { return restarted.read(snapshot()) }
	if err := restarted.svc.Put(Settings{Figures: true, Toasts: true}); err != nil {
		t.Fatal(err)
	}
	if _, ok := restarted.svc.toastOf(question, restarted.clock); ok {
		t.Error("a machine's own remuda in its warm-up pops the toast the cockpit pops")
	}

	// the cockpit stops: its claim lapses with its TTL, and the machine's own takes its herdr back
	cockpit.herdr.mu.Lock()
	delete(cockpit.herdr.held["w1"], TokenBy)
	delete(cockpit.herdr.held["w2"], TokenBy)
	cockpit.herdr.mu.Unlock()
	own.clock = own.clock.Add(TTL)
	own.sync(ow, own.read(snapshot()))
	if got := claims(own); got["w1"] != "local gog:7317" || ow.yielding() != "" {
		t.Fatalf("the claim lapsed, yet the machine's own stays out: %v", got)
	}
}

func TestACockpitWithTheFiguresOffStillClaimsTheHerdrItPopsToastsOn(t *testing.T) {
	cockpit, own := onOneHerdr(t)
	if err := cockpit.svc.Put(Settings{Toasts: true}); err != nil {
		t.Fatal(err)
	}
	cw, ow := cockpit.writer(), own.writer()
	cockpit.sync(cw, snapshot())
	panes, spaces := cockpit.herdr.take()
	if len(panes) != 0 || len(spaces) != 2 || len(spaces[0].Tokens) != 1 || value(spaces[0].Tokens, TokenBy) == "" {
		t.Fatalf("the claim alone goes on each workspace: %+v %+v", panes, spaces)
	}
	own.sync(ow, snapshot())
	if panes, spaces := own.herdr.take(); len(panes)+len(spaces) != 0 || ow.yielding() == "" {
		t.Fatalf("the machine's own writes beside a cockpit that pops the toasts: %+v %+v", panes, spaces)
	}

	// both switches off: what it wrote is cleared, the other's claim never
	if err := cockpit.svc.Put(Settings{}); err != nil {
		t.Fatal(err)
	}
	cockpit.clock = cockpit.clock.Add(time.Second)
	cockpit.sync(cw, snapshot())
	if got := claims(cockpit); got["w1"] != "" {
		t.Fatalf("a cockpit that mirrors nothing keeps its claim: %v", got)
	}
	cockpit.herdr.mu.Lock()
	cockpit.herdr.held["w1"][TokenBy] = "local gog:7317"
	cockpit.herdr.mu.Unlock()
	fresh := newRig(t)
	fresh.herdr = cockpit.herdr
	fresh.svc.in.Self = "rp-orange:7317"
	if err := fresh.svc.Put(Settings{}); err != nil {
		t.Fatal(err)
	}
	fresh.sync(fresh.writer(), snapshot())
	if got := claims(fresh); got["w1"] != "local gog:7317" {
		t.Errorf("a remuda that mirrors nothing cleared another's claim: %v", got)
	}
}

func TestAClaimOutranksByRoleThenByName(t *testing.T) {
	for _, c := range []struct {
		other, ours string
		want        bool
	}{
		{"remote rp-orange:7317", "local gog:7317", true},
		{"local gog:7317", "remote rp-orange:7317", false},
		{"remote a:7317", "remote b:7317", true},
		{"remote b:7317", "remote a:7317", false},
		{"local a:7417", "local a:7317", false},
	} {
		if got := outranks(c.other, c.ours); got != c.want {
			t.Errorf("outranks(%q, %q) = %v, want %v", c.other, c.ours, got, c.want)
		}
	}
	if claimOf("", false) != "remote" || claimOf("gog:7317", true) != "local gog:7317" {
		t.Errorf("a claim is written as herdr keeps it: %q %q", claimOf("", false), claimOf("gog:7317", true))
	}
}
