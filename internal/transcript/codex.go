package transcript

// Codex's rollout: ~/.codex/sessions/YYYY/MM/DD/rollout-<time>-<thread id>.jsonl, one record a line,
// {"timestamp","type","payload"}: session_meta (the cwd), turn_context (the model), response_item (the API's
// own items: message, reasoning, function_call and its output, custom_tool_call and its output, local_shell_call,
// web_search_call), event_msg (the TUI's events, which repeat the items), token_usage_record (one API response's
// tokens) and compacted (the history replaced in place: the file goes on). The thread id in the file name is what
// `codex resume` takes and what herdr reports as the session.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/text"
)

type codexRecord struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Ordinal   int             `json:"ordinal"`
	Payload   json.RawMessage `json:"payload"`
}

type codexPayload struct {
	Type       string          `json:"type"`
	ID         string          `json:"id"`
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	Summary    json.RawMessage `json:"summary"` // a reasoning item's texts; a string on a turn_context
	Name       string          `json:"name"`
	Arguments  string          `json:"arguments"`
	Input      string          `json:"input"`
	CallID     string          `json:"call_id"`
	Output     json.RawMessage `json:"output"`
	Status     string          `json:"status"`
	Action     codexAction     `json:"action"`
	Cwd        string          `json:"cwd"`
	Model      string          `json:"model"`
	ResponseID string          `json:"response_id"`
	Usage      *codexUsage     `json:"usage"`
	Window     int             `json:"model_context_window"`
	Info       *struct {
		Window int `json:"model_context_window"`
	} `json:"info"`
}

type codexText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type codexAction struct {
	Command []string `json:"command"`
	Query   string   `json:"query"`
}

type codexUsage struct {
	Input      int64 `json:"input_tokens"`
	Cached     int64 `json:"cached_input_tokens"`
	CacheWrite int64 `json:"cache_write_input_tokens"`
	Output     int64 `json:"output_tokens"`
}

// codexRecordID names a record for the page's jumps and pins: the item's own id, else its place in the file.
func codexRecordID(rec codexRecord, p codexPayload, line int) string {
	switch {
	case p.ID != "":
		return p.ID
	case rec.Ordinal > 0:
		return fmt.Sprintf("o%d", rec.Ordinal)
	}
	return fmt.Sprintf("l%d", line)
}

// codexHarness are the envelopes Codex puts in the user's slot that the person never typed.
var codexHarness = []string{"<environment_context>", "<user_instructions>", "<permissions instructions>", "<collaboration_mode>",
	"<apps_instructions>", "<plugins_instructions>", "<app_instructions>", "<skills", "<host_skills", "<turn_aborted>", "<user_shell_command>", "<memory"}

func codexHarnessText(t string) bool {
	tt := strings.TrimSpace(t)
	for _, tag := range codexHarness {
		if strings.HasPrefix(tt, tag) {
			return true
		}
	}
	return false
}

// codexTexts are the texts of a message's content blocks (input_text, output_text, text), or the string it is.
func codexTexts(raw json.RawMessage) []string {
	if s := stringContent(raw); s != "" {
		return []string{s}
	}
	var blocks []codexText
	if json.Unmarshal(raw, &blocks) != nil {
		return nil
	}
	var out []string
	for _, b := range blocks {
		if b.Text != "" {
			out = append(out, b.Text)
		}
	}
	return out
}

// codexOutput flattens a tool's output and says whether the command behind it failed: the exec tools write
// "Process exited with code N", the JS exec tool a JSON chunk per command with its exit_code and output.
func codexOutput(raw json.RawMessage) (string, bool) {
	failed := false
	var parts []string
	for _, t := range codexTexts(raw) {
		part, bad := codexChunk(t)
		failed = failed || bad
		parts = append(parts, part)
	}
	return strings.Join(parts, "\n"), failed
}

var exitedRe = regexp.MustCompile(`(?m)^Process exited with code (\d+)`)

func codexChunk(t string) (string, bool) {
	trimmed := strings.TrimSpace(t)
	if strings.HasPrefix(trimmed, "{") {
		var chunk struct {
			Output   string `json:"output"`
			ExitCode *int   `json:"exit_code"`
		}
		if json.Unmarshal([]byte(trimmed), &chunk) == nil && (chunk.Output != "" || chunk.ExitCode != nil) {
			if chunk.ExitCode != nil && *chunk.ExitCode != 0 {
				return fmt.Sprintf("exit code %d\n%s", *chunk.ExitCode, chunk.Output), true
			}
			return chunk.Output, false
		}
	}
	if m := exitedRe.FindStringSubmatch(t); m != nil && m[1] != "0" {
		return t, true
	}
	return t, false
}

