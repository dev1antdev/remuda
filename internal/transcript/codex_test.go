package transcript

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

// codexSample is a rollout as Codex 0.164 writes one, anonymized: the harness envelopes, a prompt, a commentary
// message, a JS exec call whose second command failed, a plain function call, a plan, a question, the token records.
const codexSample = `{"timestamp":"2026-10-29T13:27:52.000Z","type":"session_meta","payload":{"session_id":"01a187d8-720d-7860-9918-8fc3e6cd3a1f","id":"01a187d8-720d-7860-9918-8fc3e6cd3a1f","timestamp":"2026-10-29T13:27:52.000Z","cwd":"/home/u/proj","originator":"codex-tui","cli_version":"0.164.0"}}
{"timestamp":"2026-10-29T13:27:52.416Z","ordinal":1,"type":"event_msg","payload":{"type":"task_started","turn_id":"t1","model_context_window":475000}}
{"timestamp":"2026-10-29T13:27:53.000Z","ordinal":2,"type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"<permissions instructions>\nnever shown"}]}}
{"timestamp":"2026-10-29T13:27:53.798Z","ordinal":7,"type":"response_item","payload":{"type":"message","id":"msg_env","role":"user","content":[{"type":"input_text","text":"<environment_context>\n  <cwd>/home/u/proj</cwd>\n</environment_context>"}]}}
{"timestamp":"2026-10-29T13:27:53.799Z","ordinal":9,"type":"turn_context","payload":{"turn_id":"t1","cwd":"/home/u/proj","model":"gpt-6.1-sol","summary":"auto","effort":"medium"}}
{"timestamp":"2026-10-29T13:27:53.867Z","ordinal":11,"type":"response_item","payload":{"type":"message","id":"msg_u1","role":"user","content":[{"type":"input_text","text":"Run ls and make hello.txt"}]}}
{"timestamp":"2026-10-29T13:27:53.868Z","ordinal":12,"type":"event_msg","payload":{"type":"item_completed","turn_id":"t1","item":{"type":"UserMessage","id":"x","content":[{"type":"text","text":"Run ls and make hello.txt"}]}}}
{"timestamp":"2026-10-29T13:27:55.000Z","ordinal":13,"type":"response_item","payload":{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"**Listing first**\n\nThe folder is small."}],"encrypted_content":"gAAAA"}}
{"timestamp":"2026-10-29T13:27:56.069Z","ordinal":14,"type":"response_item","payload":{"type":"message","status":"completed","id":"msg_a1","role":"assistant","content":[{"type":"output_text","text":"I’ll run ls, then make the file."}],"phase":"commentary"}}
{"timestamp":"2026-10-29T13:27:56.557Z","ordinal":15,"type":"response_item","payload":{"type":"custom_tool_call","id":"ctc_1","status":"completed","call_id":"call_1","name":"exec","input":"text(await tools.exec_command({cmd:\"ls -la\"}));\ntext(await tools.exec_command({cmd:\"printf HELLO > hello.txt\"}));\n"}}
{"timestamp":"2026-10-29T13:27:56.600Z","ordinal":16,"type":"token_usage_record","payload":{"thread_id":"01a187d8-720d-7860-9918-8fc3e6cd3a1f","turn_id":"t1","response_id":"resp_1","usage":{"input_tokens":16443,"cached_input_tokens":12288,"cache_write_input_tokens":0,"output_tokens":82,"reasoning_output_tokens":0,"total_tokens":16525}}}
{"timestamp":"2026-10-29T13:28:02.165Z","ordinal":18,"type":"response_item","payload":{"type":"custom_tool_call_output","id":"ctco_1","call_id":"call_1","output":[{"type":"input_text","text":"Script completed\nWall time 0.2 seconds\nOutput:\n"},{"type":"input_text","text":"{\"chunk_id\":\"d6\",\"wall_time_seconds\":0.0001,\"exit_code\":0,\"original_token_count\":44,\"output\":\"total 8\\nnotes.txt\\n\"}"},{"type":"input_text","text":"{\"chunk_id\":\"42\",\"wall_time_seconds\":0,\"exit_code\":1,\"original_token_count\":11,\"output\":\"zsh:1: operation not permitted: hello.txt\\n\"}"}]}}
{"timestamp":"2026-10-29T13:28:02.166Z","ordinal":19,"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":16443,"cached_input_tokens":12288,"cache_write_input_tokens":0,"output_tokens":82,"reasoning_output_tokens":0,"total_tokens":16525},"last_token_usage":{"input_tokens":16443,"cached_input_tokens":12288,"cache_write_input_tokens":0,"output_tokens":82,"reasoning_output_tokens":0,"total_tokens":16525},"model_context_window":475000}}}
{"timestamp":"2026-10-29T13:28:02.500Z","ordinal":20,"type":"response_item","payload":{"type":"function_call","id":"fc_1","name":"exec_command","arguments":"{\"cmd\":\"cat notes.txt\",\"max_output_tokens\":2000}","call_id":"call_2"}}
{"timestamp":"2026-10-29T13:28:02.900Z","ordinal":21,"type":"response_item","payload":{"type":"function_call_output","call_id":"call_2","output":"Chunk ID: f6\nWall time: 0.3 seconds\nProcess exited with code 0\nOriginal token count: 4\nOutput:\nalpha\nbeta\n"}}
{"timestamp":"2026-10-29T13:28:03.000Z","ordinal":22,"type":"response_item","payload":{"type":"function_call","id":"fc_2","name":"update_plan","arguments":"{\"explanation\":null,\"plan\":[{\"step\":\"List the folder\",\"status\":\"completed\"},{\"step\":\"Make hello.txt\",\"status\":\"in_progress\"}]}","call_id":"call_3"}}
{"timestamp":"2026-10-29T13:28:03.100Z","ordinal":23,"type":"response_item","payload":{"type":"function_call_output","call_id":"call_3","output":"Plan updated"}}
{"timestamp":"2026-10-29T13:28:03.746Z","ordinal":24,"type":"response_item","payload":{"type":"message","status":"completed","id":"msg_a2","role":"assistant","content":[{"type":"output_text","text":"ls showed 2 entries; hello.txt could not be written."}],"phase":"final_answer"}}
{"timestamp":"2026-10-29T13:28:03.750Z","ordinal":25,"type":"token_usage_record","payload":{"thread_id":"01a187d8-720d-7860-9918-8fc3e6cd3a1f","turn_id":"t1","response_id":"resp_2","usage":{"input_tokens":16708,"cached_input_tokens":16384,"cache_write_input_tokens":0,"output_tokens":35,"reasoning_output_tokens":0,"total_tokens":16743}}}
{"timestamp":"2026-10-29T13:28:03.904Z","ordinal":26,"type":"event_msg","payload":{"type":"task_complete","turn_id":"t1","last_agent_message":"ls showed 2 entries; hello.txt could not be written."}}
{"timestamp":"2026-10-29T13:30:00.000Z","ordinal":27,"type":"response_item","payload":{"type":"message","id":"msg_u2","role":"user","content":[{"type":"input_text","text":"Ask me which name to use"}]}}
{"timestamp":"2026-10-29T13:30:01.000Z","ordinal":28,"type":"response_item","payload":{"type":"function_call","id":"fc_3","name":"request_user_input","arguments":"{\"questions\":[{\"id\":\"name\",\"header\":\"Name\",\"question\":\"Which name?\",\"options\":[{\"label\":\"hello.txt\"},{\"label\":\"hi.txt\"}]}]}","call_id":"call_4"}}
`

