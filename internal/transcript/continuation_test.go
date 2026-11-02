package transcript

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

const (
	oldID = "11111111-1111-1111-1111-111111111111"
	newID = "22222222-2222-2222-2222-222222222222"
)

func writeSession(t *testing.T, dir, id string, mod time.Time, lines ...string) string {
	t.Helper()
	path := filepath.Join(dir, id+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
	return path
}

// continuedSession is a conversation as Claude Code lays it out when it moves to a new file: the old file ends with a
// continued-in record naming the new session, and the new file opens with records too long for the first parentUuid to
// sit in its first 64 KB.
func continuedSession(t *testing.T) (old, next string) {
	t.Helper()
	dir := t.TempDir()
	now := time.Now().Truncate(time.Second)
	old = writeSession(t, dir, oldID, now.Add(-2*time.Minute),
		`{"parentUuid":null,"type":"user","uuid":"u1","timestamp":"2026-10-06T20:00:00Z","message":{"role":"user","content":"build it"}}`,
		`{"parentUuid":"u1","type":"assistant","uuid":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","timestamp":"2026-10-06T20:00:05Z","message":{"role":"assistant","content":[{"type":"text","text":"Done."}]}}`,
		`{"type":"continued-in","timestamp":"2026-10-06T21:17:48Z","sessionId":"`+oldID+`","continuedInSessionId":"`+newID+`"}`,
		`{"type":"cost-state","sessionId":"`+oldID+`","totalCostUSD":1.5}`)
	snapshot := `{"type":"file-history-snapshot","messageId":"x","snapshot":{"blob":"` + strings.Repeat("x", 100*1024) + `"}}`
	next = writeSession(t, dir, newID, now.Add(-time.Minute),
		`{"type":"ai-title","aiTitle":"Build it","sessionId":"`+newID+`"}`,
		snapshot,
		`{"parentUuid":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","isCompactSummary":true,"type":"user","uuid":"u2","timestamp":"2026-10-06T21:17:49Z","message":{"role":"user","content":"This session is being continued from a previous conversation."}}`,
		`{"parentUuid":"u2","type":"assistant","uuid":"a2","timestamp":"2026-10-06T21:18:00Z","message":{"role":"assistant","content":[{"type":"text","text":"Carrying on."}]}}`)
	if firstParent(next) != "" {
		t.Fatal("the fixture's first parentUuid sits in the head: the record must be the only way to the old file")
	}
	return old, next
}

func TestContinuationFollowsTheContinuedInRecord(t *testing.T) {
	old, next := continuedSession(t)
	if got := Continuation(old); got != next {
		t.Fatalf("Continuation(old) = %q, want the file the record names", got)
	}
	if got := Predecessor(next); got != old {
		t.Fatalf("Predecessor(new) = %q, want the file whose record names it", got)
	}
	if got := Continuation(next); got != "" {
		t.Fatalf("Continuation(new) = %q, want none", got)
	}
	if got := Predecessor(old); got != "" {
		t.Fatalf("Predecessor(old) = %q, want none", got)
	}
	reader := NewReader(shell.Local{})
	if got := reader.Live(old); got != next {
		t.Fatalf("Live(old) = %q", got)
	}
	if chain := reader.Lineage(next); len(chain) != 2 || chain[0] != old || chain[1] != next {
		t.Fatalf("Lineage(new) = %v", chain)
	}
}

func TestContinuationScriptsFollowTheContinuedInRecord(t *testing.T) {
	old, next := continuedSession(t)
	if got := runScript(t, ContinuationScript(script.Quote(old))); got != next {
		t.Fatalf("continuation script printed %q, want %q", got, next)
	}
	if got := runScript(t, PredecessorScript(script.Quote(next))); got != old {
		t.Fatalf("predecessor script printed %q, want %q", got, old)
	}
	if got := runScript(t, ContinuationScript(script.Quote(next))); got != "" {
		t.Fatalf("continuation script printed %q for the live file", got)
	}
}

func TestContinuationStillReadsTheParentOfAnOlderFile(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().Truncate(time.Second)
	old := writeSession(t, dir, oldID, now.Add(-2*time.Minute),
		`{"parentUuid":null,"type":"user","uuid":"u1","timestamp":"2026-10-06T20:00:00Z","message":{"role":"user","content":"build it"}}`,
		`{"parentUuid":"u1","type":"assistant","uuid":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","timestamp":"2026-10-06T20:00:05Z","message":{"role":"assistant","content":[{"type":"text","text":"Done."}]}}`)
	next := writeSession(t, dir, newID, now.Add(-time.Minute),
		`{"parentUuid":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","isCompactSummary":true,"type":"user","uuid":"u2","timestamp":"2026-10-06T21:17:49Z","message":{"role":"user","content":"This session is being continued from a previous conversation."}}`)
	if got := Continuation(old); got != next {
		t.Fatalf("Continuation(old) = %q", got)
	}
	if got := Predecessor(next); got != old {
		t.Fatalf("Predecessor(new) = %q", got)
	}
	if got := runScript(t, ContinuationScript(script.Quote(old))); got != next {
		t.Fatalf("continuation script printed %q", got)
	}
	if got := runScript(t, PredecessorScript(script.Quote(next))); got != old {
		t.Fatalf("predecessor script printed %q", got)
	}
}

func runScript(t *testing.T, line string) string {
	t.Helper()
	out, err := exec.Command("sh", "-c", line).Output()
	if err != nil {
		t.Fatalf("sh -c %q: %v", line, err)
	}
	return strings.TrimSpace(string(out))
}
