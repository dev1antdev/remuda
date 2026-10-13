package transcript

import (
	"strconv"
	"strings"
	"testing"
)

func strconvQuote(s string) string { return strconv.Quote(s) }

func TestMatchLine(t *testing.T) {
	user := `{"type":"user","uuid":"u1","cwd":"/p","timestamp":"2026-10-07T10:00:00Z","message":{"role":"user","content":"Давай подключим Hexagon к задачам"}}`
	m, ok := MatchLine([]byte(user), "hexagon")
	if !ok || m.UUID != "u1" || m.Role != "user" || m.Cwd != "/p" || m.Ts.IsZero() || !strings.Contains(m.Snippet, "Hexagon") {
		t.Fatalf("user: %+v %v", m, ok)
	}
	if _, ok := MatchLine([]byte(user), "ШЕСТИГРАННИК"); ok {
		t.Fatal("matched a different word")
	}
	if m, ok := MatchLine([]byte(user), "ДАВАЙ"); !ok || !strings.HasPrefix(m.Snippet, "Давай") {
		t.Fatalf("cyrillic fold: %+v %v", m, ok)
	}
	asst := `{"type":"assistant","uuid":"a1","cwd":"/p","timestamp":"2026-10-07T10:01:00Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"secret hexagon"},{"type":"tool_use","id":"t","name":"Bash","input":{"command":"grep hexagon"}},{"type":"text","text":"Done.\n\nThe Hexagon token stays server-side."}]}}`
	m, ok = MatchLine([]byte(asst), "token stays")
	if !ok || m.Role != "assistant" || m.Snippet != "Done. The Hexagon token stays server-side." {
		t.Fatalf("assistant: %+v %v", m, ok)
	}
	if _, ok := MatchLine([]byte(asst), "secret"); ok {
		t.Fatal("matched thinking")
	}
	if _, ok := MatchLine([]byte(asst), "grep"); ok {
		t.Fatal("matched tool input")
	}
	toolResult := `{"type":"user","uuid":"u2","cwd":"/p","timestamp":"2026-10-07T10:02:00Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t","content":"hexagon hexagon"}]}}`
	if _, ok := MatchLine([]byte(toolResult), "hexagon"); ok {
		t.Fatal("matched a tool result")
	}
	side := `{"type":"user","isSidechain":true,"uuid":"u3","cwd":"/p","message":{"role":"user","content":"hexagon"}}`
	if _, ok := MatchLine([]byte(side), "hexagon"); ok {
		t.Fatal("matched a sidechain")
	}
	long := `{"type":"assistant","uuid":"a2","cwd":"/p","timestamp":"2026-10-07T10:03:00Z","message":{"role":"assistant","content":[{"type":"text","text":"` + strings.Repeat("x ", 200) + `needle` + strings.Repeat(" y", 200) + `"}]}}`
	m, _ = MatchLine([]byte(long), "needle")
	if !strings.HasPrefix(m.Snippet, "…") || !strings.HasSuffix(m.Snippet, "…") || len(m.Snippet) > 300 {
		t.Fatalf("snippet %q", m.Snippet)
	}
}

func TestTurnsCarryRecordIDs(t *testing.T) {
	data := `{"type":"user","uuid":"u1","timestamp":"2026-10-07T10:00:00Z","message":{"role":"user","content":"hi"}}
{"type":"assistant","uuid":"a1","timestamp":"2026-10-07T10:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"one"}]}}
{"type":"assistant","uuid":"a2","timestamp":"2026-10-07T10:00:02Z","message":{"role":"assistant","content":[{"type":"text","text":"two"}]}}
`
	turns, err := ParseBytes([]byte(data))
	if err != nil || len(turns) != 2 {
		t.Fatalf("%v %d", err, len(turns))
	}
	if strings.Join(turns[1].IDs, ",") != "a1,a2" || turns[0].IDs[0] != "u1" {
		t.Fatalf("ids %v %v", turns[0].IDs, turns[1].IDs)
	}
}

func TestSlashCommandReadsAsTyped(t *testing.T) {
	raw := "<command-message>grill-me</command-message>\n<command-name>/grill-me</command-name>\n<command-args>давай подготовим фичер</command-args>"
	if got := userText(raw); got != "/grill-me давай подготовим фичер" {
		t.Fatalf("got %q", got)
	}
	if got := userText("plain"); got != "plain" {
		t.Fatalf("got %q", got)
	}
	rec := `{"type":"user","uuid":"c1","cwd":"/p","timestamp":"2026-10-07T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":` + strconvQuote(raw) + `}]}}`
	m, ok := MatchLine([]byte(rec), "grill")
	if !ok || m.Snippet != "/grill-me давай подготовим фичер" {
		t.Fatalf("%+v %v", m, ok)
	}
	turns, _ := ParseBytes([]byte(rec + "\n"))
	if len(turns) != 1 || turns[0].Parts[0].Text != "/grill-me давай подготовим фичер" {
		t.Fatalf("turns %+v", turns)
	}
}

func TestEncodePathCountsUTF16Units(t *testing.T) {
	cases := map[string]string{
		"/Users/x/app":        "-Users-x-app",
		"/Users/x/Проекты/a":  "-Users-x---------a", // 7 letters, 7 dashes
		"/u/😀/a":              "-u----a",            // an astral rune is two units
		"/Users/x/my.app_dir": "-Users-x-my-app-dir",
	}
	for in, want := range cases {
		if got := EncodePath(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestTurnAnswerAndToolCalls(t *testing.T) {
	turn := Turn{Role: "assistant", IDs: []string{"a", "b"}, Parts: []Part{
		{Type: "text", Text: "thinking out loud"},
		{Type: "tool", ToolName: "Read", Input: []byte(`{"f":1}`)},
		{Type: "text", Text: " done "},
		{Type: "text", Text: "really"},
	}}
	if turn.Answer() != "done\n\nreally" || turn.LastID() != "b" {
		t.Fatalf("answer %q id %q", turn.Answer(), turn.LastID())
	}
	if calls := turn.ToolCalls(); len(calls) != 1 || calls[0] != "Read\x00{\"f\":1}" {
		t.Fatalf("tool calls %q", calls)
	}
	stopped := Turn{Parts: []Part{{Type: "text", Text: "first"}, {Type: "tool", ToolName: "Bash"}}}
	if stopped.Answer() != "" || (Turn{}).LastID() != "" {
		t.Fatal("a turn ending in a tool call is no answer")
	}
}
