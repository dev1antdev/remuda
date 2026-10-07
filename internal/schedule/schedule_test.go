package schedule

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/herdr-ui/internal/push"
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
	st.Tick() // the same minute: not again
	if n := count(); n != 1 {
		t.Fatalf("sent %d", n)
	}
	now = now.Add(time.Minute)
	st.Tick() // 09:01: no match
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
