package said

import (
	"reflect"
	"testing"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestTrackerKeepsTheIssueKeysWhileTheAgentWorks(t *testing.T) {
	f := &fake{versions: map[string]string{"local|w1:p1": "10-1"}, answers: map[string]string{"local|w1:p1": "Done with CRMOPS-6279."}}
	keys := []string{"CRMOPS-6279"}
	tr := New(Inputs{Version: f.version, Answer: f.answer, Keys: func(m, p string) []string { return keys }}, nil)
	tr.OnStatus(status(herdr.StatusDone))
	waitFor(t, "the keys", func() bool { return len(tr.Keys("local", "w1:p1")) == 1 })

	tr.OnStatus(status(herdr.StatusWorking))
	if got := tr.Keys("local", "w1:p1"); !reflect.DeepEqual(got, keys) {
		t.Fatalf("a working agent lost the keys its work is about: %v", got)
	}
	tr.OnStatus(status(events.Gone))
	if tr.Keys("local", "w1:p1") != nil {
		t.Fatal("a closed pane keeps its keys")
	}
	var none *Tracker
	if none.Keys("local", "w1:p1") != nil {
		t.Fatal("a nil tracker named keys")
	}
}
