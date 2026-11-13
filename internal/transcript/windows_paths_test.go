package transcript

import "testing"

func TestAWindowsSessionPathIsUnderItsHomesRoots(t *testing.T) {
	home := `C:\Users\me`
	for path, want := range map[string]bool{
		`C:\Users\me\.claude\projects\C--Users-me-app\0b4f.jsonl`:               true,
		`c:/users/me/.codex/sessions/2026/11/13/rollout-x.jsonl`:                true,
		`C:\Users\me\.claude\settings.json`:                                     false,
		`C:\Users\me\.claude\projects`:                                          false,
		`C:\Users\me\.claude\projects\..\..\Desktop\x.jsonl`:                    false,
		`D:\Users\me\.claude\projects\C--Users-me-app\0b4f.jsonl`:               false,
		`/home/me/.claude/projects/-home-me-app/0b4f.jsonl`:                     false,
		`C:\Users\me\.gemini\tmp\4f1e\chats\session-2026-11-13T00-00-ab12.json`: true,
	} {
		if got := UnderRoot(home, path); got != want {
			t.Errorf("UnderRoot(%q) = %v, want %v", path, got, want)
		}
	}
	if !UnderRoot("/home/me", "/home/me/.claude/projects/-home-me-app/0b4f.jsonl") || UnderRoot("/home/me", "/home/me/.claude/projects") {
		t.Error("a POSIX home reads as before")
	}
	if got := EncodePath(`C:\Users\me\app`); got != "C--Users-me-app" {
		t.Errorf("Claude Code's folder for a Windows project: %q", got)
	}
}
