package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScanNewestFirstWithFilters(t *testing.T) {
	projects := t.TempDir()
	write := func(rel, body string, mod time.Time) {
		p := filepath.Join(projects, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(body), 0o644)
		_ = os.Chtimes(p, mod, mod)
	}
	rec := func(uuid, cwd, ts, text string) string {
		return `{"type":"user","uuid":"` + uuid + `","cwd":"` + cwd + `","timestamp":"` + ts + `","message":{"role":"user","content":"` + text + `"}}` + "\n"
	}
	old := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	fresh := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	write("-p-a/old.jsonl", rec("o1", "/p/a", "2026-09-01T00:00:00Z", "Needle in September"), old)
	write("-p-a/new.jsonl", rec("n1", "/p/a", "2026-10-07T00:00:00Z", "needle in October")+rec("n2", "/p/a", "2026-10-07T00:01:00Z", "no match")+`{"type":"user","uuid":"n3","cwd":"/p/a","message":{"role":"user","content":[{"type":"tool_result","content":"needle"}]}}`+"\n", fresh)
	write("-p-b/b.jsonl", rec("b1", "/p/b/sub", "2026-10-06T00:00:00Z", "NEEDLE elsewhere, after a cd"), fresh.Add(-time.Hour))
	write("-p-b/b/subagents/agent-1.jsonl", rec("s1", "/p/b", "2026-10-06T00:00:00Z", "needle in a subagent"), fresh)
	write("-p-c/c.jsonl", rec("c1", "/p/c", "2026-10-07T01:00:00Z", "needle in a record far too long "+strings.Repeat("x", lineCap)), fresh)

	var b strings.Builder
	if err := Scan(projects, "needle", "", time.Time{}, 10, &b); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "./-p-a/new.jsonl:") || !strings.HasPrefix(lines[1], "./-p-b/b.jsonl:") || !strings.HasPrefix(lines[2], "./-p-a/old.jsonl:") {
		t.Fatalf("got %q", lines)
	}
	b.Reset()
	_ = Scan(projects, "needle", "/p/b", time.Time{}, 10, &b)
	if n := strings.Count(b.String(), "\n"); n != 1 || !strings.Contains(b.String(), "b1") {
		t.Fatalf("dir filter: %q", b.String())
	}
	if projectOf("./-p-b/b.jsonl") != "-p-b" || projectOf("x") != "" {
		t.Fatal("projectOf")
	}
	b.Reset()
	_ = Scan(projects, "needle", "", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), 10, &b)
	if n := strings.Count(b.String(), "\n"); n != 2 {
		t.Fatalf("since filter: %q", b.String())
	}
	b.Reset()
	_ = Scan(projects, "needle", "", time.Time{}, 1, &b)
	if n := strings.Count(b.String(), "\n"); n != 1 || !strings.Contains(b.String(), "n1") {
		t.Fatalf("limit: %q", b.String())
	}
}

func TestScriptQuotesAndSpellings(t *testing.T) {
	s := script("/Users/me", `say "hi"`, "", time.Time{})
	if !strings.Contains(s, `-e 'say \"hi\"'`) {
		t.Fatalf("the JSON form is missing: %s", s)
	}
	if !strings.Contains(s, `-e 'SAY \"HI\"'`) || !strings.Contains(s, `-e 'Say \"hi\"'`) {
		t.Fatalf("spellings: %s", s)
	}
	want := `if [ -x '/Users/me/.local/bin/remuda' ] && [ "$('/Users/me/.local/bin/remuda' -version 2>/dev/null)" = 'remuda dev' ] && '/Users/me/.local/bin/remuda' search -q 'say "hi"' -limit 400; then exit 0; fi; cd '/Users/me/.claude/projects'`
	if !strings.HasPrefix(s, want) {
		t.Fatalf("prefix:\n%s\nwant\n%s", s, want)
	}
	if n := strings.Count(script("/h", "123", "", time.Time{}), "-e "); n != 1 {
		t.Fatalf("%d patterns for digits", n)
	}
	if !strings.Contains(script("/h", "<b>&", "", time.Time{}), `-e '<b>&'`) {
		t.Fatalf("html escaped: %s", script("/h", "<b>&", "", time.Time{}))
	}
	if !strings.Contains(script("/h", "it's", "", time.Time{}), `'it'\''s'`) {
		t.Fatalf("shell quote: %s", script("/h", "it's", "", time.Time{}))
	}
	if s := script("/h", "x", "/p/q", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)); !strings.Contains(s, " -dir '/p/q' -since '2026-10-01T00:00:00Z'") {
		t.Fatalf("filters: %s", s)
	}
	if capitalized("вику") != "Вику" {
		t.Fatalf("capitalized %q", capitalized("вику"))
	}
}