// codexInput is a function call's arguments as JSON, the text itself when they are not JSON.
func codexInput(arguments string) json.RawMessage {
	if json.Valid([]byte(arguments)) && strings.HasPrefix(strings.TrimSpace(arguments), "{") {
		return truncateJSON(json.RawMessage(arguments), 8*1024)
	}
	b, _ := json.Marshal(map[string]string{"input": arguments})
	return truncateJSON(b, 8*1024)
}

func parseCodexFile(path string) ([]Turn, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil && fi.Size() > tailWindow {
		if _, err := f.Seek(fi.Size()-tailWindow, 0); err != nil {
			return nil, err
		}
	}
	return parseCodex(f)
}

// parseCodex reads a rollout into turns: the person's prompts, the assistant's texts, its reasoning summaries and its
// tool calls with their outputs; what each turn spent from the token records, the model from the turn's context.
func parseCodex(r io.Reader) ([]Turn, error) {
	var turns []Turn
	type partRef struct{ turn, part int }
	pending := map[string]partRef{}
	model, window, cwd := "", 0, ""
	line := 0

	appendPart := func(role, ts, id string, p Part) {
		n := len(turns)
		if n > 0 && turns[n-1].Role == role {
			t := &turns[n-1]
			t.Parts = append(t.Parts, p)
			t.ended = ts
			if id != "" && t.IDs[len(t.IDs)-1] != id {
				t.IDs = append(t.IDs, id)
			}
			return
		}
		turns = append(turns, Turn{Role: role, Ts: ts, IDs: []string{id}, Parts: []Part{p}, ended: ts})
	}
	call := func(ts, id string, p Part) {
		appendPart("assistant", ts, id, p)
		last := len(turns) - 1
		pending[p.ToolID] = partRef{turn: last, part: len(turns[last].Parts) - 1}
	}

	err := Lines(r, 16*1024*1024, func(raw []byte) {
		line++
		if len(raw) == 0 {
			return
		}
		var rec codexRecord
		if json.Unmarshal(raw, &rec) != nil {
			return
		}
		var p codexPayload
		if json.Unmarshal(rec.Payload, &p) != nil {
			return
		}
		id := codexRecordID(rec, p, line)
		switch rec.Type {
		case "session_meta":
			if cwd == "" {
				cwd = p.Cwd
			}
		case "turn_context":
			if p.Model != "" {
				model = p.Model
			}
		case "event_msg":
			switch {
			case p.Info != nil && p.Info.Window > 0:
				window = p.Info.Window
			case p.Window > 0:
				window = p.Window
			}
		case "token_usage_record":
			if n := len(turns); p.Usage != nil && n > 0 && turns[n-1].Role == "assistant" {
				t := &turns[n-1]
				t.ended = rec.Timestamp
				t.Responses = append(t.Responses, codexUsageRecord(p, rec.Timestamp, model, window, cwd))
			}
		case "response_item":
			switch p.Type {
			case "message":
				switch p.Role {
				case "user":
					for _, t := range codexTexts(p.Content) {
						if !codexHarnessText(t) {
							appendPart("user", rec.Timestamp, id, Part{Type: "text", Text: t})
						}
					}
				case "assistant":
					for _, t := range codexTexts(p.Content) {
						if strings.TrimSpace(t) != "" {
							appendPart("assistant", rec.Timestamp, id, Part{Type: "text", Text: t})
						}
					}
				}
			case "reasoning":
				var texts []string
				for _, s := range codexTexts(p.Summary) {
					if strings.TrimSpace(s) != "" {
						texts = append(texts, s)
					}
				}
				if len(texts) > 0 {
					appendPart("assistant", rec.Timestamp, id, Part{Type: "thinking", Text: strings.Join(texts, "\n\n")})
				}
			case "function_call":
				call(rec.Timestamp, id, Part{Type: "tool", ToolName: p.Name, ToolID: p.CallID, At: rec.Timestamp, Input: codexInput(p.Arguments)})
			case "custom_tool_call":
				input, _ := json.Marshal(map[string]string{"input": p.Input})
				call(rec.Timestamp, id, Part{Type: "tool", ToolName: p.Name, ToolID: p.CallID, At: rec.Timestamp, Input: truncateJSON(input, 8*1024)})
			case "local_shell_call":
				input, _ := json.Marshal(map[string]string{"command": strings.Join(p.Action.Command, " ")})
				call(rec.Timestamp, id, Part{Type: "tool", ToolName: "shell", ToolID: p.CallID, At: rec.Timestamp, Input: input})
			case "web_search_call":
				input, _ := json.Marshal(map[string]string{"query": p.Action.Query})
				appendPart("assistant", rec.Timestamp, id, Part{Type: "tool", ToolName: "web_search", ToolID: p.ID, At: rec.Timestamp, Input: input, Output: p.Status})
			case "function_call_output", "custom_tool_call_output", "local_shell_call_output":
				ref, ok := pending[p.CallID]
				if !ok {
					return
				}
				part := &turns[ref.turn].Parts[ref.part]
				output, failed := codexOutput(p.Output)
				part.Output = truncate(output, 16*1024)
				part.IsError = failed
				part.Took = secondsBetween(part.At, rec.Timestamp)
				turns[ref.turn].ended = rec.Timestamp
				delete(pending, p.CallID)
			}
		}
	})
	settleSpending(turns)
	return turns, err
}