func TestParseCodexReadsPromptsAnswersAndCalls(t *testing.T) {
	turns, err := ParseBytesOf(Codex, []byte(codexSample))
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 4 {
		t.Fatalf("turns: %d, want 4 (user, assistant, user, assistant)", len(turns))
	}
	user := turns[0]
	if user.Role != "user" || len(user.Parts) != 1 || user.Parts[0].Text != "Run ls and make hello.txt" || user.IDs[0] != "msg_u1" {
		t.Fatalf("the prompt, without the environment envelope: %+v", user)
	}
	answer := turns[1]
	if answer.Role != "assistant" {
		t.Fatalf("second turn: %+v", answer)
	}
	kinds := make([]string, 0, len(answer.Parts))
	for _, p := range answer.Parts {
		kinds = append(kinds, p.Type+":"+p.ToolName)
	}
	want := "thinking: text: tool:exec tool:exec_command tool:update_plan text:"
	if got := strings.Join(kinds, " "); got != want {
		t.Fatalf("parts: %s, want %s", got, want)
	}
	if answer.Parts[0].Text != "**Listing first**\n\nThe folder is small." {
		t.Fatalf("reasoning summary: %q", answer.Parts[0].Text)
	}
	exec := answer.Parts[2]
	if exec.ToolID != "call_1" || !exec.IsError || !strings.Contains(exec.Output, "total 8\nnotes.txt") || !strings.Contains(exec.Output, "exit code 1\nzsh:1: operation not permitted") {
		t.Fatalf("the JS exec call, its chunks unwrapped and the failed command noted: %+v", exec)
	}
	if exec.Took < 5 || exec.Took > 6 {
		t.Fatalf("took: %v", exec.Took)
	}
	if InputField(exec.Input, "input") == "" {
		t.Fatalf("a custom tool's text is its input: %s", exec.Input)
	}
	cat := answer.Parts[3]
	if InputField(cat.Input, "cmd") != "cat notes.txt" || cat.IsError || !strings.HasSuffix(cat.Output, "alpha\nbeta\n") {
		t.Fatalf("exec_command: %+v", cat)
	}
	if ShellCommand(&cat) != "cat notes.txt" {
		t.Fatalf("ShellCommand: %q", ShellCommand(&cat))
	}
	if answer.Answer() != "ls showed 2 entries; hello.txt could not be written." {
		t.Fatalf("Answer: %q", answer.Answer())
	}
	if answer.Tokens != 16443+82+16708+35 {
		t.Fatalf("tokens: %d", answer.Tokens)
	}
	if len(answer.Responses) != 2 || answer.Responses[0].Model != "gpt-6.1-sol" || answer.Responses[0].Window != 475000 || answer.Responses[0].Cwd != "/home/u/proj" {
		t.Fatalf("responses: %+v", answer.Responses)
	}
	if r := answer.Responses[1]; r.Input != 16708-16384 || r.CacheRead != 16384 || r.Output != 35 || r.MessageID != "resp_2" {
		t.Fatalf("the second response's tokens: %+v", r)
	}
	if answer.Took < 9 || answer.Took > 11 {
		t.Fatalf("took %v, want about 10 s from the prompt to the last record", answer.Took)
	}
	plan := Plan(turns)
	if len(plan) != 2 || plan[0].Text != "List the folder" || plan[0].Status != "completed" || plan[1].Status != "in_progress" || plan[1].ID != "2" {
		t.Fatalf("plan: %+v", plan)
	}
	question := turns[3].Parts[0]
	if !IsQuestion(question.ToolName) || question.Output != "" {
		t.Fatalf("the open question: %+v", question)
	}
}

