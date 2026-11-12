package timelapse

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// screensAt is a finished recording of a pane from whole screens, one every 4 s, started at the minute after t0
func screensAt(machine, pane string, minute int, screens ...string) Recording {
	start := t0.Add(time.Duration(minute) * time.Minute)
	var frames []frame
	for i, screen := range screens {
		frames = append(frames, frame{at: start.Add(time.Duration(i) * 4 * time.Second), rows: strings.Split(screen, "\n")})
	}
	meta := Meta{ID: newID(start), Machine: machine, Pane: pane, Workspace: "app", Dir: "/repo/app", Folder: "/repo", Agent: "claude", Start: start, End: start.Add(2 * time.Minute), Count: len(frames), Ended: "done"}
	return Recording{Meta: meta, Frames: encode(frames, start)}
}

func written(t *testing.T, r *Recorder, recs ...Recording) {
	t.Helper()
	for _, rec := range recs {
		if err := r.write(rec); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTheTextIsFoundWhereItFirstStoodHowLongItStayedAndWhenItCameBack(t *testing.T) {
	r := Open(t.TempDir(), Inputs{})
	rec := screensAt("local", "w1:p1", 0,
		"$ make migrate\n",
		"$ make migrate\n\x1b[31mMigration\x1b[0m failed: users",
		"\x1b[31mMigration\x1b[0m failed: users\n> ",   // scrolled up a row: still on screen
		"\x1b[1m> fixing\x1b[0m\n",                     // gone
		"> fixing\nmigration FAILED again\n\n",         // back
		"> fixing\nmigration FAILED again\n\n\x1b[2m…", // still there, a row below it moving
	)
	written(t, r, rec)
	found := r.Search(context.Background(), SearchQuery{Text: "migration failed"})
	if found.Searched != 1 || found.Unread != 0 || found.Cut != "" || len(found.Hits) != 1 {
		t.Fatalf("%+v", found)
	}
	hit := found.Hits[0]
	if hit.Recording != rec.ID || hit.Pane != "w1:p1" || hit.Workspace != "app" || hit.Folder != "/repo" || !hit.Start.Equal(rec.Start) {
		t.Fatalf("named %+v", hit)
	}
	if hit.First != (Appearance{Frame: 1, At: 4000, For: 8000, Line: 1}) {
		t.Fatalf("first %+v", hit.First)
	}
	if hit.Times != 2 || len(hit.Again) != 1 || hit.Again[0] != (Appearance{Frame: 4, At: 16000, For: 120000 - 16000, Line: 1}) {
		t.Fatalf("again %d %+v", hit.Times, hit.Again)
	}
	if hit.Row != "Migration failed: users" || len(hit.Before) != 1 || hit.Before[0] != "$ make migrate" || len(hit.After) != 0 || hit.Col != 0 {
		t.Fatalf("excerpt %q %q %q at %d", hit.Before, hit.Row, hit.After, hit.Col)
	}
	if none := r.Search(context.Background(), SearchQuery{Text: "never printed"}); len(none.Hits) != 0 || none.Searched != 1 {
		t.Fatalf("a text never shown: %+v", none)
	}
	if colourOnly := r.Search(context.Background(), SearchQuery{Text: "[31m"}); len(colourOnly.Hits) != 0 {
		t.Fatalf("an escape's own bytes were found: %+v", colourOnly.Hits)
	}
}

func TestTheAppearancesPastTheCapAreCountedNotListed(t *testing.T) {
	r := Open(t.TempDir(), Inputs{})
	var screens []string
	for i := 0; i < 2*(MaxAgain+10); i++ {
		if i%2 == 0 {
			screens = append(screens, fmt.Sprintf("step %d\nERROR: disk full", i))
		} else {
			screens = append(screens, fmt.Sprintf("step %d\n", i))
		}
	}
	written(t, r, screensAt("local", "w1:p1", 0, screens...))
	hit := r.Search(context.Background(), SearchQuery{Text: "error: disk"}).Hits[0]
	if hit.Times != MaxAgain+10 || len(hit.Again) != MaxAgain || hit.First.For != 4000 || hit.Again[0].Frame != 2 {
		t.Fatalf("%d times, %d listed, first %+v, again %+v", hit.Times, len(hit.Again), hit.First, hit.Again[0])
	}
}

func TestAnExcerptIsCutAroundTheMatchItsNeighboursAtTheSameColumns(t *testing.T) {
	r := Open(t.TempDir(), Inputs{})
	long := strings.Repeat("ж", 200) + "Миграция упала" + strings.Repeat("-", 100)
	above := strings.Repeat("в", 400)
	written(t, r, screensAt("local", "w1:p1", 0, "\n\nfirst\nsecond\n"+above+"\nstart", "\n\nfirst\nsecond\n"+above+"\n"+long+"\nbelow\n\n"))
	hit := r.Search(context.Background(), SearchQuery{Text: "МИГРАЦИЯ"}).Hits[0]
	n := len([]rune("МИГРАЦИЯ"))
	if want := 200 - (ExcerptWidth-n)/2; hit.Col != want {
		t.Fatalf("cut at %d, want %d", hit.Col, want)
	}
	if !strings.Contains(hit.Row, "Миграция упала") || len([]rune(hit.Row)) != ExcerptWidth {
		t.Fatalf("row %q", hit.Row)
	}
	// "second" and "below" hold nothing past the cut: the one at the top and the one at the foot are left out
	if len(hit.Before) != 1 || len([]rune(hit.Before[0])) != ExcerptWidth || len(hit.After) != 0 {
		t.Fatalf("neighbours %q / %q", hit.Before, hit.After)
	}
	if hit.First.Line != 5 {
		t.Fatalf("line %d", hit.First.Line)
	}
}

func TestASearchNamesItsScopeNewestFirst(t *testing.T) {
	r := Open(t.TempDir(), Inputs{})
	old, newer, other, elsewhere := screensAt("local", "w1:p1", 0, "a", "panic: x"), screensAt("local", "w1:p2", 10, "a", "panic: y"), screensAt("gog", "w1:p1", 20, "a", "panic: z"), screensAt("local", "w3:p1", 30, "a", "panic: w")
	elsewhere.Dir = "/repo/other"
	written(t, r, old, newer, other, elsewhere)
	ids := func(q SearchQuery) string {
		var out []string
		for _, hit := range r.Search(context.Background(), q).Hits {
			out = append(out, hit.Recording)
		}
		return strings.Join(out, " ")
	}
	if got, want := ids(SearchQuery{Text: "panic"}), strings.Join([]string{elsewhere.ID, other.ID, newer.ID, old.ID}, " "); got != want {
		t.Fatalf("every recording: %s, want %s", got, want)
	}
	if got := ids(SearchQuery{Text: "panic", Machine: "local", Pane: "w1:p1"}); got != old.ID {
		t.Fatalf("a pane: %s", got)
	}
	if got := ids(SearchQuery{Text: "panic", Machine: "local", Pane: "w9:p9", Dir: "/repo/app"}); got != newer.ID+" "+old.ID {
		t.Fatalf("a pane and its folder: %s", got)
	}
	if got := ids(SearchQuery{Text: "panic", Since: newer.End.Add(-time.Second)}); got != elsewhere.ID+" "+other.ID+" "+newer.ID {
		t.Fatalf("since: %s", got)
	}
	live := &take{meta: screensAt("local", "w5:p1", 40, "a").Meta}
	r.mu.Lock()
	r.live[key("local", "w5:p1")] = live
	r.mu.Unlock()
	written(t, r, Recording{Meta: live.meta, Frames: screensAt("local", "w5:p1", 40, "a", "panic: live").Frames})
	if got := ids(SearchQuery{Text: "panic: live"}); got != "" {
		t.Fatalf("a turn still going was searched: %s", got)
	}
}

func TestAQueryReadsNoMoreThanItsBudgetAndTheCacheCostsNothing(t *testing.T) {
	dir := t.TempDir()
	r := Open(dir, Inputs{})
	var recs []Recording
	for minute := 0; minute < 4; minute++ {
		recs = append(recs, screensAt("local", fmt.Sprintf("w%d:p1", minute), minute*10, "a", "deploy failed"))
	}
	written(t, r, recs...)
	r.searchBudget = 1 // the newest is read whatever its size, then nothing more off the disk
	cold := r.Search(context.Background(), SearchQuery{Text: "deploy failed"})
	if cold.Searched != 1 || cold.Unread != 3 || cold.Cut != CutBudget || len(cold.Hits) != 1 || cold.Hits[0].Recording != recs[3].ID {
		t.Fatalf("cold: %+v", cold)
	}
	r.Search(context.Background(), SearchQuery{Text: "deploy failed", Machine: "local", Pane: "w0:p1"}) // the oldest into the cache
	// the newest costs nothing now, so the next one is read; the one after is past the budget, the oldest in the cache
	further := r.Search(context.Background(), SearchQuery{Text: "deploy failed"})
	if further.Searched != 3 || further.Unread != 1 || further.Cut != CutBudget || len(further.Hits) != 3 || further.Hits[1].Recording != recs[2].ID || further.Hits[2].Recording != recs[0].ID {
		t.Fatalf("the same query again: %+v", further)
	}
	r.searchBudget = SearchBudget
	if warm := r.Search(context.Background(), SearchQuery{Text: "deploy failed"}); warm.Searched != 4 || warm.Cut != "" {
		t.Fatalf("within the budget: %+v", warm)
	}
	r.searchBudget = 1
	if cached := r.Search(context.Background(), SearchQuery{Text: "failed"}); cached.Searched != 4 || cached.Cut != "" || len(cached.Hits) != 4 {
		t.Fatalf("every recording in the cache, the budget untouched: %+v", cached)
	}
	again := screensAt("local", "w3:p1", 30, "a", "deploy failed", "rolled back at last")
	again.ID = recs[3].ID
	written(t, r, again) // the turn's end overwrites its checkpoint: another version of the file
	if moved := r.Search(context.Background(), SearchQuery{Text: "rolled back"}); len(moved.Hits) != 1 || moved.Hits[0].First.Frame != 2 {
		t.Fatalf("the new version was not read: %+v", moved)
	}
	if err := r.Delete(recs[2].ID); err != nil {
		t.Fatal(err)
	}
	if r.plain.get(recs[2].ID, version{}) != nil || len(r.plain.held) != 3 {
		t.Fatalf("a deleted recording's text stays in the cache: %d held", len(r.plain.held))
	}
	if _, err := r.DeleteAll(); err != nil || len(r.plain.held) != 0 {
		t.Fatalf("%v; %d held after every recording went", err, len(r.plain.held))
	}
}

func TestAQueryStopsAtItsTimeLimitAndNeverHoldsTheRecorder(t *testing.T) {
	r := Open(t.TempDir(), Inputs{})
	for minute := 0; minute < 5; minute++ {
		written(t, r, screensAt("local", fmt.Sprintf("w%d:p1", minute), minute*10, "a", "timeout"))
	}
	clock, free, calls := t0, true, 0
	r.now = func() time.Time {
		if r.mu.TryLock() {
			r.mu.Unlock()
		} else {
			free = false
		}
		calls++
		clock = clock.Add(time.Second)
		return clock
	}
	r.searchFor = 2500 * time.Millisecond
	found := r.Search(context.Background(), SearchQuery{Text: "timeout"})
	if found.Searched != 2 || found.Unread != 3 || found.Cut != CutTime || len(found.Hits) != 2 {
		t.Fatalf("%+v after %d looks at the clock", found, calls)
	}
	if !free {
		t.Fatal("the search held the recorder's lock while it read")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if gone := r.Search(ctx, SearchQuery{Text: "timeout"}); gone.Searched != 0 || gone.Unread != 5 {
		t.Fatalf("a query whose page went away: %+v", gone)
	}
}

func TestARecordingIsLeftHalfWayWhenTheClockRunsOutInIt(t *testing.T) {
	var screens []string
	for i := 0; i < 3*checkEvery; i++ {
		screens = append(screens, fmt.Sprintf("frame %d", i))
	}
	plain := plainOf(screensAt("local", "w1:p1", 0, screens...))
	looks := 0
	if seen, stopped := sight(plain, "frame 700", "frame 700", func() bool { looks++; return looks > 1 }); !stopped || seen != nil || looks != 2 {
		t.Fatalf("stopped %v after %d looks: %+v", stopped, looks, seen)
	}
	if seen, stopped := sight(plain, "frame 700", "frame 700", func() bool { return false }); stopped || seen == nil || seen.first.Frame != 700 {
		t.Fatalf("with time to spare: %v %+v", stopped, seen)
	}
}

func TestOneQueryAnswersAtMostMaxHitsRecordings(t *testing.T) {
	r := Open(t.TempDir(), Inputs{})
	for i := 0; i < MaxHits+2; i++ {
		written(t, r, screensAt("local", fmt.Sprintf("w%d:p1", i), i, "a", "OOM killed"))
	}
	found := r.Search(context.Background(), SearchQuery{Text: "oom"})
	if len(found.Hits) != MaxHits || !found.More || found.Unread != 2 || found.Searched != MaxHits {
		t.Fatalf("%d hits, more %v, unread %d, searched %d", len(found.Hits), found.More, found.Unread, found.Searched)
	}
}

func TestSearchesRunBesideTheRecorderWritingAndDeleting(t *testing.T) {
	r := Open(t.TempDir(), Inputs{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for minute := 0; minute < 30; minute++ {
			rec := screensAt("local", fmt.Sprintf("w%d:p1", minute%5), minute, "a", "build failed")
			if err := r.write(rec); err != nil {
				t.Error(err)
				return
			}
			if minute%3 == 0 {
				r.Delete(rec.ID)
			}
		}
	}()
	for i := 0; i < 30; i++ {
		for _, hit := range r.Search(context.Background(), SearchQuery{Text: "build failed"}).Hits {
			if hit.Row != "build failed" {
				t.Fatalf("a torn read: %+v", hit)
			}
		}
	}
	<-done
	kept := r.List("local", "", "/repo/app")
	if found := r.Search(context.Background(), SearchQuery{Text: "build failed"}); len(found.Hits) != len(kept) || found.Searched != len(kept) {
		t.Fatalf("%d hits, %d searched, of %d recordings", len(found.Hits), found.Searched, len(kept))
	}
}