func codexUsageRecord(p codexPayload, ts, model string, window int, cwd string) UsageRecord {
	at, _ := time.Parse(time.RFC3339Nano, ts)
	u := p.Usage
	return UsageRecord{
		MessageID: p.ResponseID, Model: model, At: at, Cwd: cwd, Window: window,
		Input: max(u.Input-u.Cached, 0), Output: u.Output, CacheRead: u.Cached, CacheWrite5m: u.CacheWrite,
	}
}

// codexUsageScanner reads a rollout's token records a line at a time, carrying the model, the window and the cwd
// the earlier records named.
type codexUsageScanner struct {
	model  string
	window int
	cwd    string
}

var codexUsageLines = [][]byte{[]byte(`"session_meta"`), []byte(`"turn_context"`), []byte(`"token_usage_record"`), []byte(`"token_count"`), []byte(`"task_started"`)}

func (s *codexUsageScanner) Scan(line []byte) (UsageRecord, bool) {
	if !anyOf(line, codexUsageLines) {
		return UsageRecord{}, false
	}
	var rec codexRecord
	if json.Unmarshal(line, &rec) != nil {
		return UsageRecord{}, false
	}
	var p codexPayload
	if json.Unmarshal(rec.Payload, &p) != nil {
		return UsageRecord{}, false
	}
	switch rec.Type {
	case "session_meta":
		if s.cwd == "" {
			s.cwd = p.Cwd
		}
	case "turn_context":
		if p.Model != "" {
			s.model = p.Model
		}
	case "event_msg":
		switch {
		case p.Info != nil && p.Info.Window > 0:
			s.window = p.Info.Window
		case p.Window > 0:
			s.window = p.Window
		}
	case "token_usage_record":
		if p.Usage != nil {
			return codexUsageRecord(p, rec.Timestamp, s.model, s.window, s.cwd), true
		}
	}
	return UsageRecord{}, false
}

func anyOf(line []byte, needles [][]byte) bool {
	for _, n := range needles {
		if bytes.Contains(line, n) {
			return true
		}
	}
	return false
}

// MatchCodexLine is MatchLine for a rollout record: the person's and the assistant's texts, never a tool's.
func MatchCodexLine(line []byte, q string) (Match, bool) {
	if !bytes.Contains(line, []byte(`"message"`)) {
		return Match{}, false
	}
	var rec codexRecord
	if json.Unmarshal(line, &rec) != nil || rec.Type != "response_item" {
		return Match{}, false
	}
	var p codexPayload
	if json.Unmarshal(rec.Payload, &p) != nil || p.Type != "message" || (p.Role != "user" && p.Role != "assistant") {
		return Match{}, false
	}
	needle := strings.ToLower(q)
	for _, t := range codexTexts(p.Content) {
		if p.Role == "user" && codexHarnessText(t) {
			continue
		}
		folded := strings.ToLower(t)
		i := strings.Index(folded, needle)
		if i < 0 {
			continue
		}
		if len(folded) != len(t) {
			t = folded
		}
		ts, _ := time.Parse(time.RFC3339Nano, rec.Timestamp)
		return Match{UUID: codexRecordID(rec, p, 0), Ts: ts, Role: p.Role, Snippet: snippet(t, i, len(needle))}, true
	}
	return Match{}, false
}

// ---- the files ----

