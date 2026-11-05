package opinion

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseReadsTheAskedForm(t *testing.T) {
	answer := Parse(`FINDINGS
internal/server/opinion.go:42: [high] the lock is released before the store is saved, so two answers can interleave
web/src/features/review/Review.tsx:118: [low] the button stays enabled while the request runs
SUMMARY
A sound change; the race in the store is worth fixing before merging.`, "/Users/me/app")
	want := []Finding{
		{Path: "internal/server/opinion.go", Line: 42, Severity: "high", Text: "the lock is released before the store is saved, so two answers can interleave"},
		{Path: "web/src/features/review/Review.tsx", Line: 118, Severity: "low", Text: "the button stays enabled while the request runs"},
	}
	if !reflect.DeepEqual(answer.Findings, want) {
		t.Fatalf("findings %+v", answer.Findings)
	}
	if answer.Summary != "A sound change; the race in the store is worth fixing before merging." || answer.None || !answer.Readable() {
		t.Fatalf("answer %+v", answer)
	}
}

func TestParseReadsWhatClaudeAndCodexActuallyWrite(t *testing.T) {
	claude := "I reviewed the uncommitted changes and the two commits.\n\n**FINDINGS**\n\n" +
		"- `src/app.go:6`: **medium** — the greeting is now printed twice when `-quiet` is set\n" +
		"- `./src/new.ts:1` (low): the constant is exported but nothing imports it\n" +
		"- **docs/notes.md:4** - [nit] a trailing space\n" +
		"1. Makefile:12: high: the release target no longer writes checksums.txt\n\n" +
		"**Summary:** Mostly fine; the quiet flag regression matters.\n"
	codex := "```text\nFINDINGS\n/Users/me/app/src/app.go:50-52: [critical] answer() returns 42 where the tests expect 41\nsrc/app.go:7 the comment no longer matches the code\n```\n\nSUMMARY\nOne real bug.\n"
	got := Parse(claude, "/Users/me/app")
	want := []Finding{
		{Path: "src/app.go", Line: 6, Severity: "medium", Text: "the greeting is now printed twice when -quiet is set"},
		{Path: "src/new.ts", Line: 1, Severity: "low", Text: "the constant is exported but nothing imports it"},
		{Path: "docs/notes.md", Line: 4, Severity: "low", Text: "a trailing space"},
		{Path: "Makefile", Line: 12, Severity: "high", Text: "the release target no longer writes checksums.txt"},
	}
	if !reflect.DeepEqual(got.Findings, want) {
		t.Fatalf("claude's findings:\n%+v", got.Findings)
	}
	if got.Summary != "Mostly fine; the quiet flag regression matters." {
		t.Fatalf("claude's summary %q", got.Summary)
	}
	got = Parse(codex, "/Users/me/app/")
	want = []Finding{
		{Path: "src/app.go", Line: 50, Severity: "high", Text: "answer() returns 42 where the tests expect 41"},
		{Path: "src/app.go", Line: 7, Text: "the comment no longer matches the code"},
	}
	if !reflect.DeepEqual(got.Findings, want) || got.Summary != "One real bug." {
		t.Fatalf("codex's answer %+v", got)
	}
}

func TestParseTellsNothingFoundFromNonsense(t *testing.T) {
	for _, text := range []string{"FINDINGS\nNONE\nSUMMARY\nClean.", "## Findings\n\nNo findings.\n\n## Summary\nLooks good.", "FINDINGS:\n- none\n"} {
		if answer := Parse(text, ""); !answer.None || len(answer.Findings) != 0 || !answer.Readable() {
			t.Errorf("%q read as %+v", text, answer)
		}
	}
	for _, text := range []string{
		"I could not run git here: permission denied.",
		"The job ran at 12:30 and again at 14:05; see https://ci.example.com:8443/run/9 for the log.",
		"",
	} {
		if answer := Parse(text, ""); answer.Readable() {
			t.Errorf("%q read as findings %+v", text, answer.Findings)
		}
	}
	if answer := Parse("Something went wrong.\nI stopped.", ""); answer.Summary != "I stopped." {
		t.Errorf("an answer without sections keeps its last line as the summary: %q", answer.Summary)
	}
}

func TestParseKeepsAFindingOnceAndCutsTheLongOnes(t *testing.T) {
	line := "a.go:3: [low] same\n"
	long := "b.go:9: " + strings.Repeat("x", MaxText+50)
	answer := Parse("FINDINGS\n"+line+line+long, "")
	if len(answer.Findings) != 2 || len([]rune(answer.Findings[1].Text)) != MaxText || !strings.HasSuffix(answer.Findings[1].Text, "…") {
		t.Fatalf("findings %+v", answer.Findings)
	}
	var many strings.Builder
	for i := 1; i <= MaxFindings+20; i++ {
		many.WriteString("c.go:" + strings.Repeat("1", 1) + ": finding " + strings.Repeat("y", i%7) + string(rune('a'+i%26)) + "\n")
	}
	if got := Parse("FINDINGS\n"+many.String(), ""); len(got.Findings) > MaxFindings {
		t.Fatalf("%d findings kept", len(got.Findings))
	}
}

func TestPromptAsksForTheFormParseReads(t *testing.T) {
	prompt := Prompt(Scope{Branch: "task-41", Base: "abc1234", Commits: 2, Files: []string{"src/app.go (+2 −2)", "src/new.ts (new)"}}, "9f2c")
	for _, want := range []string{"branch task-41", "git diff HEAD", "git log -p abc1234..HEAD", "2 commits", "src/new.ts (new)", "Do not edit", "FINDINGS", "path/to/file:LINE: [high|medium|low]", "SUMMARY", "NONE", "(second opinion request 9f2c)"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt lacks %q", want)
		}
	}
	if !IsRequest(prompt, "9f2c") || IsRequest(prompt, "77aa") || IsRequest(prompt, "") || IsRequest("Fix the login page (second opinion request 9f2c)", "9f2c") {
		t.Error("a request is not told by its own ref")
	}
	if strings.Contains(Prompt(Scope{}, ""), "git log") {
		t.Error("a folder never reviewed is asked about its commits")
	}
	files := make([]string, maxListed+5)
	for i := range files {
		files[i] = "f.go"
	}
	if !strings.Contains(Prompt(Scope{Files: files}, ""), "… and 5 more") {
		t.Error("a long file list is not cut")
	}
}
