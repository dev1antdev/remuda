package usage

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

// fakeDays answers a read with one response a day in folder -p from the day asked to the clock's, and notes each ask.
type fakeDays struct {
	mu    sync.Mutex
	asked []time.Time
	clock *time.Time
	fail  error
}

func (f *fakeDays) read(_ shell.Host, since time.Time, loc *time.Location, _ string) (Report, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, since)
	if f.fail != nil {
		return Report{}, f.fail
	}
	var rep Report
	for day := since; !day.After(*f.clock); day = day.AddDate(0, 0, 1) {
		at := day.Add(10 * time.Hour).Unix()
		rep.Daily = append(rep.Daily, Day{Date: day.In(loc).Format("2006-01-02"), Folders: []DayFolder{{
			Project: "-p", Dir: "/p", Cost: 1, Tokens: 100, Turns: 2,
			Models:   map[string]int64{"claude-opus-5-5": 100},
			Sessions: map[string]Span{"s1": {First: at, Last: at + 60}},
		}}})
	}
	return rep, nil
}

func (f *fakeDays) asks() []time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]time.Time(nil), f.asked...)
}

func newTestBook(t *testing.T, dir string, now *time.Time) (*Book, *fakeDays) {
	t.Helper()
	fake := &fakeDays{clock: now}
	book := OpenBook(dir)
	book.Read = fake.read
	book.Now = func() time.Time { return *now }
	return book, fake
}

func TestAFirstLookReadsTheYearInTheBackgroundAndKeepsTheDaysThatEnded(t *testing.T) {
	dir := t.TempDir()
	kyiv, _ := time.LoadLocation("Europe/Kyiv")
	now := time.Date(2026, 11, 13, 15, 0, 0, 0, kyiv)
	book, fake := newTestBook(t, dir, &now)
	gog := []YearHost{{Host: &shell.Fake{Name: "gog"}}}

	first := book.Look(gog, kyiv, "Europe/Kyiv")
	if first.From != "2025-11-14" || first.To != "2026-11-13" || first.Machines[0].Reading != YearDays-1 || len(first.Machines[0].Days) != 0 {
		t.Fatalf("a first look: %+v", first)
	}
	book.Wait()
	if asks := fake.asks(); len(asks) != 1 || !asks[0].Equal(time.Date(2025, 11, 14, 0, 0, 0, 0, kyiv)) {
		t.Fatalf("asked %v", asks)
	}
	year := book.Look(gog, kyiv, "Europe/Kyiv")
	machine := year.Machines[0]
	if machine.Reading != 0 || len(machine.Days) != YearDays || machine.Today == "" || len(fake.asks()) != 1 {
		t.Fatalf("a second look reads nothing: reading %d, %d days, asked %d", machine.Reading, len(machine.Days), len(fake.asks()))
	}
	if len(machine.Folders) != 1 || machine.Folders[0].Sessions != 1 || machine.Folders[0].Longest != int64((YearDays-1)*86400+60) {
		t.Fatalf("one session a year long: %+v", machine.Folders)
	}
	if year.Names["claude-opus-5-5"] != "Opus 5.5" {
		t.Fatalf("names %v", year.Names)
	}

	now = now.Add(TodayFresh + time.Second)
	book.Look(gog, kyiv, "Europe/Kyiv")
	book.Wait()
	if asks := fake.asks(); len(asks) != 2 || !asks[1].Equal(time.Date(2026, 11, 13, 0, 0, 0, 0, kyiv)) {
		t.Fatalf("today read again from its midnight alone: %v", asks)
	}

	now = time.Date(2026, 11, 14, 0, 5, 0, 0, kyiv)
	if year := book.Look(gog, kyiv, "Europe/Kyiv"); year.Machines[0].Reading != 1 {
		t.Fatalf("yesterday, ended, is read once more: %+v", year.Machines[0])
	}
	book.Wait()
	if asks := fake.asks(); len(asks) != 3 || !asks[2].Equal(time.Date(2026, 11, 13, 0, 0, 0, 0, kyiv)) {
		t.Fatalf("asked %v", asks)
	}

	again, fresh := newTestBook(t, dir, &now)
	year = again.Look(gog, kyiv, "Europe/Kyiv")
	again.Wait()
	if year.Machines[0].Reading != 0 || len(year.Machines[0].Days) != YearDays-1 {
		t.Fatalf("the kept days outlive a restart: reading %d, %d days", year.Machines[0].Reading, len(year.Machines[0].Days))
	}
	if asks := fresh.asks(); len(asks) != 1 || !asks[0].Equal(time.Date(2026, 11, 14, 0, 0, 0, 0, kyiv)) {
		t.Fatalf("after a restart only today is read: %v", asks)
	}
	if info, err := os.Stat(filepath.Join(dir, bookFile)); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the book's file: %v %v", info, err)
	}
}

func TestAMachineAwayIsAnsweredFromWhatWasKeptAndAFailedReadIsTriedAgainLater(t *testing.T) {
	now := time.Date(2026, 11, 13, 15, 0, 0, 0, time.UTC)
	book, fake := newTestBook(t, t.TempDir(), &now)
	away := []YearHost{{Host: &shell.Fake{Name: "pi"}, Away: true}}
	if year := book.Look(away, time.UTC, "UTC"); year.Machines[0].Reading != 0 || !year.Machines[0].Away || len(fake.asks()) != 0 {
		t.Fatalf("a machine away is read nothing: %+v", year.Machines[0])
	}

	fake.fail = errors.New("ssh: connection refused")
	here := []YearHost{{Host: &shell.Fake{Name: "pi"}}}
	book.Look(here, time.UTC, "UTC")
	book.Wait()
	if year := book.Look(here, time.UTC, "UTC"); year.Machines[0].Error == "" || len(fake.asks()) != 1 {
		t.Fatalf("a failed read says why and waits: %+v, asked %d", year.Machines[0], len(fake.asks()))
	}
	fake.fail = nil
	now = now.Add(RetryAfter + time.Second)
	book.Look(here, time.UTC, "UTC")
	book.Wait()
	if year := book.Look(here, time.UTC, "UTC"); year.Machines[0].Error != "" || len(year.Machines[0].Days) != YearDays {
		t.Fatalf("tried again: %+v", year.Machines[0])
	}

	book.Forget("pi")
	if year := book.Look(away, time.UTC, "UTC"); len(year.Machines[0].Days) != 0 {
		t.Fatal("a removed machine's days are forgotten")
	}
}

func TestDaysOlderThanAYearAreDropped(t *testing.T) {
	now := time.Date(2026, 11, 13, 15, 0, 0, 0, time.UTC)
	book, _ := newTestBook(t, t.TempDir(), &now)
	gog := []YearHost{{Host: &shell.Fake{Name: "gog"}}}
	book.Look(gog, time.UTC, "UTC")
	book.Wait()
	now = now.AddDate(0, 0, 10)
	book.Look(gog, time.UTC, "UTC")
	book.Wait()
	book.mu.Lock()
	defer book.mu.Unlock()
	days := book.shelves[shelfKey("gog", "UTC")].Days
	if len(days) != YearDays-1 {
		t.Fatalf("%d days kept", len(days))
	}
	if _, kept := days["2025-11-14"]; kept {
		t.Fatal("a day past the year is kept")
	}
}
