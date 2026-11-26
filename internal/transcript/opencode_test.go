package transcript

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/dev1antdev/remuda/internal/shell"
)

// opencodeCheckout makes a home with opencode's database in it and returns the folder a session names and its id.
const opencodeTestBase = int64(1_795_000_000_000)

func opencodeCheckout(t *testing.T) (home, dir, id string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	data := filepath.Join(home, ".local", "share")
	t.Setenv("XDG_DATA_HOME", data)
	dir = filepath.Join(home, "code", "app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(data, "opencode", "opencode.db")
	if err := os.MkdirAll(filepath.Dir(db), 0o755); err != nil {
		t.Fatal(err)
	}
	handle, err := sql.Open("sqlite", "file:"+db)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	for _, schema := range []string{
		`CREATE TABLE session_v2 (id text PRIMARY KEY, project_id text NOT NULL, parent_id text, directory text NOT NULL, title text, model text, cost real DEFAULT 0 NOT NULL, tokens_input integer DEFAULT 0 NOT NULL, tokens_output integer DEFAULT 0 NOT NULL, tokens_reasoning integer DEFAULT 0 NOT NULL, tokens_cache_read integer DEFAULT 0 NOT NULL, tokens_cache_write integer DEFAULT 0 NOT NULL, time_created integer NOT NULL, time_updated integer NOT NULL, time_idle integer)`,
		`CREATE TABLE session_message (id text PRIMARY KEY, session_id text NOT NULL, type text NOT NULL, seq integer NOT NULL, time_created integer NOT NULL, time_updated integer NOT NULL, data text NOT NULL)`,
		`CREATE TABLE session_part (message_id text NOT NULL, session_id text NOT NULL, ordinal integer NOT NULL, type text NOT NULL, call_id text, time_created integer NOT NULL, time_updated integer NOT NULL, data text NOT NULL, PRIMARY KEY(message_id, ordinal))`,
	} {
		if _, err := handle.Exec(schema); err != nil {
			t.Fatal(err)
		}
	}
	id = "ses_test1"
	if _, err := handle.Exec(`INSERT INTO session_v2 (id, project_id, directory, title, model, cost, tokens_input, tokens_output, tokens_cache_read, time_created, time_updated) VALUES (?, ?, ?, ?, ?, 1.5, 300, 40, 700, ?, ?)`,
		id, "prj_1", dir, "A session", `{"id":"deepseek-v4.1-flash","providerID":"opencode-go"}`, opencodeTestBase, opencodeTestBase+3000); err != nil {
		t.Fatal(err)
	}
	insertMessage := func(messageID, kind string, seq, at int64, data string) {
		if _, err := handle.Exec(`INSERT INTO session_message (id, session_id, type, seq, time_created, time_updated, data) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			messageID, id, kind, seq, at, at, data); err != nil {
			t.Fatal(err)
		}
	}
	insertPart := func(messageID string, ordinal int, data string) {
		if _, err := handle.Exec(`INSERT INTO session_part (message_id, session_id, ordinal, type, time_created, time_updated, data) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			messageID, id, ordinal, "x", 1000, 1000, data); err != nil {
			t.Fatal(err)
		}
	}
	insertMessage("msg_u1", "user", 1, opencodeTestBase, fmt.Sprintf(`{"time":{"created":%d},"text":"do the thing"}`, opencodeTestBase))
	toolInput, _ := json.Marshal(map[string]any{"path": "/code/app/a.txt", "oldString": "one", "newString": "two"})
	insertMessage("msg_a1", "assistant", 2, opencodeTestBase+100, fmt.Sprintf(`{"time":{"created":%d,"completed":%d},"model":{"id":"deepseek-v4.1-flash"},"tokens":{"input":100,"output":10,"reasoning":5,"cache":{"read":200,"write":0}}}`, opencodeTestBase+100, opencodeTestBase+500))
	insertPart("msg_a1", 0, `{"type":"reasoning","text":"thinking about it"}`)
	insertPart("msg_a1", 1, fmt.Sprintf(`{"type":"tool","id":"call_1","name":"edit","state":{"status":"completed","input":%s,"content":[{"type":"text","text":"Edited a.txt"}],"time":{"created":%d,"completed":%d}}}`, string(toolInput), opencodeTestBase+200, opencodeTestBase+400))
	insertMessage("msg_a2", "assistant", 3, opencodeTestBase+600, fmt.Sprintf(`{"time":{"created":%d,"completed":%d},"model":{"id":"deepseek-v4.1-flash"},"tokens":{"input":50,"output":20,"reasoning":0,"cache":{"read":100,"write":0}}}`, opencodeTestBase+600, opencodeTestBase+1000))
	insertPart("msg_a2", 0, `{"type":"text","text":"done"}`)
	return home, dir, id
}

func TestAnOpencodeSessionIsFoundAndRead(t *testing.T) {
	_, dir, id := opencodeCheckout(t)
	reader := NewReader(shell.Local{Name: "local"})
	sessions, err := reader.Sessions(Opencode, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].ID != id || sessions[0].Title != "A session" || sessions[0].Agent != "opencode" {
		t.Fatalf("sessions: %+v", sessions)
	}
	path, err := reader.ResolveSession("opencode", dir, "", "")
	if err != nil || path != sessions[0].Path {
		t.Fatalf("resolved %q, %v", path, err)
	}
	version, err := reader.Version(path)
	if err != nil || version.Time().UnixMilli() != opencodeTestBase+3000 {
		t.Fatalf("version %+v, %v", version, err)
	}
	turns, err := reader.Turns(path, version)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[0].Role != "user" || turns[1].Role != "assistant" {
		t.Fatalf("turns: %+v", turns)
	}
	if turns[0].Parts[0].Text != "do the thing" {
		t.Errorf("the prompt: %+v", turns[0].Parts)
	}
	if len(turns[1].IDs) != 2 || turns[1].Took != 1 || turns[1].Output != 30 || turns[1].Tokens != 485 {
		t.Errorf("the answer: %+v", turns[1])
	}
	var edit *Part
	for i := range turns[1].Parts {
		if turns[1].Parts[i].Type == "tool" {
			edit = &turns[1].Parts[i]
		}
	}
	if edit == nil || edit.ToolName != "Edit" || edit.ToolID != "call_1" || edit.Output != "Edited a.txt" || edit.Took != 0.2 {
		t.Fatalf("the tool: %+v", edit)
	}
	var fields map[string]string
	if json.Unmarshal(edit.Input, &fields) != nil || fields["file_path"] != "/code/app/a.txt" || fields["old_string"] != "one" || fields["new_string"] != "two" {
		t.Errorf("the tool's input: %s", edit.Input)
	}
	usage, err := reader.OpencodeUsage(path)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Context != 150 || usage.Cost != 1.5 || usage.Turns != 2 || usage.Model != "deepseek-v4.1-flash" || usage.Updated != opencodeTestBase+3000 {
		t.Errorf("usage: %+v", usage)
	}
}

func opencodeWithSubagent(t *testing.T) (dir, parentID, childID, toolID string) {
	t.Helper()
	_, dir, parentID = opencodeCheckout(t)
	db := filepath.Join(os.Getenv("XDG_DATA_HOME"), "opencode", "opencode.db")
	handle, err := sql.Open("sqlite", "file:"+db)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	childID, toolID = "ses_child1", "call_spawn"
	if _, err := handle.Exec(`INSERT INTO session_v2 (id, project_id, parent_id, directory, title, model, time_created, time_updated, time_idle) VALUES (?, 'prj_1', ?, ?, 'Atlas review', '{"id":"deepseek-v4.1-flash"}', ?, ?, ?)`,
		childID, parentID, dir, opencodeTestBase+2100, opencodeTestBase+3000, opencodeTestBase+3000); err != nil {
		t.Fatal(err)
	}
	spawn := fmt.Sprintf(`{"type":"tool","id":%q,"name":"subagent","state":{"status":"completed","input":{"agent":"general","description":"Atlas review","prompt":"review it"},"content":[{"type":"text","text":"The subagent is working in the background (sessionID: %s)."}],"time":{"created":%d,"completed":%d}}}`, toolID, childID, opencodeTestBase+2100, opencodeTestBase+2200)
	if _, err := handle.Exec(`INSERT INTO session_part (message_id, session_id, ordinal, type, time_created, time_updated, data) VALUES ('msg_a2', ?, 1, 'tool', ?, ?, ?)`,
		parentID, opencodeTestBase+2100, opencodeTestBase+2200, spawn); err != nil {
		t.Fatal(err)
	}
	if _, err := handle.Exec(`INSERT INTO session_message (id, session_id, type, seq, time_created, time_updated, data) VALUES ('msg_c1', ?, 'assistant', 1, ?, ?, ?)`,
		childID, opencodeTestBase+2300, opencodeTestBase+2300, fmt.Sprintf(`{"time":{"created":%d,"completed":%d},"model":{"id":"deepseek-v4.1-flash"},"tokens":{"input":10,"output":5}}`, opencodeTestBase+2300, opencodeTestBase+2400)); err != nil {
		t.Fatal(err)
	}
	if _, err := handle.Exec(`INSERT INTO session_part (message_id, session_id, ordinal, type, time_created, time_updated, data) VALUES ('msg_c1', ?, 0, 'tool', ?, ?, ?)`,
		childID, opencodeTestBase+2300, opencodeTestBase+2300, `{"type":"tool","id":"call_c1","name":"shell","state":{"status":"error","input":{"command":"false"},"content":[{"type":"text","text":"exit 1"}]}}`); err != nil {
		t.Fatal(err)
	}
	if _, err := handle.Exec(`INSERT INTO session_part (message_id, session_id, ordinal, type, time_created, time_updated, data) VALUES ('msg_c1', ?, 1, 'text', ?, ?, ?)`,
		childID, opencodeTestBase+2400, opencodeTestBase+2400, `{"type":"text","text":"Reviewed it: fits, with one note."}`); err != nil {
		t.Fatal(err)
	}
	return dir, parentID, childID, toolID
}

func TestAnOpencodeSessionNamesItsSubagentsUnderTheirCall(t *testing.T) {
	dir, parentID, childID, toolID := opencodeWithSubagent(t)
	reader := NewReader(shell.Local{Name: "local"})
	path, err := reader.ResolveSession("opencode", dir, "id", parentID)
	if err != nil {
		t.Fatal(err)
	}
	subs, err := reader.SubagentsFor(path, map[string]bool{toolID: true}, time.UnixMilli(opencodeTestBase+3500))
	if err != nil || len(subs) != 1 {
		t.Fatalf("subagents: %+v, %v", subs, err)
	}
	sub := subs[0]
	if sub.ID != childID || sub.ToolID != toolID || sub.Description != "Atlas review" || sub.Kind != "general" ||
		sub.Model != "deepseek-v4.1-flash" || sub.Status != SubagentDone || sub.Tools != 1 || sub.Failed != 1 ||
		sub.Report != "Reviewed it: fits, with one note." || sub.Took != 0.9 {
		t.Fatalf("the subagent: %+v", sub)
	}
	if none, _ := reader.SubagentsFor(path, map[string]bool{}, time.UnixMilli(opencodeTestBase+int64(20*time.Minute/time.Millisecond))); len(none) != 0 {
		t.Errorf("a call not on screen was shown: %+v", none)
	}
	if fresh, _ := reader.SubagentsFor(path, map[string]bool{}, time.UnixMilli(opencodeTestBase+2200)); len(fresh) != 1 {
		t.Errorf("a child at work was left out: %+v", fresh)
	}
	turns, version, found, err := reader.OpencodeSubagent(path, childID)
	if err != nil || !found || version.MTime == 0 || len(turns) != 1 {
		t.Fatalf("the child's own conversation: %d turns, %v, %v, %v", len(turns), version, found, err)
	}
	if _, _, found, _ := reader.OpencodeSubagent(path, "ses_other"); found {
		t.Error("a session that is no child of this one opened as one")
	}
}

func TestAnOpencodeSessionAnswersAreSummedSinceAMoment(t *testing.T) {
	_, dir, _ := opencodeCheckout(t)
	reader := NewReader(shell.Local{Name: "local"})
	path, _ := reader.ResolveSession("opencode", dir, "", "")
	responses, err := reader.OpencodeSince(time.UnixMilli(opencodeTestBase + 500))
	if err != nil {
		t.Fatal(err)
	}
	if len(responses) != 1 || responses[0].MessageID != "msg_a2" || responses[0].Model != "deepseek-v4.1-flash" ||
		responses[0].Input != 50 || responses[0].Output != 20 || responses[0].CacheRead != 100 || responses[0].Session != SessionID(path) {
		t.Fatalf("responses: %+v", responses)
	}
}

func TestAnOpencodeSessionListNamesOnlyItsFolder(t *testing.T) {
	_, dir, id := opencodeCheckout(t)
	reader := NewReader(shell.Local{Name: "local"})
	other := t.TempDir()
	sessions, err := reader.Sessions(Opencode, other)
	if err != nil || len(sessions) != 0 {
		t.Fatalf("another folder: %+v, %v", sessions, err)
	}
	path, err := reader.ResolveSession("opencode", dir, "id", id)
	if err != nil || !IsOpencode(path) || SessionID(path) != id {
		t.Fatalf("by id: %q, %v", path, err)
	}
	if byId, err := reader.ResolveSession("", dir, "id", id); err != nil || byId != path {
		t.Fatalf("a session id with no agent: %q, %v", byId, err)
	}
	if _, err := reader.ResolveSession("opencode", dir, "path", "/code/app/session.jsonl"); err == nil {
		t.Error("opencode took a session file path")
	}
}

func TestAnOpencodeSessionReadAtAVersionLeavesLaterMessagesOut(t *testing.T) {
	_, dir, _ := opencodeCheckout(t)
	reader := NewReader(shell.Local{Name: "local"})
	path, _ := reader.ResolveSession("opencode", dir, "", "")
	early := Version{MTime: (opencodeTestBase + 500) * int64(time.Millisecond)}
	turns, err := reader.Turns(path, early)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || len(turns[1].IDs) != 1 {
		t.Fatalf("a message written after the version was read: %+v", turns)
	}
}