// CodexSessionID is the thread id a rollout's name carries: rollout-<time>-<id>.jsonl.
func CodexSessionID(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	if i := strings.LastIndex(name, "-"); i >= 0 && len(name)-i-1 == 36 {
		return name[i+1:]
	}
	parts := strings.Split(name, "-")
	if len(parts) < 5 {
		return ""
	}
	return strings.Join(parts[len(parts)-5:], "-")
}

// codexHead is what a rollout's first record says: the folder it was started in, kept per file since it never changes.
var (
	codexHeadMu    sync.Mutex
	codexHeadCache = map[string]string{}
)

// CodexCwd is the folder a rollout was recorded in, "" when the file does not say.
func CodexCwd(path string) string {
	codexHeadMu.Lock()
	cwd, ok := codexHeadCache[path]
	codexHeadMu.Unlock()
	if ok {
		return cwd
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	head, _ := bufio.NewReaderSize(f, 64*1024).ReadBytes('\n')
	var rec codexRecord
	var p codexPayload
	if json.Unmarshal(bytes.TrimSpace(head), &rec) == nil && rec.Type == "session_meta" && json.Unmarshal(rec.Payload, &p) == nil {
		cwd = p.Cwd
	}
	codexHeadMu.Lock()
	if len(codexHeadCache) > 4096 {
		codexHeadCache = map[string]string{}
	}
	codexHeadCache[path] = cwd
	codexHeadMu.Unlock()
	return cwd
}

// CodexFiles lists every rollout under root, newest first.
func CodexFiles(root string) []SessionInfo {
	var out []SessionInfo
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") || !strings.HasPrefix(d.Name(), "rollout-") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		out = append(out, SessionInfo{ID: CodexSessionID(p), Path: p, ModTime: info.ModTime(), Size: info.Size(), Agent: "codex"})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out
}

const codexSessionsLookAt = 300 // the newest rollouts whose folder is read for a listing by folder

// codexSessions lists the rollouts recorded in cwd, newest first, titled from Codex's own index.
func codexSessions(home, cwd string) []SessionInfo {
	var out []SessionInfo
	for i, s := range CodexFiles(SessionRoots(home)[Codex]) {
		if i >= codexSessionsLookAt {
			break
		}
		if CodexCwd(s.Path) == cwd {
			out = append(out, s)
		}
	}
	names := codexThreadNames(home)
	for i := range out {
		if name := names[out[i].ID]; name != "" {
			out[i].Title = name
		} else if i < 5 {
			out[i].Title = codexFirstPrompt(out[i].Path)
		}
	}
	return out
}

// codexThreadNames is Codex's own session index: the thread id and the name it gave the thread.
func codexThreadNames(home string) map[string]string {
	names := map[string]string{}
	f, err := os.Open(filepath.Join(home, ".codex", "session_index.jsonl"))
	if err != nil {
		return names
	}
	defer f.Close()
	_ = Lines(f, 64*1024, func(line []byte) {
		var entry struct {
			ID   string `json:"id"`
			Name string `json:"thread_name"`
		}
		if json.Unmarshal(line, &entry) == nil && entry.ID != "" && entry.Name != "" {
			names[entry.ID] = entry.Name
		}
	})
	return names
}

func codexFirstPrompt(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	title, n := "", 0
	_ = Lines(f, 8*1024*1024, func(line []byte) {
		if title != "" || n >= titleScanLines {
			return
		}
		n++
		var rec codexRecord
		var p codexPayload
		if json.Unmarshal(line, &rec) != nil || rec.Type != "response_item" || json.Unmarshal(rec.Payload, &p) != nil || p.Type != "message" || p.Role != "user" {
			return
		}
		for _, t := range codexTexts(p.Content) {
			if !codexHarnessText(t) {
				title = truncate(t, 80)
				return
			}
		}
	})
	return title
}

// findCodexSession is the rollout of a thread id under home, wherever in the date tree it sits.
func findCodexSession(home, id string) (string, error) {
	if !text.SessionID(id) {
		return "", os.ErrNotExist
	}
	hits, _ := filepath.Glob(filepath.Join(SessionRoots(home)[Codex], "*", "*", "*", "rollout-*-"+id+".jsonl"))
	if len(hits) == 0 {
		return "", os.ErrNotExist
	}
	sort.Strings(hits)
	return hits[len(hits)-1], nil
}

// codexFindScript is findCodexSession as a shell line for a remote machine: prints the path, or nothing.
func codexFindScript(quotedHome, id string) string {
	return `ls ` + quotedHome + `/.codex/sessions/*/*/*/rollout-*-` + id + `.jsonl 2>/dev/null | tail -1`
}
