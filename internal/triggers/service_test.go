package triggers

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/watch"
)

// heard is what a service did, on fakes.
type heard struct {
	mu      sync.Mutex
	pushes  []push.Payload
	feed    []string
	moves   []string
	asked   []int // the ports the previews were asked about
	viewed  bool
	listens int // the try on which the port is found listening, 0 for never
}

func (h *heard) service(t *testing.T) *Service {
	t.Helper()
	saved := listenTries
	listenTries = []time.Duration{0, 10 * time.Millisecond, 10 * time.Millisecond}
	t.Cleanup(func() { listenTries = saved })
	s := New(t.TempDir())
	s.Notify = func(p push.Payload) { h.mu.Lock(); h.pushes = append(h.pushes, p); h.mu.Unlock() }
	s.Feed = func(machine, pane, title, body string) {
		h.mu.Lock()
		h.feed = append(h.feed, title+" | "+body)
		h.mu.Unlock()
	}
	s.Listening = func(machine string, port int) (string, bool) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.asked = append(h.asked, port)
		return "vite", h.listens > 0 && len(h.asked) >= h.listens
	}
	s.MoveTask = func(machine, pane, column, why string) (string, bool, error) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.moves = append(h.moves, pane+" → "+column+" ("+why+")")
		return "Task #4 → " + column, true, nil
	}
	s.Viewed = func(machine, pane string) bool { h.mu.Lock(); defer h.mu.Unlock(); return h.viewed }
	return s
}

func (h *heard) ports() []int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]int(nil), h.asked...)
}

func (h *heard) counts() (pushes, feed, moves int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.pushes), len(h.feed), len(h.moves)
}

