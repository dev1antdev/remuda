package transcript

import (
	"fmt"
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

func TestRecentSubagentsReadOnlyTheOnesThatMovedWithinQuietAfter(t *testing.T) {
	session, dir := sessionWithSubagent(t, metaOpen, forkRef, subTool)
	old := filepath.Join(dir, "agent-old.jsonl")
	for _, file := range []string{old, filepath.Join(dir, "agent-old.meta.json")} {
		if err := os.WriteFile(file, []byte(forkRef+"\n"+subTool+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(file, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	reader := NewReader(shell.Local{})

	subs, err := reader.RecentSubagents(session, time.Now())
	if err != nil || len(subs) != 1 || subs[0].ID != "agent-a1" || subs[0].Status != SubagentWorking {
		t.Fatalf("recent subagents = %+v, %v", subs, err)
	}
	if _, read := reader.subagents[old]; read {
		t.Error("a subagent silent past QuietAfter was read: it can only be quiet or done")
	}
	if all, _, _ := reader.Subagents(session, time.Now()); len(all) != 2 {
		t.Errorf("the chat still lists every subagent: %+v", all)
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

func TestTheSubagentCacheLetsGoOfTheSummaryAskedForLongestAgo(t *testing.T) {
	r := NewReader(shell.Local{})
	v := Version{Size: 1, MTime: 1}
	for i := 0; i < SubagentsKept; i++ {
		r.subagents[fmt.Sprintf("/s/agent-%d.jsonl", i)] = &subagentEntry{version: v, used: uint64(i + 1)}
	}
	r.subagentUses = SubagentsKept
	r.subagentEntry("/s/agent-0.jsonl", "agent-0", v, "/s/agent-0.meta.json", Version{}) // asked for again: now the newest
	r.subagentEntry("/s/agent-new.jsonl", "agent-new", v, "/s/agent-new.meta.json", Version{})
	if len(r.subagents) != SubagentsKept {
		t.Fatalf("%d summaries kept, not %d", len(r.subagents), SubagentsKept)
	}
	if r.subagents["/s/agent-0.jsonl"] == nil || r.subagents["/s/agent-1.jsonl"] != nil || r.subagents["/s/agent-new.jsonl"] == nil {
		t.Fatal("the summary asked for longest ago goes, the one just asked for again stays")
	}
}

// twoSubagents is a session with the subagent toolu_1 spawned and another, toolu_2's, both silent for an hour.
func twoSubagents(t *testing.T) (session, first, second string) {
	t.Helper()
	session, dir := sessionWithSubagent(t, metaDone, forkRef, subTool, subResult, subReport)
	first = filepath.Join(dir, "agent-a1.jsonl")
	second = filepath.Join(dir, "agent-old.jsonl")
	if err := os.WriteFile(second, []byte(forkRef+"\n"+subTool+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent-old.meta.json"), []byte(strings.Replace(metaDone, "toolu_1", "toolu_2", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	long := time.Now().Add(-time.Hour)
	for _, name := range []string{"agent-a1.jsonl", "agent-a1.meta.json", "agent-old.jsonl", "agent-old.meta.json"} {
		if err := os.Chtimes(filepath.Join(dir, name), long, long); err != nil {
			t.Fatal(err)
		}
	}
	return session, first, second
}

// A chat reads the subagents its turns' calls spawned and the ones that may still work, never the session's every one.
func TestSubagentsForReadsOnlyTheOnesAChatNeeds(t *testing.T) {
	session, first, second := twoSubagents(t)
	reader := NewReader(shell.Local{})

	none, err := reader.SubagentsFor(session, nil, time.Now())
	if err != nil || len(none) != 0 || len(reader.subagents) != 0 {
		t.Fatalf("with no call on screen and nothing at work: %+v, %v, read %d", none, err, len(reader.subagents))
	}
	subs, err := reader.SubagentsFor(session, map[string]bool{"toolu_1": true}, time.Now())
	if err != nil || len(subs) != 1 || subs[0].ID != "agent-a1" || subs[0].ToolID != "toolu_1" || subs[0].Status != SubagentDone {
		t.Fatalf("the call's subagent = %+v, %v", subs, err)
	}
	if _, read := reader.subagents[second]; read {
		t.Error("a subagent no call on screen spawned was read")
	}
	if _, read := reader.subagents[first]; !read {
		t.Error("the call's subagent was not kept")
	}
	// one whose transcript just moved may be at work: it is answered whatever the calls on screen
	now := time.Now()
	if err := os.Chtimes(second, now, now); err != nil {
		t.Fatal(err)
	}
	recent, err := reader.SubagentsFor(session, nil, now)
	if err != nil || len(recent) != 1 || recent[0].ID != "agent-old" {
		t.Fatalf("the one that may be at work = %+v, %v", recent, err)
	}
	if !reader.HasSubagent(session, "agent-old", now) || reader.HasSubagent(session, "agent-zz", now) || reader.HasSubagent(session, "../s", now) {
		t.Error("HasSubagent does not go by the folder's listing")
	}
}

// On a remote the folder's meta files come in one round trip, and a transcript nobody asked for is never fetched.
func TestSubagentsForReadsARemoteFolderInFewRoundTrips(t *testing.T) {
	session, _, second := twoSubagents(t)
	host := remoteHere(t.TempDir())
	reader := NewReader(host)

	subs, err := reader.SubagentsFor(session, map[string]bool{"toolu_1": true}, time.Now())
	if err != nil || len(subs) != 1 || subs[0].ID != "agent-a1" || subs[0].Description != "PR: quiet hours" {
		t.Fatalf("the call's subagent over ssh = %+v, %v", subs, err)
	}
	metas := 0
	for _, script := range host.Scripts() {
		if strings.Contains(script, "cat \"$f\"") { // the script that prints every meta file, not the folder's listing
			metas++
		}
		if strings.Contains(script, second) {
			t.Errorf("a transcript nobody asked for was read: %s", script)
		}
	}
	if metas != 1 {
		t.Errorf("the folder's meta files were read in %d round trips", metas)
	}
	before := len(host.Scripts())
	if again, err := reader.SubagentsFor(session, map[string]bool{"toolu_1": true}, time.Now()); err != nil || len(again) != 1 {
		t.Fatalf("asked again: %+v, %v", again, err)
	}
	if after := len(host.Scripts()); after != before {
		t.Errorf("asked again a moment later, %d more scripts ran", after-before)
	}
}

func TestParseMetasReadsEachFileByItsName(t *testing.T) {
	answer := "\x1eagent-a1.meta.json\x1f" + metaDone + "\x1eagent-b2.meta.json\x1f{\"toolUseId\":\"toolu_9\",\"description\":\"cut sho" +
		"\x1e../evil.meta.json\x1f{\"toolUseId\":\"x\"}\x1enotes.txt\x1f{}"
	metas := parseMetas(answer)
	if len(metas) != 1 || metas["agent-a1.meta.json"].ToolUseID != "toolu_1" {
		t.Fatalf("metas = %+v", metas)
	}
}
