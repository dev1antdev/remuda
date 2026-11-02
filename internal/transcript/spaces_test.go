package transcript

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

// remoteHere is a remote machine whose scripts run in a shell on this one, so a script's quoting meets real paths.
func remoteHere(home string) *shell.Fake {
	return &shell.Fake{Name: "gog", Dir: home, Remote: true, Answer: func(script string) (string, error) {
		out, err := exec.Command("sh", "-c", script).Output()
		return string(out), err
	}}
}

func TestRemoteScriptsKeepAHomeWithASpaceWhole(t *testing.T) {
	home := filepath.Join(t.TempDir(), "My Home")
	project := filepath.Join(home, ".claude", "projects", "-Users-me-my-app")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	// an older Claude Code's continuation: no continued-in record, only the new file's first parent
	old := writeSession(t, project, oldID, now.Add(-2*time.Minute),
		`{"parentUuid":null,"type":"user","uuid":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","timestamp":"2026-10-06T20:00:00Z","message":{"role":"user","content":"build it"}}`)
	next := writeSession(t, project, newID, now.Add(-time.Minute),
		`{"parentUuid":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","type":"user","uuid":"u2","timestamp":"2026-10-06T21:17:49Z","message":{"role":"user","content":"carry on"}}`)
	reader := NewReader(remoteHere(home))
	if got := reader.Live(old); got != next {
		t.Fatalf("Live over the remote script = %q, want %q", got, next)
	}

	const id = "1f2e3d4c-0000-4000-8000-0000000000aa"
	chats := filepath.Join(home, ".gemini", "tmp", GeminiProjectHash("/Users/me/my app"), "chats")
	if err := os.MkdirAll(chats, 0o700); err != nil {
		t.Fatal(err)
	}
	chat := filepath.Join(chats, "session-2026-10-24T10-00-"+id[:8]+".jsonl")
	if err := os.WriteFile(chat, []byte(`{"sessionId":"`+id+`","projectHash":"x","startTime":"2026-10-24T10:00:00Z"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := reader.ResolveSession("gemini", "/Users/me/my app", "id", id)
	if err != nil || strings.TrimSpace(got) != chat {
		t.Fatalf("a Gemini chat over the remote script = %q, %v; want %q", got, err, chat)
	}
}
