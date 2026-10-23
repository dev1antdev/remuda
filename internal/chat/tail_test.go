package chat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/transcript"
)

func TestLastTurnsKeepsTheTail(t *testing.T) {
	turns := make([]transcript.Turn, 5)
	for i := range turns {
		turns[i].Role = []string{"user", "assistant"}[i%2]
	}
	got := lastTurns(turns, 2)
	if len(got) != 2 || got[0].Role != turns[3].Role || got[1].Role != turns[4].Role {
		t.Fatalf("last 2 of 5 = %d turns, the first %q", len(got), got[0].Role)
	}
	if &got[0] == &turns[3] {
		t.Fatal("the tail must be a copy: a shared array would keep the whole conversation in the cache")
	}
	if got := lastTurns(turns, 9); len(got) != 5 {
		t.Fatalf("asking for more than there are = %d turns", len(got))
	}
	if got := lastTurns(turns, 0); len(got) != 5 {
		t.Fatalf("no limit = %d turns", len(got))
	}
}

func TestTailCacheEvictsTheOldest(t *testing.T) {
	var cache tailCache
	for i := 0; i <= tailCacheSize; i++ {
		cache.remember(tailKey{path: fmt.Sprint("f", i), n: 12}, Tail{Turns: []transcript.Turn{{Role: "user"}}})
	}
	if _, ok := cache.lookup(tailKey{path: "f0", n: 12}); ok {
		t.Fatal("the oldest entry should have gone")
	}
	if _, ok := cache.lookup(tailKey{path: fmt.Sprint("f", tailCacheSize), n: 12}); !ok {
		t.Fatal("the newest entry should be there")
	}
	if _, ok := cache.lookup(tailKey{path: "f5", version: transcript.Version{Size: 1}, n: 12}); ok {
		t.Fatal("another version of the file is another entry")
	}
	cache.remember(tailKey{path: "f5", n: 12}, Tail{})
	if len(cache.order) != tailCacheSize {
		t.Fatalf("a known key remembered again should not grow the order: %d", len(cache.order))
	}
}

func TestLastTurnsParsesOncePerVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	lines := []string{
		`{"type":"assistant","uuid":"a0","timestamp":"2026-10-21T09:59:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"t0","name":"TaskCreate","input":{"subject":"Plan the work"}}]}}`,
		`{"type":"user","uuid":"u0","timestamp":"2026-10-21T09:59:01Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t0","content":"Task #1 created successfully: Plan the work"}]}}`,
		`{"type":"user","uuid":"u1","timestamp":"2026-10-21T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"one"}]}}`,
		`{"type":"assistant","uuid":"a1","timestamp":"2026-10-21T10:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"two"}]}}`,
		`{"type":"user","uuid":"u2","timestamp":"2026-10-21T10:00:02Z","message":{"role":"user","content":[{"type":"text","text":"three"}]}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader := transcript.NewReader(shell.Local{})
	version, err := reader.Version(path)
	if err != nil {
		t.Fatal(err)
	}
	var cache tailCache
	conv := Conversation{Path: path, Version: version, reader: reader, tails: &cache}
	got, err := conv.LastTurns(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Turns) != 2 || got.Turns[0].Role != "assistant" || got.Turns[1].Role != "user" {
		t.Fatalf("last 2 turns = %+v", got.Turns)
	}
	if len(got.Plan) != 1 || got.Plan[0].Text != "Plan the work" {
		t.Fatalf("the plan set before the tail should ride with it: %+v", got.Plan)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if again, err := conv.LastTurns(2); err != nil || len(again.Turns) != 2 || len(again.Plan) != 1 {
		t.Fatalf("the same tail should come from the cache once the file is gone: %v, %d turns", err, len(again.Turns))
	}
	if _, err := conv.LastTurns(1); err == nil {
		t.Fatal("another slice is another parse, which the missing file refuses")
	}
}
