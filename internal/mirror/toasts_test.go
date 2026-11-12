package mirror

import (
	"errors"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/push"
)

func toastRig(t *testing.T) (*rig, *herdr.Snapshot) {
	r := newRig(t)
	snap := snapshot()
	snap.Tabs = []herdr.TabInfo{{TabID: "w1:t1", WorkspaceID: "w1"}, {TabID: "w2:t1", WorkspaceID: "w2"}}
	snap.Panes[0].TabID, snap.Panes[1].TabID = "w1:t1", "w2:t1"
	snap.FocusedTabID = "w2:t1"
	r.svc.in.Snapshot = func(string) *herdr.Snapshot { return snap }
	return r, snap
}

func TestToastsCarryTheAlertsTheSwitchAndTheHoursAllow(t *testing.T) {
	r, snap := toastRig(t)
	question := push.Payload{Machine: "gog", Pane: "w1:p1", Kind: "input", Title: "api needs you", Body: "Bash command · rm -rf build"}
	if _, ok := r.svc.toastOf(question, r.clock); ok {
		t.Fatal("toasts are off until the person turns them on")
	}
	if err := r.svc.Put(Settings{Figures: true, Toasts: true}); err != nil {
		t.Fatal(err)
	}
	toast, ok := r.svc.toastOf(question, r.clock)
	if !ok || toast.Title != question.Title || toast.Body != question.Body || toast.Sound != "request" {
		t.Fatalf("a question pops up with its words and the request sound: %+v %v", toast, ok)
	}
	for _, c := range []struct {
		p    push.Payload
		want bool
	}{
		{push.Payload{Machine: "gog", Pane: "w1:p1", Kind: "done", Worked: 300, Title: "api finished"}, true},
		{push.Payload{Machine: "gog", Pane: "w1:p1", Kind: "done", Worked: 12, Title: "api finished"}, false},
		{push.Payload{Machine: "gog", Pane: "w1:p1", Kind: "stuck", Title: "api looks stuck"}, true},
		{push.Payload{Machine: "gog", Pane: "w1:p1", Kind: "loop", Title: "api looks stuck"}, true},
		{push.Payload{Machine: "gog", Pane: "w1:p1", Kind: "waiting", Title: "api is still waiting"}, true},
		{push.Payload{Machine: "gog", Pane: "w1:p1", Kind: "budget", Title: "Budget at 82 %"}, false},
		{push.Payload{Machine: "gog", Kind: "machine", Title: "GOG is away"}, false},
		{push.Payload{Machine: "gog", Pane: "w2:p1", Kind: "input", Title: "web needs you"}, false}, // the tab herdr has in front
	} {
		if _, ok := r.svc.toastOf(c.p, r.clock); ok != c.want {
			t.Errorf("%s %s (%ds): toast %v, want %v", c.p.Kind, c.p.Pane, c.p.Worked, ok, c.want)
		}
	}
	if toast, _ := r.svc.toastOf(push.Payload{Machine: "gog", Pane: "w1:p1", Kind: "done", Worked: 300, Title: "x"}, r.clock); toast.Sound != "done" {
		t.Errorf("a finish plays the done sound: %q", toast.Sound)
	}

	snap.FocusedTabID = ""
	hour := r.clock.UTC().Format("15:04")
	later := r.clock.Add(2 * time.Hour).UTC().Format("15:04")
	if err := r.svc.Put(Settings{Toasts: true, Quiet: &push.Quiet{From: hour, To: later, TZ: "UTC"}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.svc.toastOf(question, r.clock.Add(time.Minute)); ok {
		t.Error("a toast inside the quiet hours")
	}
	if _, ok := r.svc.toastOf(question, r.clock.Add(3*time.Hour)); !ok {
		t.Error("no toast once the quiet hours are over")
	}
	if err := r.svc.Put(Settings{Toasts: true, Quiet: &push.Quiet{From: "25:00", To: "07:00"}}); err != nil || r.svc.Settings().Quiet != nil {
		t.Errorf("a quiet window that does not parse is dropped: %v %+v", err, r.svc.Settings().Quiet)
	}
}

func TestAToastsOutcomeIsKeptForTheSection(t *testing.T) {
	r, _ := toastRig(t)
	r.herdr.shown = herdr.Shown{Shown: false, Reason: "disabled"}
	outcome := r.svc.show("gog", herdr.Toast{Title: "api needs you"})
	if outcome.Shown || outcome.Reason != "disabled" || outcome.Label != "GOG" {
		t.Fatalf("outcome %+v", outcome)
	}
	if st := r.svc.Status(); len(st.Last) != 1 || st.Last[0].Reason != "disabled" || st.Snippet == "" || len(st.Tokens) != len(PaneTokens)+1 {
		t.Fatalf("the status carries the last toast, the snippet and the names: %+v", st)
	}
	r.herdr.err = errors.New("cannot reach herdr")
	outcomes, err := r.svc.Test()
	if err != nil || len(outcomes) != 1 || outcomes[0].Error == "" || outcomes[0].Machine != "gog" {
		t.Fatalf("a test toast on every connected machine says how each went: %+v %v", outcomes, err)
	}
	if len(r.herdr.toasts) != 2 || r.herdr.toasts[1].Sound != "none" {
		t.Errorf("the test toast plays no sound: %+v", r.herdr.toasts)
	}
	closed := New(t.TempDir(), Inputs{}, "a scratch instance")
	if _, err := closed.Test(); !errors.Is(err, ErrClosed) {
		t.Errorf("a scratch instance pops toasts on the person's herdr: %v", err)
	}
	closed.Put(Settings{Toasts: true})
	if _, ok := closed.toastOf(push.Payload{Machine: "gog", Pane: "w1:p1", Kind: "input"}, r.clock); ok {
		t.Error("a scratch instance's alert pops up in herdr")
	}
}
