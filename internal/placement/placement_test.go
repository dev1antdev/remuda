package placement

import (
	"strings"
	"testing"
)

func idle(id string) Machine {
	return Machine{ID: id, Label: strings.ToUpper(id), Dir: "/w/app", Workspace: "w1", PerCore: 0.1, Plan: 10}
}

func picked(t *testing.T, r Result) string {
	t.Helper()
	if r.Pick == nil {
		t.Fatalf("nothing picked: %+v", r.Others)
	}
	return r.Pick.ID
}

func TestTheLightestLoadWins(t *testing.T) {
	busy, light := idle("gog"), idle("roofr")
	busy.PerCore, light.PerCore = 1.2, 0.2
	r := Choose([]Machine{busy, light})
	if picked(t, r) != "roofr" || len(r.Others) != 1 || r.Others[0].ID != "gog" || r.Others[0].Out != "" {
		t.Fatalf("%+v", r)
	}
}

func TestAWarmCPUAndABatteryLoseToACoolMachineOnPower(t *testing.T) {
	warm, laptop, cool := idle("pi"), idle("mac"), idle("gog")
	warm.TempC = 78
	laptop.Battery = &Battery{Percent: 80, Discharging: true}
	cool.PerCore = 0.4 // busier, but cool and on power
	if got := picked(t, Choose([]Machine{warm, laptop, cool})); got != "gog" {
		t.Fatalf("picked %s", got)
	}
	charging := idle("mac")
	charging.Battery = &Battery{Percent: 30} // on its charger: no points lost
	if got := picked(t, Choose([]Machine{cool, charging})); got != "mac" {
		t.Fatalf("a Mac on its charger lost: %s", got)
	}
}

func TestAgentsAlreadyWorkingCount(t *testing.T) {
	crowded, quiet := idle("gog"), idle("roofr")
	crowded.Working = 3
	quiet.PerCore = 0.6
	if got := picked(t, Choose([]Machine{crowded, quiet})); got != "roofr" {
		t.Fatalf("picked %s", got)
	}
}

func TestTheFullerPlanLoses(t *testing.T) {
	full, empty := idle("gog"), idle("roofr")
	full.Plan, empty.Plan = 90, 10
	if got := picked(t, Choose([]Machine{full, empty})); got != "roofr" {
		t.Fatalf("picked %s", got)
	}
}

func TestTheHomeMachineWinsATie(t *testing.T) {
	away, home := idle("gog"), idle("mac")
	home.Home = true
	if got := picked(t, Choose([]Machine{away, home})); got != "mac" {
		t.Fatalf("picked %s", got)
	}
	if got := picked(t, Choose([]Machine{idle("a"), idle("b")})); got != "a" {
		t.Fatalf("a tie did not keep the order: %s", got)
	}
}

func TestMachinesWithNoRoomAreLeftOutWithTheirReason(t *testing.T) {
	hot, low, loaded, held, broke := idle("pi"), idle("mac"), idle("gog"), idle("roofr"), idle("box")
	hot.TempC = 88
	low.Battery = &Battery{Percent: 15, Discharging: true}
	loaded.PerCore = 2.4
	held.Held = "Claude's 5 h window is at 96 % until Nov 12 16:50"
	broke.Budget = "the budget for app is spent"
	r := Choose([]Machine{hot, low, loaded, held, broke})
	if r.Pick != nil || len(r.Others) != 5 {
		t.Fatalf("%+v", r)
	}
	outs := map[string]string{}
	for _, m := range r.Others {
		outs[m.ID] = m.Out
	}
	want := map[string]string{"pi": OutHot, "mac": OutBattery, "gog": OutLoad, "roofr": OutPlan, "box": OutBudget}
	for id, out := range want {
		if outs[id] != out {
			t.Errorf("%s: out %q, want %q", id, outs[id], out)
		}
	}
	if r.PlanOnly() {
		t.Fatal("left out for heat and load too, yet plan only")
	}
	words := NoRoom(r)
	for _, part := range []string{"PI: hot at 88 °C", "MAC: on battery at 15 %", "GOG: load 2.4 per core", "ROOFR: Claude's 5 h window", "BOX: the budget"} {
		if !strings.Contains(words, part) {
			t.Errorf("%q lacks %q", words, part)
		}
	}
}

func TestEveryMachineHeldOnThePlanAloneIsPlanOnly(t *testing.T) {
	a, b := idle("gog"), idle("roofr")
	a.Held, b.Held = "spent", "spent"
	if r := Choose([]Machine{a, b}); !r.PlanOnly() {
		t.Fatalf("%+v", r)
	}
	if (Result{}).PlanOnly() {
		t.Fatal("no candidate at all is plan only")
	}
	if !strings.Contains(NoRoom(Result{}), "no connected machine") {
		t.Fatal(NoRoom(Result{}))
	}
}

func TestFactsSayThePicksFigures(t *testing.T) {
	m := idle("gog")
	m.PerCore, m.Working, m.Plan, m.TempC = 0.34, 1, 40, 77
	m.Battery = &Battery{Percent: 60, Discharging: true}
	if got := Facts(m, "Claude"); got != "load 0.3 per core, warm at 77 °C, on battery at 60 %, 1 agent working, Claude at 40 %" {
		t.Fatal(got)
	}
	m.Plan = -1
	if got := Facts(m, "Claude"); strings.Contains(got, "Claude") {
		t.Fatalf("an unread plan said: %s", got)
	}
}

func TestLoadPerCore(t *testing.T) {
	if LoadPerCore(3, 4) != 0.75 || LoadPerCore(1.5, 0) != 1.5 {
		t.Fatal(LoadPerCore(3, 4), LoadPerCore(1.5, 0))
	}
}
