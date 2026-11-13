package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

func recIn(id, model, ts, cwd string, out int64) string {
	return strings.Replace(rec(id, model, ts, out), `"cwd":"/p/a"`, `"cwd":"`+cwd+`"`, 1)
}

// writeHome lays transcripts out under a home's Claude Code projects, each written at mod.
func writeHome(t *testing.T, home string, files map[string]string, mod time.Time) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(home, ".claude", "projects", rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(p, mod, mod)
	}
}

func TestEveryDayIsBrokenDownByFolderAndSession(t *testing.T) {
	home := t.TempDir()
	kyiv, _ := time.LoadLocation("Europe/Kyiv")
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	writeHome(t, home, map[string]string{
		// 22:30 UTC on the 11th is half past midnight on the 12th in Kyiv
		"-p-a/s1.jsonl":                   recIn("m1", "claude-fable-5-1", "2026-11-11T10:00:00Z", "/p/a", 10) + recIn("m2", "claude-fable-5-1", "2026-11-11T22:30:00Z", "/p/a", 20),
		"-p-a/s1/subagents/agent-1.jsonl": recIn("m3", "claude-haiku-4-5", "2026-11-11T11:00:00Z", "/p/a", 30),
		"-p-b/s2.jsonl":                   recIn("m4", "claude-opus-5-5", "2026-11-12T09:00:00Z", "/p/b", 40),
	}, now)
	rep, err := ScanHomeDays(home, time.Date(2026, 11, 1, 0, 0, 0, 0, kyiv), kyiv)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Daily) != 2 || rep.Daily[0].Date != "2026-11-11" || rep.Daily[1].Date != "2026-11-12" {
		t.Fatalf("days %+v", rep.Daily)
	}
	first := rep.Daily[0].Folders
	if len(first) != 1 || first[0].Project != "-p-a" || first[0].Dir != "/p/a" || first[0].Turns != 2 {
		t.Fatalf("the 11th: %+v", first)
	}
	if span := first[0].Sessions["s1"]; span.Last-span.First != 3600 || len(first[0].Sessions) != 1 {
		t.Fatalf("a subagent's responses are its session's: %+v", first[0].Sessions)
	}
	if first[0].Models["claude-fable-5-1"] != 1110 || first[0].Models["claude-haiku-4-5"] != 1130 || first[0].Tokens != 1110+1130 {
		t.Fatalf("models %+v tokens %d", first[0].Models, first[0].Tokens)
	}
	second := rep.Daily[1].Folders
	if len(second) != 2 || second[0].Project != "-p-a" || second[0].Turns != 1 || second[1].Project != "-p-b" || second[1].Cost <= 0 {
		t.Fatalf("the 12th: %+v", second)
	}
	plain, _ := ScanHome(home, time.Date(2026, 11, 1, 0, 0, 0, 0, kyiv), kyiv)
	if plain.Daily != nil {
		t.Fatal("the usage report carries no days unless asked")
	}
}

func TestARemoteMachinesDaysComeThroughItsOwnRemuda(t *testing.T) {
	answer, _ := json.Marshal(Report{Daily: []Day{{Date: "2026-11-12", Folders: []DayFolder{{Project: "-p", Turns: 3}}}}})
	host := &shell.Fake{Name: "gog", Dir: "/home/me", Remote: true, Answer: func(string) (string, error) { return string(answer), nil }}
	rep, err := ReadHost(host, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.UTC, "UTC", true, time.Minute)
	if err != nil || len(rep.Daily) != 1 || rep.Daily[0].Folders[0].Turns != 3 {
		t.Fatalf("report %+v, %v", rep, err)
	}
	if script := host.Scripts()[0]; !strings.Contains(script, " usage -since ") || !strings.Contains(script, " -daily") || !strings.Contains(script, "-tz 'UTC'") {
		t.Fatalf("script %s", script)
	}
	if _, err := ReadHost(&shell.Fake{Name: "pi", Remote: true, Answer: func(string) (string, error) { return "nope", nil }}, time.Now(), time.UTC, "", true, time.Minute); err != errNoReport {
		t.Fatalf("an answer that is no report: %v", err)
	}
}