func TestCodexUsageScannerCarriesTheModel(t *testing.T) {
	scanner := NewUsageScanner("/home/u/.codex/sessions/2026/10/29/rollout-2026-10-29T13-27-52-01a187d8-720d-7860-9918-8fc3e6cd3a1f.jsonl")
	var recs []UsageRecord
	for _, line := range strings.Split(codexSample, "\n") {
		if r, ok := scanner.Scan([]byte(line)); ok {
			recs = append(recs, r)
		}
	}
	if len(recs) != 2 {
		t.Fatalf("records: %d", len(recs))
	}
	for _, r := range recs {
		if r.Model != "gpt-6.1-sol" || r.Cwd != "/home/u/proj" || r.Window != 475000 {
			t.Fatalf("a record without what the lines before it said: %+v", r)
		}
	}
	if recs[0].MessageID != "resp_1" || recs[1].MessageID != "resp_2" {
		t.Fatalf("ids: %+v", recs)
	}
}

func TestMatchCodexLine(t *testing.T) {
	lines := strings.Split(codexSample, "\n")
	var hits []Match
	for _, line := range lines {
		if m, ok := MatchCodexLine([]byte(line), "HELLO.txt"); ok {
			hits = append(hits, m)
		}
	}
	if len(hits) != 2 || hits[0].UUID != "msg_u1" || hits[0].Role != "user" || hits[1].UUID != "msg_a2" || hits[1].Role != "assistant" {
		t.Fatalf("hits: %+v", hits)
	}
	if hits[0].Ts.IsZero() || !strings.Contains(hits[0].Snippet, "hello.txt") {
		t.Fatalf("first hit: %+v", hits[0])
	}
	for _, line := range lines { // the environment envelope, the developer message and a tool's text never match
		if m, ok := MatchCodexLine([]byte(line), "cwd"); ok {
			t.Fatalf("matched the envelope: %+v", m)
		}
		if m, ok := MatchCodexLine([]byte(line), "never shown"); ok {
			t.Fatalf("matched the developer message: %+v", m)
		}
		if m, ok := MatchCodexLine([]byte(line), "operation not permitted"); ok {
			t.Fatalf("matched a tool's output: %+v", m)
		}
	}
}

