package budget

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/transcript"
)

// rootedLocal is a machine of the fleet as the spend reader asks it: one whose reader knows the folders its agents named.
type rootedLocal struct {
	shell.Host
	roots transcript.Roots
}

func (h rootedLocal) TranscriptRoots() transcript.Roots { return h.roots }

// TestABudgetCountsTheFolderALearnedRunLeft: a budget's spend is read through the same session folders the weekly report
// reads, so a folder an agent named is counted once that agent stopped and remuda restarted.
func TestABudgetCountsTheFolderItsReaderKeptAcrossARestart(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projects := filepath.Join(t.TempDir(), "profile", "projects")
	if err := os.MkdirAll(filepath.Join(projects, "-p-a"), 0o755); err != nil {
		t.Fatal(err)
	}
	record := `{"type":"assistant","timestamp":"2026-10-06T10:00:00Z","cwd":"/p/a","message":{"id":"m1","model":"claude-fable-5-1","usage":{"input_tokens":100,"output_tokens":10,"cache_read_input_tokens":1000,"cache_creation_input_tokens":0}}}` + "\n"
	if err := os.WriteFile(filepath.Join(projects, "-p-a", "s1.jsonl"), []byte(record), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	transcript.OpenRootStore(dir).Keep("local", transcript.KeptRoots{Roots: map[transcript.Format][]string{transcript.Claude: {projects}}})
	local := shell.Local{Name: "local"}
	host := rootedLocal{Host: local, roots: transcript.NewReader(local).Keeping(transcript.OpenRootStore(dir)).Roots()}

	loc := kyiv(t)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, loc)
	s := New(dir, func() []shell.Host { return []shell.Host{host} }, nil)
	s.Loc, s.now = loc, func() time.Time { return now }
	if _, err := s.Replace([]Budget{{ID: "all", Limit: 100, Period: "month"}}); err != nil {
		t.Fatal(err)
	}
	s.Wait()
	st := s.Status()
	if len(st) != 1 || st[0].Spent <= 0 || len(st[0].Missing) != 0 {
		t.Fatalf("the folder its reader kept was not counted: spent %.4f, missing %v", st[0].Spent, st[0].Missing)
	}
}
