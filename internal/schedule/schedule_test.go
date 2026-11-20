package schedule

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/state"
)

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func TestCron(t *testing.T) {
	cases := []struct {
		expr string
		when string
		want bool
	}{
		{"0 9 * * 1-5", "2026-10-07 09:00", true}, // a Wednesday
		{"0 9 * * 1-5", "2026-10-07 09:01", false},
		{"0 9 * * 1-5", "2026-10-11 09:00", false}, // a Sunday
		{"0 9 * * 0", "2026-10-11 09:00", true},
		{"0 9 * * 7", "2026-10-11 09:00", true},
		{"*/15 * * * *", "2026-10-07 09:30", true},
		{"*/15 * * * *", "2026-10-07 09:31", false},
		{"30 18 1,15 * *", "2026-10-15 18:30", true},
		{"30 18 1,15 * *", "2026-10-16 18:30", false},
		{"0 */6 * * *", "2026-10-07 12:00", true},
		{"0 */6 * * *", "2026-10-07 13:00", false},
	}
	for _, c := range cases {
		cr, err := Parse(c.expr)
		if err != nil {
			t.Fatalf("%q: %v", c.expr, err)
		}
		if got := cr.Matches(at(c.when)); got != c.want {
			t.Errorf("%q at %s: %v, want %v", c.expr, c.when, got, c.want)
		}
	}
	for _, bad := range []string{"", "0 9 * *", "60 9 * * *", "0 25 * * *", "0 9 * * 8", "a b c d e", "0 9 * * */0"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
	cr, _ := Parse("0 9 * * 1-5")
	next, ok := cr.Next(at("2026-10-09 10:00")) // a Friday, after nine
	if !ok || !next.Equal(at("2026-10-12 09:00")) {
		t.Fatalf("next: %v %v", next, ok)
	}
}

func TestTickFiresOncePerMatchingMinute(t *testing.T) {
	var mu sync.Mutex
	var sent []string
	st := New(t.TempDir(), func(machine, dir, text string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		sent = append(sent, text)
		return "pane", nil
	}, nil)
	count := func() int {
		st.Wait()
		mu.Lock()
		defer mu.Unlock()
		return len(sent)
	}
	now := at("2026-10-07 09:00")
	st.now = func() time.Time { return now }
	if _, err := st.Put(Schedule{Machine: "local", Dir: "/tmp/x", Cron: "0 9 * * *", Prompt: "status", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	st.Tick()
	st.Wait()
	now = now.Add(20 * time.Second)
	st.Tick()
	if n := count(); n != 1 {
		t.Fatalf("sent %d", n)
	}
	now = now.Add(time.Minute)
	st.Tick()
	if n := count(); n != 1 {
		t.Fatalf("sent %d after the minute passed", n)
	}
	list := st.List()
	if !list[0].LastOK || list[0].LastResult != "sent to pane" {
		t.Fatalf("result %+v", list[0])
	}
	if list[0].Next.IsZero() || !list[0].Next.Equal(at("2026-10-08 09:00")) {
		t.Fatalf("next %v", list[0].Next)
	}
}

// A delivery can outlast the half minute between ticks — the pane's lock held by a restart, a remote slow to answer —
// and the next tick, finding no run noted yet, must not start the same schedule again.
func TestASlowRunIsNotStartedAgainWithinItsMinute(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	st := New(t.TempDir(), func(machine, dir, text string) (string, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		started <- struct{}{}
		<-release
		return "pane", nil
	}, nil)
	now := at("2026-10-07 09:00")
	st.now = func() time.Time { return now }
	if _, err := st.Put(Schedule{Machine: "local", Dir: "/tmp/x", Cron: "0 9 * * *", Prompt: "status", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	st.Tick()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("the first tick did not fire")
	}
	now = now.Add(30 * time.Second)
	st.Tick()
	close(release)
	st.Wait()
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("the prompt went %d times within one minute", calls)
	}
	if got := st.List()[0]; !got.LastOK || got.LastResult != "sent to pane" {
		t.Fatalf("result %+v", got)
	}
}

func TestFailureIsNotedAndPushed(t *testing.T) {
	var pushed []push.Payload
	st := New(t.TempDir(), func(string, string, string) (string, error) { return "", errors.New("no agent in the folder") }, func(p push.Payload) { pushed = append(pushed, p) })
	s, _ := st.Put(Schedule{Machine: "local", Dir: "/tmp/x", Cron: "* * * * *", Prompt: "hi", Enabled: true})
	if _, err := st.Run(s.ID); err != nil {
		t.Fatal(err)
	}
	got := st.List()[0]
	if got.LastOK || got.LastResult != "no agent in the folder" || len(pushed) != 1 || pushed[0].Kind != "schedule" {
		t.Fatalf("got %+v pushed %+v", got, pushed)
	}
}

func TestDisabledNeverFires(t *testing.T) {
	fired := 0
	st := New(t.TempDir(), func(string, string, string) (string, error) { fired++; return "p", nil }, nil)
	st.now = func() time.Time { return at("2026-10-07 09:00") }
	_, _ = st.Put(Schedule{Machine: "local", Dir: "/tmp/x", Cron: "* * * * *", Prompt: "hi", Enabled: false})
	st.Tick()
	st.Wait()
	if fired != 0 {
		t.Fatal("a disabled schedule fired")
	}
}

func TestCronDayFieldsOrLikeVixie(t *testing.T) {
	c, _ := Parse("0 9 1 * 1")              // the 1st, or a Monday
	if !c.Matches(at("2026-10-01 09:00")) { // a Thursday, the 1st
		t.Fatal("the 1st did not match")
	}
	if !c.Matches(at("2026-10-05 09:00")) { // a Monday, the 5th
		t.Fatal("a Monday did not match")
	}
	if c.Matches(at("2026-10-06 09:00")) { // a Tuesday, the 6th
		t.Fatal("a plain Tuesday matched")
	}
	only, _ := Parse("0 9 1 * *")
	if only.Matches(at("2026-10-05 09:00")) {
		t.Fatal("a restricted day-of-month alone matched a Monday")
	}
}

func TestKindDefaultsToPromptOnLoadAndAStandupDigestIsDropped(t *testing.T) {
	dir := t.TempDir()
	if err := state.SaveJSON(dir, file, []map[string]any{{"id": "a", "machine": "local", "dir": "/tmp/x", "label": "old", "cron": "0 9 * * *", "prompt": "hi", "enabled": true}, {"id": "b", "kind": "digest", "label": "d", "cron": "0 9 * * *", "since": "midnight", "enabled": true}, {"id": "c", "kind": "scene", "scene": "s1", "label": "morning", "cron": "0 8 * * *", "enabled": true}}); err != nil {
		t.Fatal(err)
	}
	st := New(dir, nil, nil)
	list := st.List()
	kinds := map[string]string{}
	for _, s := range list {
		kinds[s.ID] = s.Kind
	}
	if len(kinds) != 2 || kinds["a"] != KindPrompt || kinds["c"] != KindScene {
		t.Fatalf("loaded %v", kinds)
	}
	var kept []map[string]any
	if err := state.LoadJSON(dir, file, &kept); err != nil || len(kept) != 2 {
		t.Fatalf("the file keeps %d schedules (%v): the digest is dropped from it once", len(kept), err)
	}
}

func TestAnUnreadableFileRunsNothingAndIsNeverWrittenOver(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`[{"id":"d","kind":"digest","cron":"0 9 * * *","enabled":true},{"id":"p","machine":"local","dir":"/tmp/x","cron":"* * * * *","prompt":"hi","enabled":true},{"id":"q","machine":"local","dir":"/tmp/y","cron":"0 9 * * *","prompt":"hi","enabled":"true"}]`)
	if err := os.WriteFile(filepath.Join(dir, file), body, 0o600); err != nil {
		t.Fatal(err)
	}
	fired := 0
	st := New(dir, func(string, string, string) (string, error) { fired++; return "p", nil }, nil)
	st.now = func() time.Time { return at("2026-10-07 09:00") }
	if list := st.List(); len(list) != 0 {
		t.Fatalf("a file read only in part still runs %d schedules", len(list))
	}
	st.Tick()
	st.Wait()
	if _, err := st.Put(Schedule{Machine: "local", Dir: "/tmp/z", Cron: "0 9 * * *", Prompt: "new"}); err == nil || !strings.Contains(err.Error(), "unreadable") {
		t.Fatalf("a save over an unreadable file: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, file)); string(got) != string(body) || fired != 0 {
		t.Fatalf("fired %d, the file now %s", fired, got)
	}
}

func TestPutChecksTheKind(t *testing.T) {
	st := New(t.TempDir(), nil, nil)
	if _, err := st.Put(Schedule{Kind: gone, Cron: "0 9 * * *", Enabled: true}); err == nil {
		t.Fatal("a standup digest was kept")
	}
	if _, err := st.Put(Schedule{Cron: "0 9 * * *", Prompt: "hi"}); err == nil {
		t.Fatal("a prompt without a machine was kept")
	}
	if _, err := st.Put(Schedule{Kind: "coffee", Cron: "0 9 * * *"}); err == nil {
		t.Fatal("an unknown kind was kept")
	}
	prompt, err := st.Put(Schedule{Machine: "local", Dir: "/tmp/x", Cron: "0 9 * * *", Prompt: "hi"})
	if err != nil || prompt.Kind != KindPrompt {
		t.Fatalf("prompt %+v %v", prompt, err)
	}
}

// Vixie cron reads a day field that starts with "*" as unrestricted, a step like "*/2" too: such a day-of-month and a
// restricted day-of-week fire only on the days both allow.
func TestCronStepInADayFieldReadsAsStarLikeVixie(t *testing.T) {
	c, err := Parse("0 9 */2 * 1") // an odd day of the month that is a Monday
	if err != nil {
		t.Fatal(err)
	}
	if c.Matches(at("2026-10-07 09:00")) { // the 7th, a Wednesday
		t.Fatal("an odd day that is no Monday matched")
	}
	if !c.Matches(at("2026-10-05 09:00")) { // the 5th, a Monday
		t.Fatal("an odd Monday did not match")
	}
	if c.Matches(at("2026-10-12 09:00")) { // the 12th, a Monday
		t.Fatal("an even Monday matched")
	}
}

// The night a clock falls back reads 01:30 twice; a schedule at 01:30 runs once that night, and the list says its next
// run is the next night's.
func TestTickFiresOnceInTheHourAFallBackRepeats(t *testing.T) {
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no zone database:", err)
	}
	var mu sync.Mutex
	sent := 0
	st := New(t.TempDir(), func(machine, dir, text string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		sent++
		return "pane", nil
	}, nil)
	now := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC).In(newYork) // 01:30 EDT
	st.now = func() time.Time { return now }
	if _, err := st.Put(Schedule{Machine: "local", Dir: "/tmp/x", Cron: "30 1 * * *", Prompt: "status", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	st.Tick()
	st.Wait()
	now = time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC).In(newYork) // 01:30 EST, an hour later
	st.Tick()
	st.Wait()
	mu.Lock()
	defer mu.Unlock()
	if sent != 1 {
		t.Fatalf("the 01:30 schedule ran %d times the night the clock fell back", sent)
	}
	now = time.Date(2026, 11, 1, 5, 45, 0, 0, time.UTC).In(newYork) // 01:45 EDT, after the run
	if next := st.List()[0].Next; !next.Equal(time.Date(2026, 11, 2, 6, 30, 0, 0, time.UTC)) {
		t.Fatalf("next %v, want the next night's 01:30", next.In(newYork))
	}
}
