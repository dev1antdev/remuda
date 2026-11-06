package transcript

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

func TestAWallOfRemoteAgentsFetchesEachFileWholeOnce(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(home, ".claude", "projects", "-p")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	var whole atomic.Int32
	host := &shell.Fake{Name: "gog", Dir: home, Remote: true, Answer: func(script string) (string, error) {
		if strings.HasPrefix(script, "head -c") {
			whole.Add(1)
		}
		out, err := exec.Command("sh", "-c", script).Output()
		return string(out), err
	}}
	reader := NewReader(host)
	record := `{"type":"assistant","uuid":"u","timestamp":"2026-11-06T10:00:00Z","message":{"role":"assistant","content":[{"type":"text","text":"working"}]}}` + "\n"
	var paths []string
	for i := range 12 { // more agents than the cache once kept files
		path := filepath.Join(project, fmt.Sprintf("%08d-0000-4000-8000-000000000000.jsonl", i))
		if err := os.WriteFile(path, []byte(record), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	for range 3 { // every tile reads its agent's file in turn as each grows
		for _, path := range paths {
			f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			f.WriteString(record)
			f.Close()
			v, err := reader.Version(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reader.Turns(path, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	if got := whole.Load(); got != int32(len(paths)) {
		t.Fatalf("%d whole fetches for %d files: a file pushed out of the cache is fetched whole again at its next read", got, len(paths))
	}
}

func TestATailCacheKeepsItsBudget(t *testing.T) {
	now := time.Now()
	tail := func(size int, readAgo time.Duration) *cachedTail {
		return &cachedTail{data: make([]byte, size), read: now.Add(-readAgo)}
	}
	tails := map[string]*cachedTail{"old": tail(40, time.Hour), "recent": tail(40, time.Second)}
	keepTail(tails, "new", tail(40, 0), 100)
	if _, ok := tails["old"]; ok {
		t.Fatal("the tail read longest ago stays past the budget")
	}
	if tails["recent"] == nil || tails["new"] == nil {
		t.Fatalf("kept %v, want the recent and the new tail", keys(tails))
	}

	keepTail(tails, "another", tail(40, 0), 100)
	if tails["another"] != nil || tails["recent"] == nil || tails["new"] == nil {
		t.Fatalf("kept %v: a file not cached yet pushed out a tail read a moment ago", keys(tails))
	}

	keepTail(tails, "new", tail(80, 0), 100)
	if tails["recent"] != nil || tails["new"] == nil {
		t.Fatalf("kept %v: a cached file that grew past the budget keeps the others", keys(tails))
	}
}

func keys(tails map[string]*cachedTail) []string {
	var out []string
	for k := range tails {
		out = append(out, k)
	}
	return out
}
