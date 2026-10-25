package budget

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/usage"
)

func kyiv(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Skip("no zone database")
	}
	return loc
}

func TestPeriodStart(t *testing.T) {
	loc := kyiv(t)
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", s, loc)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	cases := []struct{ period, now, want string }{
		{"day", "2026-10-25 13:40", "2026-10-25 00:00"},
		{"week", "2026-10-25 13:40", "2026-10-19 00:00"}, // a Sunday goes back to Monday
		{"week", "2026-10-19 00:30", "2026-10-19 00:00"}, // a Monday is its own start
		{"month", "2026-10-25 13:40", "2026-10-01 00:00"},
		{"day", "2026-10-25 02:30", "2026-10-25 00:00"},   // the clocks went back at 04:00 that night in Kyiv
		{"week", "2026-01-02 09:00", "2025-12-29 00:00"},  // a week across the year end
		{"month", "2026-01-01 00:00", "2026-01-01 00:00"}, // the first moment of a month is its start
		{"month", "2025-12-31 23:59", "2025-12-01 00:00"},
	}
	for _, c := range cases {
		got := PeriodStart(c.period, at(c.now), loc)
		if got.Format("2006-01-02 15:04") != c.want || got.Location() != loc {
			t.Errorf("PeriodStart(%s, %s) = %s, want %s", c.period, c.now, got.Format("2006-01-02 15:04 -07"), c.want)
		}
	}
	utc := PeriodStart("day", at("2026-10-25 01:00").UTC(), loc)
	if utc.Day() != 25 {
		t.Errorf("a UTC instant is read by loc's clock, got day %d", utc.Day())
	}
}

type notes struct {
	mu   sync.Mutex
	list []push.Payload
}

func (n *notes) add(p push.Payload) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.list = append(n.list, p)
}

func (n *notes) all() []push.Payload {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]push.Payload(nil), n.list...)
}

// fakeSpend builds a Service whose readings come from the reports given per machine, with a clock of its own.
func fakeSpend(t *testing.T, reports map[string]*usage.Report) (*Service, *notes, *time.Time) {
	t.Helper()
	loc := kyiv(t)
	now := time.Date(2026, 10, 25, 13, 40, 0, 0, loc)
	hosts := []shell.Host{&shell.Fake{Name: "local", Dir: "/Users/me"}, &shell.Fake{Name: "gog", Dir: "/home/me", Remote: true}}
	n := &notes{}
	s := New(t.TempDir(), func() []shell.Host { return hosts }, n.add)
	s.Loc = loc
	s.now = func() time.Time { return now }
	s.collect = func(_ []shell.Host, since time.Time, _ *time.Location, _ string) []usage.MachineReport {
		var out []usage.MachineReport
		for id, rep := range reports {
			out = append(out, usage.MachineReport{ID: id, Label: id, Report: rep})
		}
		out = append(out, usage.MachineReport{ID: "roofr", Error: "no remuda of this version on the machine"})
		return out
	}
	return s, n, &now
}

func report(total float64, projects ...usage.ProjectTotals) *usage.Report {
	rep := &usage.Report{Projects: projects}
	rep.Total.Cost = total
	return rep
}

