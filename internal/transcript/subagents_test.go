package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

const (
	forkRef   = `{"type":"fork-context-ref","agentId":"a1","parentSessionId":"s","parentLastUuid":"x","contextLength":10}`
	subTool   = `{"parentUuid":null,"isSidechain":true,"agentId":"a1","type":"assistant","uuid":"sa1","timestamp":"2026-10-24T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_b","name":"Bash","input":{"command":"go test ./..."}}]}}`
	subResult = `{"isSidechain":true,"agentId":"a1","type":"user","uuid":"su1","timestamp":"2026-10-24T10:00:03Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_b","content":"ok"}]}}`
	subReport = `{"isSidechain":true,"agentId":"a1","type":"assistant","uuid":"sa2","timestamp":"2026-10-24T10:00:05Z","message":{"role":"assistant","content":[{"type":"text","text":"**Done:** the tests pass.\n\nDetails follow."}]}}`
	metaOpen  = `{"agentType":"fork","isFork":true,"worktreeBranch":"wt-a1","description":"PR: quiet hours","toolUseId":"toolu_1","requestShape":"background","model":"inherit"}`
	metaDone  = `{"agentType":"fork","isFork":true,"worktreeBranch":"wt-a1","description":"PR: quiet hours","toolUseId":"toolu_1","requestShape":"background","model":"inherit","finished":true}`
)

// a session file with one subagent beside it, as Claude Code lays them out
func sessionWithSubagent(t *testing.T, meta string, lines ...string) (session, dir string) {
	t.Helper()
	root := t.TempDir()
	session = filepath.Join(root, "s.jsonl")
	parent := `{"type":"assistant","uuid":"a1","timestamp":"2026-10-24T09:59:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"Agent","input":{"description":"PR: quiet hours","prompt":"build it"}}]}}`
	if err := os.WriteFile(session, []byte(parent+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir = filepath.Join(root, "s", "subagents")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeSubagent(t, dir, meta, lines...)
	return session, dir
}

func writeSubagent(t *testing.T, dir, meta string, lines ...string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "agent-a1.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent-a1.meta.json"), []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSubagentsReadTheFolderBesideTheSession(t *testing.T) {
	session, dir := sessionWithSubagent(t, metaOpen, forkRef, subTool)
	reader := NewReader(shell.Local{})
	now := time.Now()

	subs, stamp1, err := reader.Subagents(session, now)
	if err != nil || len(subs) != 1 {
		t.Fatalf("subagents = %+v, %v", subs, err)
	}
	s := subs[0]
	if s.ID != "agent-a1" || s.ToolID != "toolu_1" || s.Kind != "fork" || s.Branch != "wt-a1" || s.Model != "" || s.Description != "PR: quiet hours" {
		t.Errorf("meta not read: %+v", s)
	}
	if s.Status != SubagentWorking || s.Tools != 1 || s.Doing == nil || s.Doing.ToolName != "Bash" || s.Started != "2026-10-24T10:00:00Z" || s.Report != "" {
		t.Errorf("a working subagent = %+v", s)
	}
	if later, _, _ := reader.Subagents(session, now.Add(QuietAfter+time.Minute)); later[0].Status != SubagentQuiet {
		t.Errorf("silent past QuietAfter should read quiet: %+v", later[0])
	}
	if _, again, _ := reader.Subagents(session, now); again != stamp1 {
		t.Errorf("nothing changed, the stamp moved: %s → %s", stamp1, again)
	}

	// a report with no finish written yet: done once it stood for ReportedAfter, at work until then
	writeSubagent(t, dir, metaOpen, forkRef, subTool, subResult, subReport)
	if reported, _, _ := reader.Subagents(session, now); reported[0].Status != SubagentWorking {
		t.Errorf("a report just written is not yet a finish: %+v", reported[0])
	}
	if reported, _, _ := reader.Subagents(session, now.Add(ReportedAfter+time.Second)); reported[0].Status != SubagentDone {
		t.Errorf("a report that stood should read done without the meta's finish: %+v", reported[0])
	}

	writeSubagent(t, dir, metaDone, forkRef, subTool, subResult, subReport)
	subs, stamp2, err := reader.Subagents(session, now)
	if err != nil || len(subs) != 1 {
		t.Fatalf("subagents after the finish = %+v, %v", subs, err)
	}
	s = subs[0]
	if s.Status != SubagentDone || s.Doing != nil || !strings.HasPrefix(s.Report, "**Done:**") || s.Tools != 1 || s.Failed != 0 || s.Took != 5 || s.Ended != "2026-10-24T10:00:05Z" {
		t.Errorf("a finished subagent = %+v", s)
	}
	if stamp2 == stamp1 {
		t.Error("the files changed, the stamp did not")
	}

	none, stamp, err := reader.Subagents(filepath.Join(filepath.Dir(session), "other.jsonl"), now)
	if err != nil || len(none) != 0 || stamp != "" {
		t.Errorf("a session without the folder = %+v, %q, %v", none, stamp, err)
	}
}

func TestSubagentTurnsKeepTheSidechainRecords(t *testing.T) {
	session, dir := sessionWithSubagent(t, metaOpen, forkRef, subTool, subResult, subReport)
	reader := NewReader(shell.Local{})

	turns, version, err := reader.SubagentTurns(session, "agent-a1")
	if err != nil || version.Size == 0 {
		t.Fatalf("turns = %v, version %v", err, version)
	}
	if len(turns) != 1 || len(turns[0].Parts) != 2 || turns[0].Parts[0].Output != "ok" || turns[0].Parts[1].Text == "" {
		t.Errorf("the subagent's own records should make its turns: %+v", turns)
	}
	if plain, _ := Parse(filepath.Join(dir, "agent-a1.jsonl")); len(plain) != 0 {
		t.Errorf("Parse drops sidechain records, as for the parent file: %+v", plain)
	}
	for _, bad := range []string{"", "../s", "a/b", "agent.a1", "agent-zz"} {
		if _, _, err := reader.SubagentTurns(session, bad); err == nil {
			t.Errorf("%q should be refused", bad)
		}
	}
}

func TestSubagentIDNamesOnlyAFileStem(t *testing.T) {
	for id, want := range map[string]bool{"agent-a0a9a5b87ea2ba5b6": true, "x_1": true, "": false, "../x": false, "a.b": false, "a b": false, strings.Repeat("a", 65): false} {
		if got := SubagentID(id); got != want {
			t.Errorf("SubagentID(%q) = %v", id, got)
		}
	}
}
