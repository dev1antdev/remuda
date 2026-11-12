package schedule

import (
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/sessions"
)

func TestAPromptRunSkipsAnAgentAtWorkOrAsking(t *testing.T) {
	var pushed []push.Payload
	var answer error
	st := New(t.TempDir(), func(string, string, string) (string, error) { return "api · w3:p1", answer }, func(p push.Payload) { pushed = append(pushed, p) }, nil)
	s, _ := st.Put(Schedule{Machine: "local", Dir: "/tmp/x", Cron: "*/10 9-16 * * 1-5", Prompt: "status", Enabled: true})

	answer = sessions.ErrAgentWorking
	got, _ := st.Run(s.ID)
	if !got.LastOK || got.Skipped != SkipWorking || got.LastResult != "skipped: the agent in api · w3:p1 is still working" || got.LastRun.IsZero() {
		t.Fatalf("working: %+v", got)
	}
	answer = sessions.ErrAgentBusy
	got, _ = st.Run(s.ID)
	if !got.LastOK || got.Skipped != SkipAsking || got.LastResult != "skipped: the agent in api · w3:p1 waits for an answer" {
		t.Fatalf("asking: %+v", got)
	}
	if len(pushed) != 0 {
		t.Fatalf("a skip pushed %+v", pushed)
	}
	answer = nil
	got, _ = st.Run(s.ID)
	if !got.LastOK || got.Skipped != "" || got.LastResult != "sent to api · w3:p1" {
		t.Fatalf("sent: %+v", got)
	}
}

func TestARunWhileTheLastIsStillOnItsWayIsSkipped(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	calls := 0
	st := New(t.TempDir(), func(string, string, string) (string, error) {
		calls++
		started <- struct{}{}
		<-release
		return "pane", nil
	}, nil, nil)
	now := at("2026-10-07 09:00")
	st.now = func() time.Time { return now }
	s, _ := st.Put(Schedule{Machine: "local", Dir: "/tmp/x", Cron: "* * * * *", Prompt: "status", Enabled: true})
	st.Tick()
	<-started
	now = now.Add(time.Minute)
	got, _ := st.Run(s.ID)
	if got.Skipped != SkipRunning || !strings.Contains(got.LastResult, "still on its way") {
		t.Fatalf("a second run beside the first: %+v", got)
	}
	close(release)
	st.Wait()
	if calls != 1 {
		t.Fatalf("the prompt went %d times", calls)
	}
}

func TestAgentScheduleSkipsWhileItsPreviousAgentWorksOrAsks(t *testing.T) {
	st, door := scoutStore(t, t.TempDir())
	s, _ := st.Put(scout)
	_, _ = st.Run(s.ID)
	for _, c := range []struct{ status, why, words string }{
		{"working", SkipWorking, "is still working"},
		{"blocked", SkipAsking, "waits for an answer"},
		{"", SkipUnsure, "no word yet"},
	} {
		door.status = map[string]string{"w5:p2": c.status}
		got, _ := st.Run(s.ID)
		if got.Skipped != c.why || !strings.Contains(got.LastResult, c.words) || !strings.Contains(got.LastResult, "w5:p2") || !got.LastOK {
			t.Fatalf("%q: %+v", c.status, got)
		}
		// the agent still at it stays watched, so its tab is closed once it answers and not forgotten
		if got.LastPane == nil || got.LastPane.Pane != "w5:p2" {
			t.Fatalf("%q: the previous agent was let go: %+v", c.status, got.LastPane)
		}
	}
	if len(door.starts) != 1 || len(door.closed) != 0 || len(door.pushed) != 0 {
		t.Fatalf("starts %d, closed %+v, pushed %+v", len(door.starts), door.closed, door.pushed)
	}
}