func TestReplaceValidates(t *testing.T) {
	s, _, _ := fakeSpend(t, nil)
	bad := []struct {
		name string
		b    Budget
	}{
		{"period", Budget{Limit: 5, Period: "fortnight"}},
		{"limit", Budget{Limit: 0, Period: "day"}},
		{"dir without machine", Budget{Limit: 5, Period: "day", Dir: "/x"}},
	}
	for _, c := range bad {
		if _, err := s.Replace([]Budget{c.b}); err == nil {
			t.Errorf("%s: accepted", c.name)
		}
	}
	out, err := s.Replace([]Budget{{Limit: 20, Period: " week ", Note: " weekly "}, {ID: "same", Limit: 5, Period: "day"}, {ID: "same", Limit: 6, Period: "day", Machine: "gog", Dir: "/home/me/remuda"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 || out[0].ID == "" || out[0].Period != "week" || out[0].Note != "weekly" {
		t.Fatalf("Replace trimmed and named wrongly: %+v", out)
	}
	if out[1].ID == out[2].ID {
		t.Errorf("a repeated id was kept twice")
	}
	again := New(s.dir, s.Hosts, nil)
	if len(again.List()) != 3 {
		t.Errorf("the file holds %d budgets, want 3", len(again.List()))
	}
}

func TestStatusAndAlerts(t *testing.T) {
	reports := map[string]*usage.Report{
		"local": report(16.40, usage.ProjectTotals{Project: "-Users-me-remuda", Dir: "/Users/me/remuda", Totals: usage.Totals{Cost: 12}}, usage.ProjectTotals{Project: "-Users-me-other", Dir: "", Totals: usage.Totals{Cost: 4.40}}),
		"gog":   report(3),
	}
	s, n, now := fakeSpend(t, reports)
	if _, err := s.Replace([]Budget{
		{ID: "all", Limit: 20, Period: "week", Note: "the week"},
		{ID: "mac", Limit: 100, Period: "day", Machine: "local"},
		{ID: "dir", Limit: 10, Period: "month", Machine: "local", Dir: "/Users/me/remuda"},
		{ID: "enc", Limit: 10, Period: "month", Machine: "local", Dir: "/Users/me/other"},
	}); err != nil {
		t.Fatal(err)
	}
	s.refresh() // the one Replace started may still run; this one is synchronous and idempotent
	waitIdle(s)
	st := byID(s.Status())
	if st["all"].Spent != 19.40 || st["all"].Collected == "" {
		t.Errorf("every machine: spent %.2f collected %q", st["all"].Spent, st["all"].Collected)
	}
	if st["mac"].Spent != 16.40 || st["mac"].Missing != nil {
		t.Errorf("one machine: spent %.2f missing %v", st["mac"].Spent, st["mac"].Missing)
	}
	if len(st["all"].Missing) != 1 || st["all"].Missing[0] != "roofr" {
		t.Errorf("the machine without a report is not named: %v", st["all"].Missing)
	}
	if st["dir"].Spent != 12 {
		t.Errorf("a folder by its path: spent %.2f", st["dir"].Spent)
	}
	if st["enc"].Spent != 4.40 {
		t.Errorf("a folder by its encoded project: spent %.2f", st["enc"].Spent)
	}
	if st["all"].Start != "2026-10-19T00:00:00+03:00" {
		t.Errorf("the week's start is %s", st["all"].Start)
	}

	fired := n.all()
	titles := map[string]string{}
	for _, p := range fired {
		if p.Kind != "budget" {
			t.Errorf("kind %q", p.Kind)
		}
		titles[p.Tag] = p.Title + " / " + p.Body
	}
	if len(fired) != 2 {
		t.Fatalf("fired %d alerts, want 2 (the week at 97 %%, the folder over its limit): %v", len(fired), titles)
	}
	if titles["budget|all"] != "Budget at 97 %: $19.4 of $20.0 this week / the week" {
		t.Errorf("week alert: %q", titles["budget|all"])
	}
	if titles["budget|dir"] != "Budget crossed: $12.0 of $10.0 this month / local · remuda" {
		t.Errorf("folder alert: %q", titles["budget|dir"])
	}

	s.refresh()
	if len(n.all()) != 2 {
		t.Errorf("a second reading fired again: %d", len(n.all()))
	}
	for _, b := range s.List() {
		if b.ID == "dir" && !(contains(b.Alerted["2026-10-01T00:00:00+03:00"], 80) && contains(b.Alerted["2026-10-01T00:00:00+03:00"], 100)) {
			t.Errorf("the crossed budget did not note both thresholds: %v", b.Alerted)
		}
	}

	reports["local"] = report(21.30, usage.ProjectTotals{Project: "-Users-me-remuda", Dir: "/Users/me/remuda", Totals: usage.Totals{Cost: 12}})
	s.refresh()
	last := n.all()
	if len(last) != 3 || last[2].Title != "Budget crossed: $24.3 of $20.0 this week" {
		t.Errorf("crossing the week's limit later: %v", last)
	}

	*now = now.Add(8 * 24 * time.Hour) // the next week, in the next month: both alerts are due again, the old periods forgotten
	s.refresh()
	again := n.all()
	if len(again) != 5 || !strings.HasPrefix(again[3].Title, "Budget crossed") || again[3].Tag != "budget|all" || again[4].Tag != "budget|dir" {
		t.Errorf("a new period did not fire anew: %v", again)
	}
	for _, b := range s.List() {
		if b.ID == "all" && len(b.Alerted) != 1 {
			t.Errorf("the old period's note stayed: %v", b.Alerted)
		}
	}
}

func TestEditResetsAlerts(t *testing.T) {
	s, n, _ := fakeSpend(t, map[string]*usage.Report{"local": report(9)})
	if _, err := s.Replace([]Budget{{ID: "a", Limit: 10, Period: "day"}}); err != nil {
		t.Fatal(err)
	}
	s.refresh()
	waitIdle(s)
	if len(n.all()) != 1 {
		t.Fatalf("fired %d, want the 80 %% one", len(n.all()))
	}
	if _, err := s.Replace([]Budget{{ID: "a", Limit: 10, Period: "day", Note: "renamed"}}); err != nil {
		t.Fatal(err)
	}
	waitIdle(s)
	s.refresh()
	if len(n.all()) != 1 {
		t.Errorf("a note change fired again: %d", len(n.all()))
	}
	if _, err := s.Replace([]Budget{{ID: "a", Limit: 11, Period: "day"}}); err != nil {
		t.Fatal(err)
	}
	waitIdle(s)
	s.refresh()
	if len(n.all()) != 2 {
		t.Errorf("a raised limit did not start afresh: %d", len(n.all()))
	}
}

func TestStatusWithoutReading(t *testing.T) {
	s, _, _ := fakeSpend(t, nil)
	s.collect = func([]shell.Host, time.Time, *time.Location, string) []usage.MachineReport { return nil }
	s.mu.Lock()
	s.all = []Budget{{ID: "a", Limit: 10, Period: "day"}}
	s.mu.Unlock()
	st := s.Status()
	if len(st) != 1 || st[0].Collected != "" || st[0].Spent != 0 {
		t.Errorf("an unread period must answer without a figure: %+v", st)
	}
	waitIdle(s)
}

func TestWords(t *testing.T) {
	if USD(0.5) != "$0.50" || USD(16.4) != "$16.4" || USD(123.4) != "$123" {
		t.Errorf("USD: %s %s %s", USD(0.5), USD(16.4), USD(123.4))
	}
	label := func(id string) string { return strings.ToUpper(id) }
	if Scope(Budget{}, label) != "every machine" || Scope(Budget{Machine: "gog"}, label) != "GOG" || Scope(Budget{Machine: "gog", Dir: "/home/me/remuda"}, label) != "GOG · remuda" {
		t.Error("Scope words")
	}
}

func byID(list []Status) map[string]Status {
	out := map[string]Status{}
	for _, st := range list {
		out[st.ID] = st
	}
	return out
}

// waitIdle lets the reading a Replace or Status started in the background finish.
func waitIdle(s *Service) {
	for i := 0; i < 200; i++ {
		s.mu.Lock()
		busy := s.inflight
		s.mu.Unlock()
		if !busy {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}
