package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/transcript"
)

// keptByARun is a machine whose reader, from an earlier run of remuda, left the folders given in the state dir: what a
// restart sees for a conversation whose agent has stopped.
func keptByARun(t *testing.T, dir, home string, folders ...string) rootedHost {
	t.Helper()
	kept := transcript.KeptRoots{Roots: map[transcript.Format][]string{transcript.Claude: folders}}
	transcript.OpenRootStore(dir).Keep("gog", kept)
	local := shell.Local{Name: "gog"}
	t.Setenv("HOME", home)
	return rootedHost{Host: local, roots: transcript.NewReader(local).Keeping(transcript.OpenRootStore(dir)).Roots()}
}

// TestAWeeklyReportReadsTheFolderALearnedRunLeft: a Claude Code started with CLAUDE_CONFIG_DIR keeps its transcripts
// there; once the agent stopped and remuda restarted, the weekly report still counts them, because the reader kept the
// folder an accepted process read named.
func TestAWeeklyReportReadsTheFolderItsReaderKeptAcrossARestart(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(t.TempDir(), "profile")
	projects := filepath.Join(profile, "projects")
	if err := os.MkdirAll(filepath.Join(projects, "-p-a"), 0o755); err != nil {
		t.Fatal(err)
	}
	record := rec("m1", "claude-fable-5-1", "2026-10-06T10:00:00Z", 10)
	if err := os.WriteFile(filepath.Join(projects, "-p-a", "s1.jsonl"), []byte(record), 0o644); err != nil {
		t.Fatal(err)
	}
	host := keptByARun(t, t.TempDir(), home, projects)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	rep, err := ReadHost(host, now.AddDate(0, 0, -7), time.UTC, "", false, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Files != 1 || rep.Total.Turns == 0 {
		t.Fatalf("the folder its reader kept was not read: %+v", rep.Total)
	}
}

// TestAYearKeepsItsDaysWhenTheReaderRestarts: the reader a restart builds offers the same folders it kept, so the year
// is not read again and the days are kept.
func TestAYearKeepsItsDaysWhenTheReaderRestarts(t *testing.T) {
	now := time.Date(2026, 11, 13, 15, 0, 0, 0, time.UTC)
	dir, home := t.TempDir(), t.TempDir()
	profile := filepath.Join(home, "profile", "projects")
	book, fake := newTestBook(t, dir, &now)
	gog := []YearHost{{Host: keptByARun(t, dir, home, profile)}}
	book.Look(gog, time.UTC, "UTC")
	book.Wait()
	if asks := fake.asks(); len(asks) != 1 {
		t.Fatalf("a first year: %d asks", len(asks))
	}

	// a restart: another book from the state dir, another reader from the kept set — the same folders
	next, nextFake := newTestBook(t, dir, &now)
	next.Look([]YearHost{{Host: keptByARun(t, dir, home, profile)}}, time.UTC, "UTC")
	next.Wait()
	// today is read again, as a restart always does; the year that ended is not
	if asks := nextFake.asks(); len(asks) != 1 || !asks[0].Equal(time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("a restart read more than today again: %v", asks)
	}
	if days := next.shelves[shelfKey("gog", "UTC")].Days; len(days) != YearDays-1 {
		t.Fatalf("a restart dropped the kept year: %d days", len(days))
	}
}
