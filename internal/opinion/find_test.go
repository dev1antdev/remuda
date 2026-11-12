package opinion

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/transcript"
)

// homeAt is this machine with a home of the test's own, where the session files are written
type homeAt struct {
	machine shell.Local
	home    string
}

func (h homeAt) ID() string    { return h.machine.ID() }
func (h homeAt) Label() string { return h.machine.Label() }
func (h homeAt) Local() bool   { return true }
func (h homeAt) Home() string  { return h.home }
func (h homeAt) Run(script string, timeout time.Duration) ([]byte, error) {
	return h.machine.Run(script, timeout)
}
func (h homeAt) Stream(script string, timeout time.Duration, sink func([]byte)) error {
	return h.machine.Stream(script, timeout, sink)
}

const (
	requestID  = "11111111-1111-4111-8111-111111111111"
	movedID    = "22222222-2222-4222-8222-222222222222"
	personalID = "33333333-3333-4333-8333-333333333333"
)

func record(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func userLine(uuid, parent, text string) string {
	parentField := `null`
	if parent != "" {
		parentField = `"` + parent + `"`
	}
	quoted := strings.ReplaceAll(strings.ReplaceAll(text, `\`, `\\`), "\n", `\n`)
	return `{"parentUuid":` + parentField + `,"type":"user","uuid":"` + uuid + `","timestamp":"2026-11-05T10:00:00Z","message":{"role":"user","content":"` + strings.ReplaceAll(quoted, `"`, `\"`) + `"}}`
}

func answerLine(uuid, parent, text string) string {
	quoted := strings.ReplaceAll(text, "\n", `\n`)
	return `{"parentUuid":"` + parent + `","type":"assistant","uuid":"` + uuid + `","timestamp":"2026-11-05T10:01:00Z","message":{"role":"assistant","content":[{"type":"text","text":"` + quoted + `"}]}}`
}

// TestTheRequestIsFoundWithoutTheSessionAndFollowedThroughACompaction is a reviewer whose pane names no session: its
// conversation is found among the folder's by the request's ref, beside the person's own agent's conversation and
// an earlier request's, and read where a compaction moved it.
func TestTheRequestIsFoundWithoutTheSessionAndFollowedThroughACompaction(t *testing.T) {
	host := homeAt{machine: shell.Local{Name: "local"}, home: t.TempDir()}
	dir := "/work/app"
	project := filepath.Join(transcript.SessionRoots(host.home)[transcript.Claude], transcript.EncodePath(dir))
	op := Opinion{Agent: "claude", Dir: dir, Ref: "ref-now"}
	record(t, filepath.Join(project, personalID+".jsonl"),
		userLine("p1", "", "Fix the login page"),
		answerLine("p2", "p1", "FINDINGS\nlogin.go:3: [high] not the review"))
	record(t, filepath.Join(project, "44444444-4444-4444-8444-444444444444.jsonl"),
		userLine("e1", "", Prompt(Scope{}, "ref-before")),
		answerLine("e2", "e1", "FINDINGS\nold.go:1: an earlier request's answer"))
	record(t, filepath.Join(project, requestID+".jsonl"),
		userLine("r1", "", Prompt(Scope{Branch: "main"}, op.Ref)),
		answerLine("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1", "r1", "Reading the diff."),
		`{"type":"continued-in","timestamp":"2026-11-05T10:02:00Z","sessionId":"`+requestID+`","continuedInSessionId":"`+movedID+`"}`)
	record(t, filepath.Join(project, movedID+".jsonl"),
		`{"parentUuid":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","isCompactSummary":true,"type":"user","uuid":"m1","timestamp":"2026-11-05T10:02:01Z","message":{"role":"user","content":"This session is being continued from a previous conversation."}}`,
		answerLine("m2", "m1", "FINDINGS\napp.go:12: [medium] the moved conversation's answer\nSUMMARY\nok"))
	reply, ok := Request(host, transcript.NewReader(host), op)
	if !ok || !strings.Contains(reply.Answer, "the moved conversation's answer") || reply.Stopped {
		t.Fatalf("the request's answer: %+v %v", reply, ok)
	}
	if _, ok := Request(host, transcript.NewReader(host), Opinion{Agent: "claude", Dir: dir, Ref: "ref-never-asked"}); ok {
		t.Fatal("a request no conversation opens with was answered")
	}
	if _, ok := Request(host, transcript.NewReader(host), Opinion{Agent: "codex", Dir: dir, Ref: op.Ref}); ok {
		t.Fatal("a Codex reviewer's request was looked for among Claude Code's files")
	}
}

func TestTheSearchCoversTheAgentsOwnFilesOnly(t *testing.T) {
	codex := FindScript("/h", "codex", "/w", "r")
	if !strings.Contains(codex, "'/h/.codex/sessions'") || strings.Contains(codex, ".claude") || !strings.Contains(codex, "-mtime -2") {
		t.Fatalf("Codex's search: %s", codex)
	}
	claude := FindScript("/h", "claude", "/w/app", "r")
	if !strings.Contains(claude, "/h/.claude/projects/"+transcript.EncodePath("/w/app")) || strings.Contains(claude, ".codex") {
		t.Fatalf("Claude Code's search: %s", claude)
	}
	other := FindScript("/h", "kimi", "/w", "r")
	if !strings.Contains(other, ".claude") || !strings.Contains(other, ".codex") || !strings.Contains(other, ".gemini") {
		t.Fatalf("another agent's search: %s", other)
	}
	if !strings.Contains(codex, `line='(second opinion request r)'`) {
		t.Fatalf("the line searched for: %s", codex)
	}
}
