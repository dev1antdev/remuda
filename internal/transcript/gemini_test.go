package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// geminiSample is a chat as the CLI's chatRecordingTypes.ts describes one: the metadata line, a prompt, an answer
// with a thought, a tool call whose result lands by a later $patch, tokens, a second exchange undone by $rewindTo.
const geminiSample = `{"sessionId":"6f1c2a3b-9d8e-4f70-a1b2-c3d4e5f60718","projectHash":"abc","startTime":"2026-10-29T10:00:00.000Z","lastUpdated":"2026-10-29T10:00:00.000Z","kind":"main"}
{"id":"m1","timestamp":"2026-10-29T10:00:01.000Z","type":"user","content":[{"text":"List the folder"}]}
{"id":"m2","timestamp":"2026-10-29T10:00:03.000Z","type":"gemini","content":[{"text":"Listing it."},{"functionCall":{"id":"c1","name":"run_shell_command","args":{"command":"ls"}}}],"thoughts":[{"subject":"Plan","description":"One command does it.","timestamp":"2026-10-29T10:00:02.000Z"}],"toolCalls":[{"id":"c1","name":"run_shell_command","args":{"command":"ls"},"status":"executing","timestamp":"2026-10-29T10:00:03.000Z"}],"tokens":{"input":1200,"output":40,"cached":1000,"thoughts":10,"tool":0,"total":1250},"model":"gemini-2.5-pro"}
{"$patch":{"updates":[{"id":"m2","toolCalls":[{"id":"c1","result":[{"functionResponse":{"name":"run_shell_command","response":{"output":"notes.txt\n"}}}],"status":"success"}]}]}}
{"id":"m3","timestamp":"2026-10-29T10:00:05.000Z","type":"gemini","content":"Two files are there.","tokens":{"input":1300,"output":12,"cached":1200,"total":1312},"model":"gemini-2.5-pro"}
{"id":"m4","timestamp":"2026-10-29T10:01:00.000Z","type":"user","content":"Delete everything"}
{"id":"m5","timestamp":"2026-10-29T10:01:02.000Z","type":"gemini","content":"No.","tokens":{"input":1400,"output":2,"cached":1300,"total":1402},"model":"gemini-2.5-pro"}
{"$rewindTo":"m4"}
{"$set":{"lastUpdated":"2026-10-29T10:02:00.000Z","summary":"Listing a folder"}}
{"id":"m6","timestamp":"2026-10-29T10:02:00.000Z","type":"info","content":"Rewound."}
`

func TestParseGeminiReadsMessagesAmendedLater(t *testing.T) {
	turns, err := ParseBytesOf(Gemini, []byte(geminiSample))
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 {
		t.Fatalf("turns: %d, want the first exchange alone, the second rewound", len(turns))
	}
	if turns[0].Role != "user" || turns[0].Parts[0].Text != "List the folder" {
		t.Fatalf("prompt: %+v", turns[0])
	}
	answer := turns[1]
	kinds := make([]string, 0, len(answer.Parts))
	for _, p := range answer.Parts {
		kinds = append(kinds, p.Type+":"+p.ToolName)
	}
	if got := strings.Join(kinds, " "); got != "thinking: text: tool:run_shell_command text:" {
		t.Fatalf("parts: %s", got)
	}
	if answer.Parts[0].Text != "Plan\n\nOne command does it." {
		t.Fatalf("thought: %q", answer.Parts[0].Text)
	}
	call := answer.Parts[2]
	if call.ToolID != "c1" || call.Output != "notes.txt\n" || call.IsError || InputField(call.Input, "command") != "ls" {
		t.Fatalf("the call with the result the patch brought: %+v", call)
	}
	if answer.Answer() != "Two files are there." || len(answer.IDs) != 2 || answer.IDs[1] != "m3" {
		t.Fatalf("answer: %q ids %v", answer.Answer(), answer.IDs)
	}
	if answer.Tokens != 1200+40+10+1300+12 || len(answer.Responses) != 2 || answer.Responses[0].Model != "gemini-2.5-pro" {
		t.Fatalf("spent: %d %+v", answer.Tokens, answer.Responses)
	}
	if r := answer.Responses[0]; r.Input != 200 || r.CacheRead != 1000 || r.Output != 50 {
		t.Fatalf("the first response's tokens: %+v", r)
	}
	legacy := `{"sessionId":"x","projectHash":"abc","startTime":"2026-10-29T10:00:00.000Z","lastUpdated":"2026-10-29T10:00:05.000Z","messages":[` +
		`{"id":"m1","timestamp":"2026-10-29T10:00:01.000Z","type":"user","content":"Hi"},` +
		`{"id":"m2","timestamp":"2026-10-29T10:00:03.000Z","type":"gemini","content":"Hello.","tokens":{"input":10,"output":2,"cached":0,"total":12},"model":"gemini-2.5-flash"}]}`
	turns, err = ParseBytesOf(Gemini, []byte(legacy))
	if err != nil || len(turns) != 2 || turns[1].Answer() != "Hello." || turns[1].Tokens != 12 {
		t.Fatalf("the legacy one-object file: %+v %v", turns, err)
	}
}

