package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

const rolloutHead = `{"timestamp":"2026-10-08T09:00:00.000Z","type":"session_meta","payload":{"session_id":"01a1-0000-7000-8000-c0dec0dec0de","id":"01a1-0000-7000-8000-c0dec0dec0de","cwd":"/p/a","originator":"codex-tui"}}` + "\n"

func codexMessage(id, role, text, ts string) string {
	return `{"timestamp":"` + ts + `","type":"response_item","payload":{"type":"message","id":"` + id + `","role":"` + role + `","content":[{"type":"input_text","text":"` + text + `"}]}}` + "\n"
}

func TestScanHomeReadsCodexRolloutsToo(t *testing.T) {
	home := t.TempDir()
	write := func(rel, body string, mod time.Time) {
		p := filepath.Join(home, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(body), 0o644)
		_ = os.Chtimes(p, mod, mod)
	}
	claude := `{"type":"user","uuid":"n1","cwd":"/p/a","timestamp":"2026-10-07T00:00:00Z","message":{"role":"user","content":"needle in October"}}` + "\n"
	write(".claude/projects/-p-a/new.jsonl", claude, time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC))
	rollout := rolloutHead +
		codexMessage("msg_env", "user", "<environment_context>needle in the envelope</environment_context>", "2026-10-08T09:00:01.000Z") +
		codexMessage("msg_u1", "user", "A Needle for Codex", "2026-10-08T09:00:02.000Z") +
		codexMessage("msg_a1", "assistant", "No needle here, said the agent.", "2026-10-08T09:00:05.000Z")
	write(".codex/sessions/2026/10/08/rollout-2026-10-08T09-00-00-01a1-0000-7000-8000-c0dec0dec0de.jsonl", rollout, time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC))

	var b strings.Builder
	if err := ScanHome(home, "needle", "", time.Time{}, 10, &b); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines: %q", lines)
	}
	// the rollout is newer, so its two hits come first, as "codex:<cwd>␟<id>␟<record>"
	for _, line := range lines[:2] {
		if !strings.HasPrefix(line, "codex:/p/a"+script.US+"01a1-0000-7000-8000-c0dec0dec0de"+script.US+"{") {
			t.Fatalf("codex line: %q", line)
		}
	}
	if !strings.HasPrefix(lines[2], "./-p-a/new.jsonl:") {
		t.Fatalf("claude line: %q", lines[2])
	}
	b.Reset()
	_ = ScanHome(home, "needle", "/p/b", time.Time{}, 10, &b)
	if b.Len() != 0 {
		t.Fatalf("another folder: %q", b.String())
	}

	host := &shell.Fake{Name: "local", Dir: home}
	hits, err := one(host, "needle", "", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 || hits[0].Agent != "codex" || hits[0].Session != "01a1-0000-7000-8000-c0dec0dec0de" || hits[0].Dir != "/p/a" || hits[0].Project != "-p-a" {
		t.Fatalf("hits: %+v", hits)
	}
	if hits[0].Role != "assistant" || hits[0].UUID != "msg_a1" || hits[1].UUID != "msg_u1" || !strings.Contains(hits[1].Snippet, "Needle") {
		t.Fatalf("codex hits: %+v %+v", hits[0], hits[1])
	}
	if hits[2].Agent != "" || hits[2].UUID != "n1" {
		t.Fatalf("claude hit: %+v", hits[2])
	}
}