func TestCodexFilesAreFoundByThreadId(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	id := "01a187d8-720d-7860-9918-8fc3e6cd3a1f"
	dir := filepath.Join(home, ".codex", "sessions", "2026", "10", "29")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rollout-2026-10-29T13-27-52-"+id+".jsonl")
	if err := os.WriteFile(path, []byte(codexSample), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "session_index.jsonl"), []byte(`{"id":"`+id+`","thread_name":"Make hello","updated_at":"2026-10-29T13:28:03Z"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if CodexSessionID(path) != id || SessionID(path) != id {
		t.Fatalf("id from the name: %q", CodexSessionID(path))
	}
	if FormatOf(path) != Codex || FormatOfAgent("codex") != Codex || FormatOfAgent("claude") != Claude || FormatOfAgent("kimi") != "" {
		t.Fatal("formats")
	}
	roots := RootsOf(home)
	for _, format := range []Format{Codex, ""} {
		found, err := findSessionIn(roots, format, "/home/u/proj", id)
		if err != nil || found != path {
			t.Fatalf("findSessionIn(%q): %q %v", format, found, err)
		}
	}
	if _, err := findSessionIn(roots, Claude, "/home/u/proj", id); err == nil {
		t.Fatal("found a rollout among Claude Code's files")
	}
	if _, err := findSessionIn(roots, Codex, "", "../etc"); err == nil {
		t.Fatal("a path passed as an id")
	}
	if CodexCwd(path) != "/home/u/proj" {
		t.Fatalf("cwd: %q", CodexCwd(path))
	}
	sessions, err := sessionsIn(roots, home, Codex, "/home/u/proj")
	if err != nil || len(sessions) != 1 || sessions[0].ID != id || sessions[0].Title != "Make hello" || sessions[0].Agent != "codex" {
		t.Fatalf("sessions of the folder: %+v %v", sessions, err)
	}
	if other, _ := sessionsIn(roots, home, Codex, "/elsewhere"); len(other) != 0 {
		t.Fatalf("another folder's listing: %+v", other)
	}
	turns, err := Parse(path)
	if err != nil || len(turns) != 4 {
		t.Fatalf("Parse by path: %d %v", len(turns), err)
	}
	if !UnderRoots(RootsOf(home), home, path) || UnderRoots(RootsOf(home), home, filepath.Join(home, "elsewhere.jsonl")) {
		t.Fatal("UnderRoot")
	}
	if got := findScript(RootsOf("/h"), Codex, "/h", "/p", id); !strings.Contains(got, "find '/h/.codex/sessions' -type f \\( -name 'rollout-*-"+id+".jsonl' -o -name 'rollout-*-"+id+"_*.jsonl' \\)") {
		t.Fatalf("remote find: %s", got)
	}
	if got := findScript(RootsOf("/h"), "", "/h", "/p", id); strings.Count(got, "exit 0") != 4 || !strings.Contains(got, "/.claude/projects") || !strings.Contains(got, ".gemini/tmp") {
		t.Fatalf("remote find across formats: %s", got)
	}
	_ = time.Now()
}

// codexRevert is a thread's rollout and the file a revert of its history wrote beside it: the same thread id, a
// rollout id of its own, and a session_meta naming the file it replays to an ordinal.
func codexRevert(t *testing.T, home, id string) (base, revert string) {
	t.Helper()
	dir := filepath.Join(home, ".codex", "sessions", "2026", "11", "20")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := func(ordinal int, role, text string) string {
		return `{"timestamp":"2026-11-20T02:0` + strconv.Itoa(ordinal) + `:00.000Z","ordinal":` + strconv.Itoa(ordinal) +
			`,"type":"response_item","payload":{"type":"message","id":"msg_` + text + `","role":"` + role + `","content":[{"type":"` +
			map[string]string{"user": "input_text", "assistant": "output_text"}[role] + `","text":"` + text + `"}]}}`
	}
	meta := `{"timestamp":"2026-11-20T02:00:00.000Z","type":"session_meta","payload":{"session_id":"` + id + `","id":"` + id + `","cwd":"/home/u/proj","originator":"codex-tui"}}`
	base = filepath.Join(dir, "rollout-2026-11-20T02-00-00-"+id+".jsonl")
	body := meta + "\n" + line(1, "user", "first prompt") + "\n" + line(2, "assistant", "first answer") + "\n" + line(3, "user", "reverted prompt") + "\n"
	if err := os.WriteFile(base, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	revert = filepath.Join(dir, "rollout-2026-11-20T03-00-00-"+id+"_0f0e0d0c-1111-4222-8333-944455556666.jsonl")
	head := `{"timestamp":"2026-11-20T03:00:00.000Z","type":"session_meta","payload":{"session_id":"` + id + `","id":"` + id + `","cwd":"/home/u/proj","originator":"codex-tui","history_base":{"path":"` + base + `","end_ordinal_exclusive":3}}}`
	body = head + "\n" + line(4, "user", "after revert") + "\n"
	if err := os.WriteFile(revert, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return base, revert
}

// TestACodexRevertReadsTheThreadWhole: after a revert of its history Codex writes a new file that keeps the thread's id
// and replays the part that stayed; the chat shows that part and what came after, the reverted turns are gone, and the
// listing names the thread once.
// TestARevertChainKeepsEverySavedPart: a second revert replays the first one's saved part before its own records.
func TestARevertChainKeepsEverySavedPart(t *testing.T) {
	home := t.TempDir()
	id := "01a187d8-720d-7860-9918-8fc3e6cd3a1f"
	_, first := codexRevert(t, home, id)
	second := filepath.Join(filepath.Dir(first), "rollout-2026-11-20T04-00-00-"+id+"_1f0e0d0c-1111-4222-8333-944455556666.jsonl")
	head := `{"timestamp":"2026-11-20T04:00:00.000Z","type":"session_meta","payload":{"session_id":"` + id + `","id":"` + id + `","cwd":"/home/u/proj","originator":"codex-tui","history_base":{"path":"` + first + `","end_ordinal_exclusive":5}}}`
	line := `{"timestamp":"2026-11-20T04:01:00.000Z","ordinal":5,"type":"response_item","payload":{"type":"message","id":"msg_second","role":"user","content":[{"type":"input_text","text":"after second revert"}]}}`
	if err := os.WriteFile(second, []byte(head+"\n"+line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	turns, err := parseCodexFile(second)
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, turn := range turns {
		for _, part := range turn.Parts {
			if part.Type == "text" {
				texts = append(texts, part.Text)
			}
		}
	}
	if got := strings.Join(texts, " | "); got != "first prompt | first answer | after revert | after second revert" {
		t.Fatalf("a chain of reverts: %s", got)
	}
}

// TestAMissingHistoryBaseStillShowsTheNewFile: a replayed file whose base was removed gives its own records, not an
// error that would leave the chat empty.
func TestAMissingHistoryBaseStillShowsTheNewFile(t *testing.T) {
	home := t.TempDir()
	id := "01a187d8-720d-7860-9918-8fc3e6cd3a1f"
	base, revert := codexRevert(t, home, id)
	if err := os.Remove(base); err != nil {
		t.Fatal(err)
	}
	turns, err := parseCodexFile(revert)
	if err != nil {
		t.Fatal(err)
	}
	seen := ""
	for _, turn := range turns {
		for _, part := range turn.Parts {
			if part.Type == "text" {
				seen += part.Text
			}
		}
	}
	if seen != "after revert" {
		t.Fatalf("a missing base shows %q, want the new file's prompt", seen)
	}
}

// TestARevertIsResolvedAgainByTheSameReader: a cold resolution kept the base before the revert file appeared; the next
// look finds the newer file, not the cached base.
func TestARevertIsResolvedAgainByTheSameReader(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	id := "01a187d8-720d-7860-9918-8fc3e6cd3a1f"
	base, revert := codexRevert(t, home, id)
	body, err := os.ReadFile(revert)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(revert); err != nil {
		t.Fatal(err)
	}
	reader := NewReader(shell.Local{Name: "local"})
	if got, err := reader.ResolveSession("codex", "/home/u/proj", "id", id); err != nil || got != base {
		t.Fatalf("before the revert: %q, %v; want %s", got, err, base)
	}
	if err := os.WriteFile(revert, body, 0o600); err != nil {
		t.Fatal(err)
	}
	reader.mu.Lock()
	for key := range reader.sessionAt {
		if key.format == Codex {
			reader.sessionAt[key] = time.Now().Add(-codexResolveFresh)
		}
	}
	reader.mu.Unlock()
	if got, err := reader.ResolveSession("codex", "/home/u/proj", "id", id); err != nil || got != revert {
		t.Fatalf("after the revert: %q, %v; want %s", got, err, revert)
	}
}

// TestARevertOnSSHFollowsItsHistory: Reader.Turns over ssh reads both the new file and the base it names.
func TestARevertOnSSHFollowsItsHistory(t *testing.T) {
	home := t.TempDir()
	id := "01a187d8-720d-7860-9918-8fc3e6cd3a1f"
	base, revert := codexRevert(t, home, id)
	host := &shell.Fake{Name: "gog", Dir: home, Remote: true, Answer: func(script string) (string, error) {
		for _, path := range []string{revert, base} {
			if strings.Contains(script, path) {
				data, err := os.ReadFile(path)
				return string(data), err
			}
		}
		return "", nil
	}}
	turns, err := NewReader(host).Turns(revert, Version{})
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, turn := range turns {
		for _, part := range turn.Parts {
			if part.Type == "text" {
				texts = append(texts, part.Text)
			}
		}
	}
	if got := strings.Join(texts, " | "); got != "first prompt | first answer | after revert" {
		t.Fatalf("ssh revert: %s", got)
	}
}

// TestAHistoryBaseOutsideTheRootsIsNotRead: a rollout cannot point history_base at any file on the machine.
func TestAHistoryBaseOutsideTheRootsIsNotRead(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	id := "01a187d8-720d-7860-9918-8fc3e6cd3a1f"
	base, revert := codexRevert(t, home, id)
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "rollout-2026-11-20T05-00-00-"+id+".jsonl")
	line := `{"timestamp":"2026-11-20T05:01:00.000Z","ordinal":1,"type":"response_item","payload":{"type":"message","id":"msg_outside","role":"user","content":[{"type":"input_text","text":"outside the roots"}]}}`
	if err := os.WriteFile(outside, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(revert)
	if err != nil {
		t.Fatal(err)
	}
	body = bytes.Replace(body, []byte(base), []byte(outside), 1)
	if err := os.WriteFile(revert, body, 0o600); err != nil {
		t.Fatal(err)
	}
	turns, err := NewReader(shell.Local{Name: "local"}).Turns(revert, Version{})
	if err != nil {
		t.Fatal(err)
	}
	for _, turn := range turns {
		for _, part := range turn.Parts {
			if strings.Contains(part.Text, "outside the roots") {
				t.Fatalf("a history base outside the roots was read: %+v", turns)
			}
		}
	}
}

func TestACodexRevertReadsTheThreadWhole(t *testing.T) {
	home := t.TempDir()
	id := "01a187d8-720d-7860-9918-8fc3e6cd3a1f"
	base, revert := codexRevert(t, home, id)
	if got := CodexSessionID(revert); got != id {
		t.Fatalf("the thread id of a revert's file: %q", got)
	}
	if got, err := findCodexIn(RootsOf(home)[Codex], id); err != nil || got != revert {
		t.Fatalf("resolved %q, %v; want the revert's file %s", got, err, revert)
	}
	turns, err := parseCodexFile(revert)
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, turn := range turns {
		for _, part := range turn.Parts {
			if part.Type == "text" {
				texts = append(texts, part.Text)
			}
		}
	}
	if got := strings.Join(texts, " | "); got != "first prompt | first answer | after revert" {
		t.Fatalf("the conversation after a revert: %s", got)
	}
	files := CodexFiles(filepath.Join(home, ".codex", "sessions"))
	if len(files) != 2 || files[0].Path != revert || files[1].Path != base {
		t.Fatalf("the physical rollouts are %d: %+v", len(files), files)
	}
	sessions, err := sessionsIn(RootsOf(home), home, Codex, "/home/u/proj")
	if err != nil || len(sessions) != 1 || sessions[0].Path != revert {
		t.Fatalf("the thread is listed %d times: %+v, %v", len(sessions), sessions, err)
	}
}