// A finish nobody heard — remuda down at the time — is taken for one at the next run: the tab closes and the answer is
// pushed, as the status change would have done, and the new agent starts.
func TestAgentScheduleSettlesAFinishNobodyHeardAndStartsAgain(t *testing.T) {
	st, door := scoutStore(t, t.TempDir())
	s, _ := st.Put(scout)
	_, _ = st.Run(s.ID)
	door.status = map[string]string{"w5:p2": "done"}
	door.pane = "w6:p1"
	got, _ := st.Run(s.ID)
	if len(door.closed) != 1 || door.closed[0].Pane != "w5:p2" || len(door.pushed) != 1 || !strings.Contains(door.pushed[0].Title, "answered") {
		t.Fatalf("closed %+v, pushed %+v", door.closed, door.pushed)
	}
	if len(door.starts) != 2 || got.LastPane == nil || got.LastPane.Pane != "w6:p1" || got.LastResult != "started in w6:p1" || got.Skipped != "" {
		t.Fatalf("after the second run: %+v %+v", got, got.LastPane)
	}
}

func TestAgentScheduleForgetsAGonePaneAndStartsAgain(t *testing.T) {
	st, door := scoutStore(t, t.TempDir())
	s, _ := st.Put(scout)
	_, _ = st.Run(s.ID)
	door.status = map[string]string{"w5:p2": events.Gone}
	door.pane = "w6:p1"
	got, _ := st.Run(s.ID)
	if len(door.closed) != 0 || len(door.starts) != 2 || got.LastPane == nil || got.LastPane.Pane != "w6:p1" {
		t.Fatalf("closed %+v, starts %d, %+v", door.closed, len(door.starts), got.LastPane)
	}
}

// An agent at rest that never worked is still coming up while its first prompt may be on its way; past that it never took
// the prompt (the launcher pushed so), and its tab is left for the person while the next run goes ahead.
func TestAgentScheduleWaitsForAnAgentComingUpThenGoesOn(t *testing.T) {
	st, door := scoutStore(t, t.TempDir())
	now := at("2026-10-07 09:00")
	st.now = func() time.Time { return now }
	door.answer = ""
	s, _ := st.Put(scout)
	_, _ = st.Run(s.ID)
	door.status = map[string]string{"w5:p2": "idle"}
	now = now.Add(time.Minute)
	if got, _ := st.Run(s.ID); got.Skipped != SkipStarting || len(door.starts) != 1 {
		t.Fatalf("coming up: %+v", got)
	}
	now = now.Add(startingFor)
	door.pane = "w6:p1"
	got, _ := st.Run(s.ID)
	if len(door.starts) != 2 || len(door.closed) != 0 || got.LastPane == nil || got.LastPane.Pane != "w6:p1" {
		t.Fatalf("past the first prompt's time: starts %d closed %+v %+v", len(door.starts), door.closed, got)
	}
}

// One at rest that never was seen working but has an answer to show worked while nobody watched: it is through.
func TestAgentScheduleTakesAnAnswerForAFinish(t *testing.T) {
	st, door := scoutStore(t, t.TempDir())
	s, _ := st.Put(scout)
	_, _ = st.Run(s.ID)
	door.status = map[string]string{"w5:p2": "idle"}
	door.pane = "w6:p1"
	if got, _ := st.Run(s.ID); len(door.closed) != 1 || len(door.starts) != 2 || got.LastPane.Pane != "w6:p1" {
		t.Fatalf("closed %+v, starts %d, %+v", door.closed, len(door.starts), got)
	}
}

func TestUpcomingRunsOfAWorkdayWindow(t *testing.T) {
	st := New(t.TempDir(), nil, nil, nil)
	st.now = func() time.Time { return at("2026-11-13 16:35") } // a Friday
	next, err := st.Upcoming("*/10 9-16 * * 1-5", 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []time.Time{at("2026-11-13 16:40"), at("2026-11-13 16:50"), at("2026-11-16 09:00")}
	if len(next) != 3 || !next[0].Equal(want[0]) || !next[1].Equal(want[1]) || !next[2].Equal(want[2]) {
		t.Fatalf("next %v", next)
	}
	if _, err := st.Upcoming("*/10 9-17", 3); err == nil {
		t.Fatal("a cron of two fields read")
	}
	if next, err := st.Upcoming("0 0 31 2 *", 3); err != nil || len(next) != 0 {
		t.Fatalf("a day that never comes: %v %v", next, err)
	}
}
