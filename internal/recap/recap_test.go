package recap

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/transcript"
)

func tool(name string, input string, took float64, output string, failed bool) transcript.Part {
	return transcript.Part{Type: "tool", ToolName: name, Input: json.RawMessage(input), Took: took, Output: output, IsError: failed}
}

func text(s string) transcript.Part { return transcript.Part{Type: "text", Text: s} }

func sample() []transcript.Turn {
	return []transcript.Turn{
		{Role: "user", Ts: "2026-10-24T08:00:00Z", Parts: []transcript.Part{text("yesterday's prompt")}},
		{Role: "assistant", Ts: "2026-10-24T08:00:05Z", Took: 5, Tokens: 100, Cost: 0.01, Parts: []transcript.Part{tool("Read", `{"file_path":"/old.go"}`, 0.2, "x", false), text("yesterday's answer")}},
		{Role: "user", Ts: "2026-10-24T10:00:00Z", Parts: []transcript.Part{text("fix the tests")}},
		{Role: "assistant", Ts: "2026-10-24T10:00:03Z", Took: 92, Tokens: 38000, Cost: 0.42, Parts: []transcript.Part{
			tool("Bash", `{"command":"go test ./...","description":"Run the tests"}`, 61.5, "FAIL", true),
			tool("Read", `{"file_path":"/a/b.go"}`, 0.3, "package b", false),
			tool("Edit", `{"file_path":"/a/b.go","old_string":"x","new_string":"y"}`, 0.4, "ok", false),
			tool("Read", `{"file_path":"/a/c.go"}`, 0.2, "package c", false),
			tool("Agent", `{"description":"check the docs","prompt":"..."}`, 30, "done", false),
			tool("AskUserQuestion", `{"questions":[]}`, 4, "picked A", false),
			text("## Fixed\n\nThe **tests** pass now."),
		}},
	}
}

func TestOfCountsTheTurnsSince(t *testing.T) {
	since := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	r := Of(sample(), since)
	if r.Prompts != 1 || r.Answers != 1 || r.Took != 92 || r.Tokens != 38000 || r.Cost != 0.42 {
		t.Errorf("numbers = %+v", r)
	}
	if r.Calls != 6 || r.Failed != 1 || r.Tools["Read"] != 2 || r.Tools["Bash"] != 1 {
		t.Errorf("calls = %d failed = %d tools = %v", r.Calls, r.Failed, r.Tools)
	}
	if r.Longest == nil || r.Longest.Name != "Bash" || r.Longest.Took != 61.5 || r.Longest.Hint != "go test ./..." {
		t.Errorf("longest = %+v", r.Longest)
	}
	if len(r.Commands) != 1 || r.Commands[0] != "go test ./..." {
		t.Errorf("commands = %v", r.Commands)
	}
	if strings.Join(r.Edited, ",") != "/a/b.go" || strings.Join(r.Read, ",") != "/a/c.go" {
		t.Errorf("edited = %v read = %v (a file edited is not listed as read)", r.Edited, r.Read)
	}
	if r.Subagents != 1 || r.Questions != 1 || r.Asking {
		t.Errorf("subagents = %d questions = %d asking = %v", r.Subagents, r.Questions, r.Asking)
	}
	if r.Said != "Fixed" || r.First != "2026-10-24T10:00:00Z" || r.Last != "2026-10-24T10:00:03Z" {
		t.Errorf("said = %q first = %s last = %s", r.Said, r.First, r.Last)
	}
	all := Of(sample(), time.Time{})
	if all.Prompts != 2 || all.Answers != 2 || all.Calls != 7 || all.Said != "Fixed" {
		t.Errorf("all = %+v", all)
	}
}

func TestOfSeesAQuestionStillOpen(t *testing.T) {
	turns := []transcript.Turn{
		{Role: "user", Parts: []transcript.Part{text("go")}},
		{Role: "assistant", Parts: []transcript.Part{text("Which one?"), tool("AskUserQuestion", `{}`, 0, "", false)}},
	}
	r := Of(turns, time.Time{})
	if !r.Asking || r.Questions != 1 || r.Answers != 0 || r.Longest != nil {
		t.Errorf("recap = %+v", r)
	}
	if !Of(nil, time.Time{}).Quiet() || r.Quiet() {
		t.Error("quiet is a recap with nothing in it")
	}
}

func TestInputFieldReadsACutInput(t *testing.T) {
	cut, _ := json.Marshal(`{"file_path":"/x/y z.go","content":"a very long`)
	if got := inputField(cut, "file_path"); got != "/x/y z.go" {
		t.Errorf("cut input: %q", got)
	}
	if got := inputField(json.RawMessage(`{"command":"ls","description":"list"}`), "file_path", "command"); got != "ls" {
		t.Errorf("object input: %q", got)
	}
	if got := inputField(nil, "command"); got != "" {
		t.Errorf("no input: %q", got)
	}
}

func TestMarkdownIsAStandupReport(t *testing.T) {
	since := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 10, 24, 18, 30, 0, 0, time.UTC)
	items := []Item{
		{Machine: "gog", Label: "gog", Workspace: "api", Agent: "claude", State: "blocked", Recap: Of(sample(), time.Time{})},
		{Machine: "local", Workspace: "remuda", Agent: "claude", State: "idle", Recap: Recap{}},
	}
	md := Markdown(items, since, now)
	for _, want := range []string{"# Standup · 24 Oct 2026", "## api on gog — needs you", "2 prompts · 2 answers · 1 min 37 s · 38k tokens · $0.43", "7 tool calls (Read 3, Agent 1, AskUserQuestion 1, Bash 1) · 1 failed call · longest Bash 1 min 2 s (go test ./...)", "- Edited: `/a/b.go`", "- Read: `/old.go`, `/a/c.go`", "1 subagent · 1 question", "- Last answer: Fixed", "Nothing to report from remuda."} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md)
		}
	}
	if quiet := Markdown(items[1:], since, now); !strings.Contains(quiet, "Nothing happened in this period.") {
		t.Errorf("quiet report:\n%s", quiet)
	}
	long := Of(sample(), time.Time{})
	long.Commands = []string{strings.Repeat("x", 100), "a", "b", "c", "d", "e", "f"}
	ran := Markdown([]Item{{Workspace: "w", Recap: long}}, since, now)
	if !strings.Contains(ran, "- Ran: `"+strings.Repeat("x", 59)+"…`, `a`, `b`, `c`, `d`, `e`, +1 more") {
		t.Errorf("commands are cut and capped:\n%s", ran)
	}
}

func TestSpentAndFigures(t *testing.T) {
	cases := map[float64]string{12: "12 s", 252: "4 min 12 s", 300: "5 min", 4320: "1 h 12 min", 7200: "2 h"}
	for seconds, want := range cases {
		if got := Spent(seconds); got != want {
			t.Errorf("Spent(%v) = %q, want %q", seconds, got, want)
		}
	}
	if tokens(38400) != "38k" || tokens(1500000) != "1.5M" || tokens(999) != "999" {
		t.Error("tokens")
	}
	if usd(0.423) != "$0.42" || usd(12.34) != "$12.3" || usd(123.4) != "$123" {
		t.Error("usd")
	}
}