func TestGeminiUsageScanner(t *testing.T) {
	scanner := NewUsageScanner("/home/u/.gemini/tmp/abc/chats/session-2026-10-29T10-00-6f1c2a3b.jsonl")
	var recs []UsageRecord
	for _, line := range strings.Split(geminiSample, "\n") {
		if r, ok := scanner.Scan([]byte(line)); ok {
			recs = append(recs, r)
		}
	}
	if len(recs) != 3 || recs[0].MessageID != "m2" || recs[0].Model != "gemini-2.5-pro" || recs[2].MessageID != "m5" {
		t.Fatalf("records: %+v", recs)
	}
}

func TestGeminiFilesAreFoundByHashAndId(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := "/home/u/proj"
	id := "6f1c2a3b-9d8e-4f70-a1b2-c3d4e5f60718"
	chats := filepath.Join(home, ".gemini", "tmp", GeminiProjectHash(cwd), "chats")
	if err := os.MkdirAll(chats, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(chats, "session-2026-10-29T10-00-6f1c2a3b.jsonl")
	if err := os.WriteFile(path, []byte(geminiSample), 0o644); err != nil {
		t.Fatal(err)
	}
	if GeminiProjectHash(cwd) != "d7f6a7c3b2d8f0c8f0a1d2e3c4b5a69788e9f0a1b2c3d4e5f60718293a4b5c6d" && len(GeminiProjectHash(cwd)) != 64 {
		t.Fatalf("hash: %q", GeminiProjectHash(cwd))
	}
	if GeminiSessionID(path) != id || SessionID(path) != id || FormatOf(path) != Gemini {
		t.Fatalf("id from the metadata: %q", GeminiSessionID(path))
	}
	for _, c := range []string{cwd, "/somewhere/else", ""} { // the folder's own chats first, then every project's
		found, err := FindSession(Gemini, c, id)
		if err != nil || found != path {
			t.Fatalf("FindSession(cwd %q): %q %v", c, found, err)
		}
	}
	if found, err := FindSession("", cwd, id); err != nil || found != path {
		t.Fatalf("across formats: %q %v", found, err)
	}
	if _, err := FindSession(Gemini, cwd, "6f1c2a3b-0000-4f70-a1b2-c3d4e5f60718"); err == nil {
		t.Fatal("found a chat whose metadata names another id")
	}
	sessions, err := Sessions(Gemini, cwd)
	if err != nil || len(sessions) != 1 || sessions[0].ID != id || sessions[0].Title != "List the folder" || sessions[0].Agent != "gemini" {
		t.Fatalf("sessions: %+v %v", sessions, err)
	}
	turns, err := Parse(path)
	if err != nil || len(turns) != 2 {
		t.Fatalf("Parse by path: %d %v", len(turns), err)
	}
	if got := findScript(Gemini, "/h", cwd, id); !strings.Contains(got, "'/h'/.gemini/tmp/*/chats/session-*-6f1c2a3b.json*") || !strings.Contains(got, id) {
		t.Fatalf("remote find: %s", got)
	}
}
