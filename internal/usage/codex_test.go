package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/transcript"
)

func rollout(cwd, model string, responses ...string) string {
	out := `{"timestamp":"2026-10-08T09:00:00.000Z","type":"session_meta","payload":{"session_id":"x","cwd":"` + cwd + `"}}` + "\n" +
		`{"timestamp":"2026-10-08T09:00:01.000Z","type":"turn_context","payload":{"turn_id":"t1","cwd":"` + cwd + `","model":"` + model + `"}}` + "\n" +
		`{"timestamp":"2026-10-08T09:00:01.000Z","type":"event_msg","payload":{"type":"task_started","turn_id":"t1","model_context_window":475000}}` + "\n"
	for _, r := range responses {
		out += r
	}
	return out
}

func codexResponse(id, ts string, input, cached, output int64) string {
	return `{"timestamp":"` + ts + `","type":"token_usage_record","payload":{"thread_id":"x","turn_id":"t1","response_id":"` + id + `","usage":{"input_tokens":` + itoa(input) + `,"cached_input_tokens":` + itoa(cached) + `,"cache_write_input_tokens":0,"output_tokens":` + itoa(output) + `,"reasoning_output_tokens":0,"total_tokens":0}}}` + "\n"
}

func TestScanHomeCountsCodexBesideClaude(t *testing.T) {
	home := t.TempDir()
	write := func(rel, body string, mod time.Time) {
		p := filepath.Join(home, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(body), 0o644)
		_ = os.Chtimes(p, mod, mod)
	}
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	write(".claude/projects/-p-a/s1.jsonl", rec("m1", "claude-fable-5-1", "2026-10-06T10:00:00Z", 10), now)
	write(".codex/sessions/2026/10/08/rollout-2026-10-08T09-00-00-01a1-0000-7000-8000-c0dec0dec0de.jsonl",
		rollout("/p/a", "gpt-6.1-sol", codexResponse("resp_1", "2026-10-08T09:00:05.000Z", 16443, 12288, 82), codexResponse("resp_2", "2026-10-08T09:00:09.000Z", 16708, 16384, 35)), now)
	write(".codex/sessions/2026/09/01/rollout-2026-09-01T09-00-00-01a1-0000-7000-8000-000000000001.jsonl",
		rollout("/p/a", "gpt-6.1-sol", codexResponse("resp_old", "2026-09-01T09:00:05.000Z", 100, 0, 10)), now.AddDate(0, 0, -40))

	rep, err := ScanHome(home, now.AddDate(0, 0, -7), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Files != 2 || rep.Total.Turns != 3 || rep.Total.Unknown != 2 {
		t.Fatalf("files %d turns %d unknown %d", rep.Files, rep.Total.Turns, rep.Total.Unknown)
	}
	if rep.Total.Output != 10+82+35 || rep.Total.CacheRead != 1000+12288+16384 || rep.Total.Input != 100+(16443-12288)+(16708-16384) {
		t.Fatalf("tokens %+v", rep.Total.Tokens)
	}
	if len(rep.Projects) != 1 || rep.Projects[0].Project != "-p-a" || rep.Projects[0].Dir != "/p/a" || rep.Projects[0].Sessions != 2 {
		t.Fatalf("one folder, both agents: %+v", rep.Projects)
	}
	var codex *SessionTotals
	for i := range rep.Sessions {
		if rep.Sessions[i].Session == "01a1-0000-7000-8000-c0dec0dec0de" {
			codex = &rep.Sessions[i]
		}
	}
	if codex == nil || codex.Turns != 2 || codex.Model != "gpt-6.1-sol" || codex.Dir != "/p/a" || codex.First != "2026-10-08T09:00:05Z" {
		t.Fatalf("the rollout's session: %+v", codex)
	}
	models := map[string]int{}
	for _, m := range rep.Models {
		models[m.Model] = m.Turns
	}
	if models["gpt-6.1-sol"] != 2 || models["claude-fable-5-1"] != 1 {
		t.Fatalf("models %+v", rep.Models)
	}
}

func TestTrackerMetersACodexPane(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".codex", "sessions", "2026", "10", "08")
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "rollout-2026-10-08T09-00-00-01a1-0000-7000-8000-c0dec0dec0de.jsonl")
	_ = os.WriteFile(path, []byte(rollout("/p/a", "gpt-6.1-sol", codexResponse("resp_1", "2026-10-08T09:00:05.000Z", 16443, 12288, 82))), 0o644)
	files := transcript.NewReader(shell.Local{Name: "local"})
	tr := NewTracker()
	tr.Observe("local", files, "p1", []string{path})
	d, ok := tr.Display("local", "p1")
	if !ok || d.Turns != 1 || !d.Unknown || d.Name != "gpt-6.1-sol" || d.Window != 475000 || d.Context != 16443 || d.Cost != 0 {
		t.Fatalf("first %+v %v", d, ok)
	}
	// the growth alone is read next: the model named before the cut is still known
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString(codexResponse("resp_2", "2026-10-08T09:00:09.000Z", 16708, 16384, 35))
	f.Close()
	tr.mu.Lock()
	tr.files[key("local", path)].checked = time.Time{}
	tr.mu.Unlock()
	tr.Observe("local", files, "p1", []string{path})
	d, _ = tr.Display("local", "p1")
	if d.Turns != 2 || d.Tokens.Output != 82+35 || d.Context != 16708 || d.Name != "gpt-6.1-sol" {
		t.Fatalf("after growth %+v", d)
	}
}
