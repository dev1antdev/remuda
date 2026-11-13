package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/transcript"
	"github.com/dev1antdev/remuda/internal/wintest"
)

// TestOnWindowsAClaudeCodeSessionIsFoundReadAndSearchedThere lays a Claude Code session out under the runner's home as
// Claude Code on Windows does (~/.claude/projects/C--Users-me-app) and reads it as a cockpit reads a remote one: the
// session resolved by id, its version and turns by stat and tail over ssh, and the search through remuda.exe there.
func TestOnWindowsAClaudeCodeSessionIsFoundReadAndSearchedThere(t *testing.T) {
	wintest.Live(t)
	host := wintest.Host(t)
	cwd := filepath.Join(host.Home(), "remuda-windows-check")
	session := "0b4f7c1e-0000-4000-8000-00000000c0de"
	folder := filepath.Join(host.Home(), ".claude", "projects", transcript.EncodePath(cwd))
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(folder)
	jsonCWD := strings.ReplaceAll(cwd, `\`, `\\`)
	lines := `{"type":"user","uuid":"u1","sessionId":"` + session + `","cwd":"` + jsonCWD + `","timestamp":"2026-11-13T10:00:00Z","message":{"role":"user","content":"find the windowsneedle please"}}` + "\n" +
		`{"type":"assistant","uuid":"a1","parentUuid":"u1","sessionId":"` + session + `","cwd":"` + jsonCWD + `","timestamp":"2026-11-13T10:00:05Z","message":{"id":"m1","role":"assistant","model":"claude-opus-5-5","content":[{"type":"text","text":"Found it."}],"usage":{"input_tokens":10,"output_tokens":3}}}` + "\n"
	if err := os.WriteFile(filepath.Join(folder, session+".jsonl"), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}

	reader := transcript.NewReader(host)
	path, err := reader.ResolveSession("claude", cwd, "id", session)
	if err != nil {
		t.Fatalf("the session by id: %v", err)
	}
	version, err := reader.Version(path)
	if err != nil || version.Size != int64(len(lines)) {
		t.Fatalf("its version %+v: %v (path %s)", version, err, path)
	}
	turns, err := reader.Turns(path, version)
	if err != nil || len(turns) != 2 {
		t.Fatalf("its turns: %d, %v", len(turns), err)
	}

	result := Search([]shell.Host{host}, "windowsneedle", "", time.Time{})
	if len(result.Hits) != 1 || result.Hits[0].Session != session || result.Hits[0].Project != transcript.EncodePath(cwd) {
		t.Fatalf("the search through remuda.exe there: %+v", result)
	}
}