func snapshot(first bool, at time.Time, panes ...herdr.PaneInfo) events.Snapshot {
	return events.Snapshot{Machine: "gog", First: first, At: at, Snap: &herdr.Snapshot{
		Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1", Label: "api"}}, Panes: panes}}
}

func shell(id string) herdr.PaneInfo { return herdr.PaneInfo{PaneID: id, WorkspaceID: "w1"} }

func match(pane, watch, line string, fresh, carried bool, at time.Time) events.OutputMatched {
	return events.OutputMatched{Pane: events.PaneRef{Machine: "gog", Pane: pane}, Watch: watch, Line: line, Fresh: fresh, Carried: carried, At: at}
}

// eventually waits for a count the service's goroutines reach.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("waited for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

const failed = "--- FAIL: TestLogin (0.01s)"

func TestWhatStoodBeforeIsNoNewsAndWhatAppearsIs(t *testing.T) {
	h := &heard{}
	s := h.service(t)
	now := time.Now()
	s.Observe(snapshot(true, now, shell("w1:p1")))

	s.OnMatch(match("w1:p1", WatchTests, failed, true, false, now))
	time.Sleep(30 * time.Millisecond)
	if p, _, _ := h.counts(); p != 0 {
		t.Fatal("a failure standing on a pane there before remuda looked is where things stood, not news")
	}
	s.OnMatch(match("w1:p1", WatchTests, failed, true, true, now.Add(time.Minute)))
	time.Sleep(30 * time.Millisecond)
	if p, _, _ := h.counts(); p != 0 {
		t.Fatal("the same row again when the stream opened anew is no news")
	}
	s.OnMatch(match("w1:p1", WatchTests, "FAIL    github.com/acme/api 0.2s", false, false, now.Add(2*time.Minute)))
	eventually(t, "the push", func() bool { p, _, _ := h.counts(); return p == 1 })
	if got := h.pushes[0]; got.Kind != "tests" || got.Title != "Tests failed in api" || got.Pane != "w1:p1" || !strings.Contains(got.Body, "FAIL") {
		t.Fatalf("the push: %+v", got)
	}
	s.OnMatch(match("w1:p1", WatchTests, failed, false, false, now.Add(10*time.Minute)))
	time.Sleep(30 * time.Millisecond)
	if p, _, _ := h.counts(); p != 1 {
		t.Fatal("within the quiet spell a pane's tests say nothing more")
	}
	s.OnMatch(match("w1:p1", WatchTests, failed, false, false, now.Add(TestsQuiet+3*time.Minute)))
	eventually(t, "the next episode's push", func() bool { p, _, _ := h.counts(); return p == 2 })

	// a pane born while watched: what it prints first is news, even as its watch begins
	s.Observe(snapshot(false, now.Add(time.Hour), shell("w1:p1"), shell("w1:p2")))
	s.OnMatch(match("w1:p2", WatchTests, failed, true, false, now.Add(time.Hour+time.Second)))
	eventually(t, "the new pane's push", func() bool { p, _, _ := h.counts(); return p == 3 })
}

func TestTestsSayNothingWhileTheAgentWorksOrThePersonLooks(t *testing.T) {
	h := &heard{}
	s := h.service(t)
	now := time.Now()
	agent := herdr.PaneInfo{PaneID: "w1:p1", WorkspaceID: "w1", Agent: "claude", AgentStatus: herdr.StatusWorking}
	s.Observe(snapshot(true, now, agent))
	s.OnMatch(match("w1:p1", WatchTests, failed, false, false, now))
	time.Sleep(30 * time.Millisecond)
	if p, _, _ := h.counts(); p != 0 {
		t.Fatal("the agent is on it")
	}
	agent.AgentStatus = herdr.StatusDone
	s.Observe(snapshot(false, now, agent))
	h.mu.Lock()
	h.viewed = true
	h.mu.Unlock()
	s.OnMatch(match("w1:p1", WatchTests, "FAIL    github.com/acme/api 0.2s", false, false, now.Add(time.Second)))
	time.Sleep(30 * time.Millisecond)
	if p, _, _ := h.counts(); p != 0 {
		t.Fatal("the person has its terminal open")
	}
	h.mu.Lock()
	h.viewed = false
	h.mu.Unlock()
	s.OnMatch(match("w1:p1", WatchTests, "FAIL    github.com/acme/web 0.2s", false, false, now.Add(2*time.Second)))
	eventually(t, "the push at rest, unwatched", func() bool { p, _, _ := h.counts(); return p == 1 })
}

func TestADevServerIsOfferedOnceItsPortListens(t *testing.T) {
	h := &heard{listens: 2}
	var changed atomic.Int32
	s := h.service(t)
	s.Changed = func() { changed.Add(1) }
	now := time.Now()
	s.Observe(snapshot(true, now, shell("w1:p1")))
	s.OnMatch(match("w1:p1", WatchServe, "  ➜  Local:   http://localhost:5173/", false, false, now))
	eventually(t, "the offer", func() bool { return changed.Load() == 1 })
	if o := s.OfferOf("gog", "w1:p1"); o.Port != 5173 || o.URL != "http://localhost:5173/" || o.Label != "vite" {
		t.Fatalf("the offer: %+v", o)
	}
	if asked := h.ports(); len(asked) != 2 {
		t.Fatalf("asked the previews %v", asked)
	}
	s.Observe(snapshot(false, now, shell("w1:p2")))
	if s.OfferOf("gog", "w1:p1") != nil {
		t.Fatal("a pane gone takes its offer")
	}

	never := &heard{}
	quiet := never.service(t)
	quiet.Observe(snapshot(true, now, shell("w1:p1")))
	quiet.OnMatch(match("w1:p1", WatchServe, "listening on :3000", false, false, now))
	eventually(t, "every try", func() bool { return len(never.ports()) == 3 })
	time.Sleep(20 * time.Millisecond)
	if quiet.OfferOf("gog", "w1:p1") != nil {
		t.Fatalf("a port nothing listens on is never offered: %v", never.ports())
	}
}

func TestThePersonsRulesDoWhatTheySay(t *testing.T) {
	h := &heard{listens: 1}
	s := h.service(t)
	resubscribed := 0
	s.Resubscribe = func() { resubscribed++ }
	rules, err := s.Replace([]Rule{
		{ID: "p", Name: "Deployed", Match: "substring", Pattern: "DEPLOYED", Action: ActionPush, Enabled: true},
		{ID: "f", Name: "Migrated", Match: "substring", Pattern: "migrated", Action: ActionFeed, Enabled: true},
		{ID: "t", Name: "Ready for QA", Match: "regex", Pattern: `^ready for qa$`, Action: ActionTask, Column: "Testing", Enabled: true},
		{ID: "v", Name: "Storybook", Match: "regex", Pattern: `Storybook .* started`, Action: ActionPreview, Enabled: true},
		{ID: "off", Name: "Off", Match: "substring", Pattern: "x", Action: ActionPush},
	})
	if err != nil || len(rules) != 5 || resubscribed != 1 {
		t.Fatalf("replace: %v %d", err, resubscribed)
	}
	now := time.Now()
	s.Observe(snapshot(true, now, shell("w1:p1")))
	s.OnMatch(match("w1:p1", "rule:p", "DEPLOYED to staging", false, false, now))
	s.OnMatch(match("w1:p1", "rule:f", "12 tables migrated", false, false, now))
	s.OnMatch(match("w1:p1", "rule:t", "ready for qa", false, false, now))
	s.OnMatch(match("w1:p1", "rule:v", "Storybook 8.0 for react-vite started at http://localhost:6006/", false, false, now))
	s.OnMatch(match("w1:p1", "rule:off", "x", false, false, now))
	s.OnMatch(match("w1:p1", "rule:gone", "x", false, false, now))
	eventually(t, "every action", func() bool {
		p, f, m := h.counts()
		return p == 1 && f == 2 && m == 1 && s.OfferOf("gog", "w1:p1") != nil
	})
	if got := h.pushes[0]; got.Kind != "trigger" || got.Title != "Deployed · api" || got.Body != "DEPLOYED to staging" {
		t.Fatalf("the push: %+v", got)
	}
	if got := strings.Join(h.feed, "\n"); !strings.Contains(got, "Migrated · api | 12 tables migrated") || !strings.Contains(got, "Task #4 → Testing | Trigger “Ready for QA”: ready for qa") {
		t.Fatalf("the activity: %s", got)
	}
	if got := h.moves[0]; !strings.HasPrefix(got, "w1:p1 → Testing (trigger “Ready for QA”") {
		t.Fatalf("the move: %s", got)
	}
	if o := s.OfferOf("gog", "w1:p1"); o.Port != 6006 || o.By != "Storybook" {
		t.Fatalf("the rule's offer: %+v", o)
	}
	s.OnMatch(match("w1:p1", "rule:p", "DEPLOYED to production", false, false, now.Add(10*time.Second)))
	time.Sleep(30 * time.Millisecond)
	if p, _, _ := h.counts(); p != 1 {
		t.Fatal("a rule says one thing a pane in its quiet spell")
	}
}

func TestTheSameRowThroughTwoStreamsIsOneMatch(t *testing.T) {
	h := &heard{}
	s := h.service(t)
	now := time.Now()
	s.Observe(snapshot(true, now, shell("w1:p1")))
	s.OnMatch(match("w1:p1", WatchTests, failed, false, false, now))
	s.OnMatch(match("w1:p1", WatchTests, failed, false, false, now.Add(time.Second)))         // the stream before, as it closes
	s.OnMatch(match("w1:p1", WatchTests, failed, true, true, now.Add(1500*time.Millisecond))) // the one after, as it opens
	time.Sleep(50 * time.Millisecond)
	if p, _, _ := h.counts(); p != 1 {
		t.Fatalf("one failure, %d pushes", p)
	}
}

func TestRowsThatStoodAreNoNewsWhenAStreamToldThemAgainButOneFromTheGapIs(t *testing.T) {
	h := &heard{}
	s := h.service(t)
	now := time.Now()
	s.Observe(snapshot(true, now, shell("w1:p1")))
	summary := "FAIL    github.com/acme/api 0.2s"
	s.OnMatch(match("w1:p1", WatchTests, failed, false, false, now))
	s.OnMatch(match("w1:p1", WatchTests, summary, false, false, now.Add(time.Second)))
	eventually(t, "the run's push", func() bool { p, _, _ := h.counts(); return p == 1 })
	// past the quiet spell a new stream begins, and remuda-core tells every row standing: the two seen, then one the
	// pane printed while no stream stood
	later := now.Add(TestsQuiet + time.Minute)
	s.OnMatch(match("w1:p1", WatchTests, failed, true, true, later))
	s.OnMatch(match("w1:p1", WatchTests, summary, true, true, later))
	time.Sleep(30 * time.Millisecond)
	if p, _, _ := h.counts(); p != 1 {
		t.Fatalf("rows told again as a stream opened are no news: %d pushes", p)
	}
	s.OnMatch(match("w1:p1", WatchTests, "--- FAIL: TestLogout (0.01s)", true, true, later))
	eventually(t, "the push for the row from the gap", func() bool { p, _, _ := h.counts(); return p == 2 })
}

func TestRefusalsAreKeptByMachine(t *testing.T) {
	s := New(t.TempDir())
	s.Refused("gog", &herdr.Error{Code: "invalid_regex", Message: "look-around is not supported"})
	if got := s.Refusals()["gog"]; !strings.Contains(got, "look-around") {
		t.Fatalf("refusals: %v", s.Refusals())
	}
	s.Refused("gog", nil)
	if len(s.Refusals()) != 0 {
		t.Fatal("taken again")
	}
	s.Refused("gog", &herdr.Error{Code: "invalid_regex"})
	s.Forget("gog")
	if len(s.Refusals()) != 0 {
		t.Fatal("a machine removed")
	}
}

func TestWatchesPutThePersonsRulesFirstAndKeepToTheCap(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.Replace([]Rule{
		{ID: "api", Name: "In api", Machine: "gog", Dir: "/home/me/api", Match: "substring", Pattern: "DONE", Action: ActionFeed, Enabled: true},
		{ID: "all", Name: "Everywhere", Match: "regex", Pattern: "panic:", Action: ActionPush, Enabled: true},
	}); err != nil {
		t.Fatal(err)
	}
	snap := &herdr.Snapshot{
		Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1", Label: "api", Worktree: &herdr.WorktreeInfo{CheckoutPath: "/home/me/api"}}, {WorkspaceID: "w2", Label: "web"}},
		Panes:      []herdr.PaneInfo{{PaneID: "w1:p1", WorkspaceID: "w1", CWD: "/home/me/api"}, {PaneID: "w2:p1", WorkspaceID: "w2", CWD: "/home/me/web"}},
	}
	var got []string
	for _, o := range s.Watches("gog", snap, nil) {
		got = append(got, o.Pane+" "+o.Key)
	}
	want := "w1:p1 rule:api,w1:p1 rule:all,w2:p1 rule:all,w1:p1 serve,w1:p1 tests,w2:p1 serve,w2:p1 tests"
	if strings.Join(got, ",") != want {
		t.Fatalf("watches:\n%s\nwant\n%s", strings.Join(got, ","), want)
	}
	many := &herdr.Snapshot{}
	for i := 0; i < MaxWatches; i++ {
		many.Panes = append(many.Panes, herdr.PaneInfo{PaneID: "w1:p" + string(rune('a'+i%26)) + strings.Repeat("x", i/26)})
	}
	if n := len(s.Watches("gog", many, nil)); n != MaxWatches {
		t.Fatalf("%d watches past the cap", n)
	}
}

func TestANewPaneTakesNoWatchFromAPaneThatStands(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.Replace([]Rule{{ID: "all", Name: "Everywhere", Match: "substring", Pattern: "DONE", Action: ActionFeed, Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	var snap herdr.Snapshot
	for i := range 66 { // 66 panes × three watches each: inside the cap, so every one of them stands
		snap.Panes = append(snap.Panes, herdr.PaneInfo{PaneID: fmt.Sprintf("w1:p%d", i)})
	}
	stood := s.Watches("gog", &snap, nil)
	if len(stood) != 198 {
		t.Fatalf("a set of %d watches", len(stood))
	}

	// a pane appears before them all, and a second snapshot comes before the stream swaps
	grown := herdr.Snapshot{Panes: append([]herdr.PaneInfo{{PaneID: "w1:new"}}, snap.Panes...)}
	s.Watches("gog", &grown, stood)
	then := s.Watches("gog", &grown, stood)
	have, own := map[string]bool{}, 0
	for _, o := range then {
		have[o.Pane+" "+o.Key] = true
		if o.Pane == "w1:new" {
			own++
		}
	}
	for _, o := range stood {
		if !have[o.Pane+" "+o.Key] {
			t.Fatalf("a pane that stands lost its watch: %s %s", o.Pane, o.Key)
		}
	}
	if own == 3 {
		t.Fatalf("a pane that just appeared took %d watches at the cap", own)
	}
}

func TestAtTheCapAPersonRuleKeepsItsPlace(t *testing.T) {
	s := New(t.TempDir())
	// 100 panes: their built-in two are the whole cap already
	snap := &herdr.Snapshot{
		Workspaces: []herdr.WorkspaceInfo{{WorkspaceID: "w1", Folder: "/home/me/api"}, {WorkspaceID: "w2", Folder: "/srv/other"}},
	}
	snap.Panes = append(snap.Panes, herdr.PaneInfo{PaneID: "w1:p0", WorkspaceID: "w1", CWD: "/home/me/api"})
	for i := range 99 { // the rule's one pane, and ninety-nine outside its folder: built-ins are the whole cap
		snap.Panes = append(snap.Panes, herdr.PaneInfo{PaneID: fmt.Sprintf("w2:p%d", i), WorkspaceID: "w2", CWD: "/srv/other"})
	}
	standing := s.Watches("gog", snap, nil) // the built-ins of every pane, the whole cap
	if len(standing) != MaxWatches {
		t.Fatalf("a full cap of built-ins: %d", len(standing))
	}
	rule := Rule{ID: "api", Name: "In api", Machine: "gog", Dir: "/home/me/api", Match: "substring", Pattern: "DONE", Action: ActionFeed, Enabled: true}
	if _, err := s.Replace([]Rule{rule}); err != nil {
		t.Fatal(err)
	}
	has := func(watches []watch.Output) bool {
		for _, o := range watches {
			if o.Key == rulePrefix+rule.ID {
				return true
			}
		}
		return false
	}
	if !has(s.Watches("gog", snap, nil)) {
		t.Fatal("a rule saved on a machine whose built-ins fill the cap never takes effect")
	}
	// the pane leaves the rule's folder for a snapshot and comes back: the rule is not lost for it
	away := *snap
	away.Panes = append([]herdr.PaneInfo(nil), snap.Panes...)
	away.Panes[0].WorkspaceID, away.Panes[0].CWD = "w2", "/srv/other"
	if has(s.Watches("gog", &away, standing)) {
		t.Fatal("the rule covers a pane in another folder")
	}
	if !has(s.Watches("gog", snap, nil)) {
		t.Fatal("a pane that left the folder for a moment took the rule with it")
	}
}

func TestTheSameRowAtANewPlaceIsANewRow(t *testing.T) {
	s := New(t.TempDir())
	now := time.Now()
	k := key("gog", "w1:p1") + "|" + WatchTests
	first := match("w1:p1", WatchTests, failed, false, false, now)
	first.Pos = 10
	s.OnMatch(first)
	s.OnMatch(first) // the same row through two streams that overlap
	if got := len(s.standing[k]); got != 1 {
		t.Fatalf("the same place kept as %d rows", got)
	}
	moved := first
	moved.Pos = 11
	moved.At = now.Add(time.Second)
	s.OnMatch(moved)
	if got := len(s.standing[k]); got != 2 {
		t.Fatalf("a new place kept as %d rows, want 2", got)
	}
}
