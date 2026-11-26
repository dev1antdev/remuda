package transcript

import (
	"path/filepath"
	"testing"
)

func TestOwnsSessionRefusesAnotherProjectsCopy(t *testing.T) {
	home := t.TempDir()
	cwd := "/work/api"
	reader := NewReader(remoteHere(home))

	own := filepath.Join(home, ".gemini", "tmp", GeminiProjectHash(cwd), "chats", "session-abc.json")
	other := filepath.Join(home, ".gemini", "tmp", GeminiProjectHash("/work/web"), "chats", "session-abc.json")
	if !reader.OwnsSession(Gemini, cwd, own) {
		t.Fatal("the pane's own Gemini chat was not its own")
	}
	if reader.OwnsSession(Gemini, cwd, other) {
		t.Fatal("a Gemini chat restored into another project was taken for the pane's")
	}

	claudeOwn := filepath.Join(home, ".claude", "projects", EncodePath(cwd), "abc.jsonl")
	claudeOther := filepath.Join(home, ".claude", "projects", EncodePath("/work/web"), "abc.jsonl")
	if !reader.OwnsSession(Claude, cwd, claudeOwn) {
		t.Fatal("the pane's own Claude session was not its own")
	}
	if reader.OwnsSession(Claude, cwd, claudeOther) {
		t.Fatal("a Claude session restored into another project was taken for the pane's")
	}

	if !reader.OwnsSession(Codex, cwd, filepath.Join(home, ".codex", "sessions", "2026", "11", "x.jsonl")) {
		t.Fatal("Codex keeps no per-project copies: its file belongs to the pane")
	}
}
