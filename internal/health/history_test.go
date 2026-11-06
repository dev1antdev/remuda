package health

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNoteKeepsADayAndReplacesAQuickRead(t *testing.T) {
	start := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	var samples []Sample
	for i := 0; i < HistoryKeep+5; i++ {
		samples = note(samples, Sample{At: start.Add(time.Duration(i) * 15 * time.Minute), Load: float64(i), DiskFree: 60})
	}
	if len(samples) != HistoryKeep || samples[0].Load != 5 || samples[HistoryKeep-1].Load != float64(HistoryKeep+4) {
		t.Fatalf("the oldest past %d go: %d kept, first %v, last %v", HistoryKeep, len(samples), samples[0].Load, samples[len(samples)-1].Load)
	}
	last := samples[len(samples)-1].At
	samples = note(samples, Sample{At: last.Add(20 * time.Second), Load: 1.5, DiskFree: 61})
	if len(samples) != HistoryKeep || samples[len(samples)-1].Load != 1.5 {
		t.Fatalf("a read within a minute replaces the last: %d, last %v", len(samples), samples[len(samples)-1])
	}
	samples = note(samples, Sample{At: last.Add(2 * time.Minute), Load: 2.5})
	if samples[len(samples)-1].Load != 2.5 || samples[len(samples)-2].Load != 1.5 {
		t.Fatalf("a read two minutes on is a new sample: %v", samples[len(samples)-2:])
	}
}

func TestSampleReadsOnlyAProbeThatReachedTheMachine(t *testing.T) {
	if _, ok := sample(Report{Error: "ssh: no route"}); ok {
		t.Fatal("a probe that read nothing adds a sample")
	}
	s, ok := sample(Report{At: time.Unix(1, 0), Load1: 2.5, DiskFreeGB: 120, DiskUsedPct: 41})
	if !ok || s.Load != 2.5 || s.DiskFree != 59 || s.At != time.Unix(1, 0) {
		t.Fatalf("sample: %+v %v", s, ok)
	}
}

func TestHistoryRoundTripsAndSurvivesADamagedFile(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, nil, "v1", false)
	at := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	c.remember("local", Report{At: at, Load1: 1, DiskFreeGB: 100, DiskUsedPct: 50})
	c.remember("local", Report{At: at.Add(15 * time.Minute), Load1: 2, DiskFreeGB: 99, DiskUsedPct: 51})
	c.remember("gog", Report{At: at, Error: "away"})
	again := New(dir, nil, "v1", false)
	if got := again.history["local"]; len(got) != 2 || got[1].Load != 2 || got[1].DiskFree != 49 || !got[0].At.Equal(at) {
		t.Fatalf("the file round trip lost samples: %+v", got)
	}
	if len(again.history["gog"]) != 0 {
		t.Fatalf("a failed probe left a sample: %+v", again.history["gog"])
	}
	again.mu.Lock()
	again.reports["local"] = &Report{At: at, Load1: 2}
	again.mu.Unlock()
	rep := again.Report("local")
	if len(rep.History) != 2 || rep.History[0].Load != 1 {
		t.Fatalf("Report carries the history: %+v", rep.History)
	}
	rep.History[0].Load = 99
	if again.Report("local").History[0].Load != 1 {
		t.Fatal("Report hands out the collector's own slice")
	}
	if err := os.WriteFile(filepath.Join(dir, HistoryFile), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if h := loadHistory(dir); len(h) != 0 {
		t.Fatalf("a damaged file is an empty history: %v", h)
	}
	var raw map[string][]map[string]any
	c.remember("local", Report{At: at.Add(30 * time.Minute), Load1: 3, DiskFreeGB: 98, DiskUsedPct: 52})
	b, _ := os.ReadFile(filepath.Join(dir, HistoryFile))
	if err := json.Unmarshal(b, &raw); err != nil || len(raw["local"]) != 3 || raw["local"][2]["disk_free"] != 48.0 {
		t.Fatalf("the file as written: %s (%v)", b, err)
	}
}
