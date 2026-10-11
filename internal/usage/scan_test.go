package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/machines"
)

func rec(id, model, ts string, out int64) string {
	return `{"type":"assistant","timestamp":"` + ts + `","cwd":"/p/a","message":{"id":"` + id + `","model":"` + model + `","usage":{"input_tokens":100,"output_tokens":` + itoa(out) + `,"cache_read_input_tokens":1000,"cache_creation_input_tokens":0}}}` + "\n"
}

func itoa(n int64) string {
	b := []byte{}
	if n == 0 {
		return "0"
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestScanAttributesAndDedupes(t *testing.T) {
	projects := t.TempDir()
	kyiv, _ := time.LoadLocation("Europe/Kyiv")
	write := func(rel, body string, mod time.Time) {
		p := filepath.Join(projects, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(body), 0o644)
		_ = os.Chtimes(p, mod, mod)
	}
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	// a session: one response written twice (two blocks), another response; 22:30 UTC is the 8th in Kyiv
	write("-p-a/s1.jsonl", rec("m1", "claude-fable-5-1", "2026-10-06T10:00:00Z", 10)+rec("m1", "claude-fable-5-1", "2026-10-06T10:00:00Z", 50)+rec("m2", "claude-fable-5-1", "2026-10-06T22:30:00Z", 100)+`{"type":"user","message":{"role":"user","content":"x"}}`+"\n", now)
	// its subagent
	write("-p-a/s1/subagents/agent-1.jsonl", rec("m3", "claude-haiku-4-5", "2026-10-06T11:00:00Z", 1000), now)
	// an old file, untouched since before the period
	write("-p-a/old.jsonl", rec("m4", "claude-fable-5-1", "2026-09-01T10:00:00Z", 1000), now.AddDate(0, 0, -30))
	// a fresh file with an old record inside
	write("-p-b/s2.jsonl", rec("m5", "claude-opus-5-5", "2026-09-30T10:00:00Z", 1000)+rec("m6", "claude-opus-5-5", "2026-10-07T09:00:00Z", 20), now)

	rep, err := Scan(projects, now.AddDate(0, 0, -7), kyiv)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Files != 3 || rep.Total.Turns != 4 {
		t.Fatalf("files %d turns %d", rep.Files, rep.Total.Turns)
	}
	if rep.Total.Output != 50+100+1000+20 {
		t.Fatalf("output %d (m1's later block must win)", rep.Total.Output)
	}
	if len(rep.Days) != 2 || rep.Days[0].Date != "2026-10-06" || rep.Days[1].Date != "2026-10-07" {
		t.Fatalf("days %+v", rep.Days)
	}
	// 22:30 UTC on the 6th is 01:30 on the 7th in Kyiv; 09:00 UTC on the 7th is the 7th
	byDate := map[string]int{}
	for _, d := range rep.Days {
		byDate[d.Date] += d.Turns
	}
	if byDate["2026-10-06"] != 2 || byDate["2026-10-07"] != 2 {
		t.Fatalf("by date %v", byDate)
	}
	if len(rep.Projects) != 2 || rep.Projects[0].Project == "" {
		t.Fatalf("projects %+v", rep.Projects)
	}
	var s1 *SessionTotals
	for i := range rep.Sessions {
		if rep.Sessions[i].Session == "s1" {
			s1 = &rep.Sessions[i]
		}
	}
	if s1 == nil || s1.Turns != 3 || s1.Model != "claude-fable-5-1" || s1.Dir != "/p/a" {
		t.Fatalf("s1 %+v", s1)
	}
	if len(rep.Models) != 3 || rep.Models[0].Name == "" {
		t.Fatalf("models %+v", rep.Models)
	}
}

func TestTrackerReadsOnlyTheGrowth(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	_ = os.WriteFile(path, []byte(rec("m1", "claude-fable-5-1", "2026-10-07T10:00:00Z", 10)), 0o644)
	mc := &machines.Machine{Cfg: machines.Config{ID: "local"}, Local: true}
	tr := NewTracker()
	tr.Observe(mc, "p1", []string{path})
	d, ok := tr.Display("local", "p1")
	if !ok || d.Turns != 1 || d.Name != "Fable 5.1" || d.Window != 1000000 || d.Context != 1100 {
		t.Fatalf("first %+v %v", d, ok)
	}
	// more responses; the tracker is rate limited, so force the look
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString(rec("m2", "claude-fable-5-1", "2026-10-07T10:05:00Z", 20) + `{"type":"assistant","partial`)
	f.Close()
	tr.mu.Lock()
	tr.files[key("local", path)].checked = time.Time{}
	tr.mu.Unlock()
	tr.Observe(mc, "p1", []string{path})
	d, _ = tr.Display("local", "p1")
	if d.Turns != 2 || d.Tokens.Output != 30 {
		t.Fatalf("after growth %+v", d)
	}
	// the partial line completes
	f, _ = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString(`"}` + "\n" + rec("m3", "claude-fable-5-1", "2026-10-07T10:06:00Z", 5))
	f.Close()
	tr.mu.Lock()
	tr.files[key("local", path)].checked = time.Time{}
	tr.mu.Unlock()
	tr.Observe(mc, "p1", []string{path})
	d, _ = tr.Display("local", "p1")
	if d.Turns != 3 || d.Tokens.Output != 35 {
		t.Fatalf("after the partial line %+v", d)
	}
	// a continuation: the old file's cost rides along
	next := filepath.Join(dir, "s2.jsonl")
	_ = os.WriteFile(next, []byte(rec("m9", "claude-fable-5-1", "2026-10-07T11:00:00Z", 1)), 0o644)
	tr.Observe(mc, "p1", []string{path, next})
	d, _ = tr.Display("local", "p1")
	if d.Turns != 4 || d.Tokens.Output != 36 || d.Context != 1100 {
		t.Fatalf("chained %+v", d)
	}
	// a subagent's transcript under the live session counts too
	subdir := filepath.Join(dir, "s2", "subagents")
	_ = os.MkdirAll(subdir, 0o755)
	_ = os.WriteFile(filepath.Join(subdir, "agent-1.jsonl"), []byte(rec("sa1", "claude-haiku-4-5", "2026-10-07T11:30:00Z", 7)), 0o644)
	tr.mu.Lock()
	delete(tr.subsChecked, key("local", next))
	tr.mu.Unlock()
	tr.Observe(mc, "p1", []string{path, next})
	d, _ = tr.Display("local", "p1")
	if d.Turns != 5 || d.Tokens.Output != 43 {
		t.Fatalf("with a subagent %+v", d)
	}
	// a second compaction: the middle file still counts
	third := filepath.Join(dir, "s3.jsonl")
	_ = os.WriteFile(third, []byte(rec("m10", "claude-fable-5-1", "2026-10-07T12:00:00Z", 2)), 0o644)
	tr.Observe(mc, "p1", []string{path, next, third})
	d, _ = tr.Display("local", "p1")
	if d.Turns != 5 || d.Tokens.Output != 38 { // s3's subagents (none) replace s2's in the sum
		t.Fatalf("twice chained %+v", d)
	}
}
