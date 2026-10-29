package transcript

// Gemini CLI's chats: ~/.gemini/tmp/<sha256 of the project root>/chats/session-<YYYY-MM-DDTHH-MM>-<id's first 8>.jsonl,
// the first line the session's metadata (sessionId, projectHash, startTime), then one message a line —
// {"id","timestamp","type":"user"|"gemini"|"info"|"error"|"warning","content"} with, on a gemini one, "thoughts",
// "toolCalls", "tokens" and "model" — and the amendments: {"$patch":…} (a tool's result that landed later, messages
// removed or reordered), {"$rewindTo":id} (that message and the ones after it undone), {"$set":…} (metadata).
// Older versions wrote the whole record as one JSON file, session-….json, with the messages inside. The session
// id is what `gemini --resume` takes. Written from the CLI's own chatRecordingTypes.ts, not seen live here.

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/text"
)

type geminiMessage struct {
	ID        string          `json:"id"`
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Content   json.RawMessage `json:"content"`
	Model     string          `json:"model"`
	Thoughts  []struct {
		Subject     string `json:"subject"`
		Description string `json:"description"`
	} `json:"thoughts"`
	ToolCalls []geminiToolCall `json:"toolCalls"`
	Tokens    *struct {
		Input    int64 `json:"input"`
		Output   int64 `json:"output"`
		Cached   int64 `json:"cached"`
		Thoughts int64 `json:"thoughts"`
	} `json:"tokens"`
}

type geminiToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Args      json.RawMessage `json:"args"`
	Result    json.RawMessage `json:"result"`
	Status    string          `json:"status"`
	Timestamp string          `json:"timestamp"`
}

type geminiPatch struct {
	ID        string           `json:"id"`
	Content   json.RawMessage  `json:"content"`
	ToolCalls []geminiToolCall `json:"toolCalls"`
	Updates   []geminiPatch    `json:"updates"`
	RemoveIDs []string         `json:"removeIds"`
	OrderIDs  []string         `json:"orderIds"`
}

// geminiLine is any line of the file: a message, an amendment, or the metadata.
type geminiLine struct {
	geminiMessage
	Patch     *geminiPatch    `json:"$patch"`
	RewindTo  string          `json:"$rewindTo"`
	Set       json.RawMessage `json:"$set"`
	SessionID string          `json:"sessionId"`
	Messages  []geminiMessage `json:"messages"`
}

type geminiPart struct {
	Text         string `json:"text"`
	FunctionCall *struct {
		ID   string          `json:"id"`
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
	} `json:"functionCall"`
	FunctionResponse *struct {
		Response json.RawMessage `json:"response"`
	} `json:"functionResponse"`
}

func geminiParts(raw json.RawMessage) []geminiPart {
	if s := stringContent(raw); s != "" {
		return []geminiPart{{Text: s}}
	}
	var parts []geminiPart
	if json.Unmarshal(raw, &parts) != nil {
		return nil
	}
	return parts
}

func parseGeminiFile(path string) ([]Turn, error) {
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
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	return parseGemini(data)
}

// geminiMessages reads a file's messages as they stand after its amendments: the legacy whole-record form first,
// else a line at a time.
func geminiMessages(data []byte) []geminiMessage {
	var whole geminiLine
	if json.Unmarshal(data, &whole) == nil && len(whole.Messages) > 0 {
		return whole.Messages
	}
	var msgs []geminiMessage
	index := func(id string) int {
		for i := range msgs {
			if msgs[i].ID == id {
				return i
			}
		}
		return -1
	}
	apply := func(p geminiPatch) {
		i := index(p.ID)
		if i < 0 {
			return
		}
		if len(p.Content) > 0 {
			msgs[i].Content = p.Content
		}
		for _, tc := range p.ToolCalls {
			for j := range msgs[i].ToolCalls {
				if msgs[i].ToolCalls[j].ID == tc.ID {
					if len(tc.Result) > 0 {
						msgs[i].ToolCalls[j].Result = tc.Result
					}
					if tc.Status != "" {
						msgs[i].ToolCalls[j].Status = tc.Status
					}
				}
			}
		}
	}
	_ = Lines(bytes.NewReader(data), 16*1024*1024, func(line []byte) {
		if len(bytes.TrimSpace(line)) == 0 {
			return
		}
		var l geminiLine
		if json.Unmarshal(line, &l) != nil {
			return
		}
		switch {
		case l.RewindTo != "":
			if i := index(l.RewindTo); i >= 0 {
				msgs = msgs[:i]
			}
		case l.Patch != nil:
			if l.Patch.ID != "" {
				apply(*l.Patch)
			}
			for _, u := range l.Patch.Updates {
				apply(u)
			}
			for _, id := range l.Patch.RemoveIDs {
				if i := index(id); i >= 0 {
					msgs = append(msgs[:i], msgs[i+1:]...)
				}
			}
			if len(l.Patch.OrderIDs) > 0 {
				ordered := make([]geminiMessage, 0, len(msgs))
				for _, id := range l.Patch.OrderIDs {
					if i := index(id); i >= 0 {
						ordered = append(ordered, msgs[i])
					}
				}
				if len(ordered) == len(msgs) {
					msgs = ordered
				}
			}
		case len(l.Set) > 0, l.SessionID != "" && l.ID == "":
			// metadata
		case l.ID != "":
			msgs = append(msgs, l.geminiMessage)
		}
	})
	return msgs
}

// parseGemini turns a chat's messages into turns: the person's texts, the model's thoughts, tool calls (in the order
// its content names them) and texts, and what each answer spent.
func parseGemini(data []byte) ([]Turn, error) {
	var turns []Turn
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
	for _, m := range geminiMessages(data) {
		switch m.Type {
		case "user":
			for _, p := range geminiParts(m.Content) {
				if strings.TrimSpace(p.Text) != "" {
					appendPart("user", m.Timestamp, m.ID, Part{Type: "text", Text: p.Text})
				}
			}
		case "gemini":
			for _, th := range m.Thoughts {
				if t := strings.TrimSpace(strings.TrimSpace(th.Subject) + "\n\n" + strings.TrimSpace(th.Description)); t != "" {
					appendPart("assistant", m.Timestamp, m.ID, Part{Type: "thinking", Text: t})
				}
			}
			placed := map[string]bool{}
			for _, p := range geminiParts(m.Content) {
				switch {
				case p.FunctionCall != nil:
					call := geminiToolCall{ID: p.FunctionCall.ID, Name: p.FunctionCall.Name, Args: p.FunctionCall.Args, Timestamp: m.Timestamp}
					for _, tc := range m.ToolCalls {
						if tc.ID == call.ID || (call.ID == "" && tc.Name == call.Name && !placed[tc.ID]) {
							call = tc
							break
						}
					}
					placed[call.ID] = true
					appendPart("assistant", m.Timestamp, m.ID, geminiToolPart(call, m.Timestamp))
				case strings.TrimSpace(p.Text) != "":
					appendPart("assistant", m.Timestamp, m.ID, Part{Type: "text", Text: p.Text})
				}
			}
			for _, tc := range m.ToolCalls {
				if !placed[tc.ID] {
					appendPart("assistant", m.Timestamp, m.ID, geminiToolPart(tc, m.Timestamp))
				}
			}
			if n := len(turns); m.Tokens != nil && n > 0 && turns[n-1].Role == "assistant" {
				turns[n-1].Responses = append(turns[n-1].Responses, geminiUsageRecord(m))
			}
		}
	}
	settleSpending(turns)
	return turns, nil
}

func geminiToolPart(call geminiToolCall, ts string) Part {
	at := call.Timestamp
	if at == "" {
		at = ts
	}
	part := Part{Type: "tool", ToolName: call.Name, ToolID: call.ID, At: at, Input: truncateJSON(call.Args, 8*1024)}
	if output, ok := geminiResult(call.Result); ok {
		part.Output = truncate(output, 16*1024)
	}
	part.IsError = call.Status == "error"
	return part
}

// geminiResult flattens a tool's result parts to text: a functionResponse's output or error, else its JSON.
func geminiResult(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	if s := stringContent(raw); s != "" {
		return s, true
	}
	var texts []string
	for _, p := range geminiParts(raw) {
		switch {
		case p.FunctionResponse != nil:
			var resp struct {
				Output string `json:"output"`
				Error  string `json:"error"`
			}
			if json.Unmarshal(p.FunctionResponse.Response, &resp) == nil && (resp.Output != "" || resp.Error != "") {
				texts = append(texts, resp.Output+resp.Error)
			} else {
				texts = append(texts, string(p.FunctionResponse.Response))
			}
		case p.Text != "":
			texts = append(texts, p.Text)
		}
	}
	return strings.Join(texts, "\n"), true
}

func geminiUsageRecord(m geminiMessage) UsageRecord {
	at, _ := time.Parse(time.RFC3339Nano, m.Timestamp)
	t := m.Tokens
	return UsageRecord{MessageID: m.ID, Model: m.Model, At: at, Input: max(t.Input-t.Cached, 0), CacheRead: t.Cached, Output: t.Output + t.Thoughts}
}

// geminiUsageScanner reads a chat's token counts a line at a time (the legacy one-object file has no lines to read).
type geminiUsageScanner struct{}

func (geminiUsageScanner) Scan(line []byte) (UsageRecord, bool) {
	if !bytes.Contains(line, []byte(`"tokens"`)) {
		return UsageRecord{}, false
	}
	var m geminiMessage
	if json.Unmarshal(line, &m) != nil || m.Type != "gemini" || m.Tokens == nil || m.ID == "" {
		return UsageRecord{}, false
	}
	return geminiUsageRecord(m), true
}

// ---- the files ----

// GeminiProjectHash is the folder's name under ~/.gemini/tmp: the sha256 of the project root, as the CLI computes it.
func GeminiProjectHash(cwd string) string {
	sum := sha256.Sum256([]byte(cwd))
	return hex.EncodeToString(sum[:])
}

func geminiChatsDir(home, cwd string) string {
	return filepath.Join(SessionRoots(home)[Gemini], GeminiProjectHash(cwd), "chats")
}

var (
	geminiHeadMu    sync.Mutex
	geminiHeadCache = map[string]string{}
)

// GeminiSessionID is the id a chat file's metadata names, "" when the file does not say.
func GeminiSessionID(path string) string {
	geminiHeadMu.Lock()
	id, ok := geminiHeadCache[path]
	geminiHeadMu.Unlock()
	if ok {
		return id
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	head := make([]byte, 64*1024)
	n, _ := io.ReadFull(bufio.NewReader(f), head)
	var meta geminiLine
	if json.Unmarshal(bytes.TrimSpace(head[:n]), &meta) != nil {
		if line, _, ok := bytes.Cut(head[:n], []byte("\n")); ok {
			_ = json.Unmarshal(line, &meta)
		}
	}
	id = meta.SessionID
	geminiHeadMu.Lock()
	if len(geminiHeadCache) > 4096 {
		geminiHeadCache = map[string]string{}
	}
	geminiHeadCache[path] = id
	geminiHeadMu.Unlock()
	return id
}

func isGeminiChat(name string) bool {
	return strings.HasPrefix(name, "session-") && (strings.HasSuffix(name, ".jsonl") || strings.HasSuffix(name, ".json"))
}

// geminiSessions lists the chats recorded in cwd, newest first.
func geminiSessions(home, cwd string) []SessionInfo {
	entries, err := os.ReadDir(geminiChatsDir(home, cwd))
	if err != nil {
		return nil
	}
	var out []SessionInfo
	for _, e := range entries {
		if e.IsDir() || !isGeminiChat(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(geminiChatsDir(home, cwd), e.Name())
		id := GeminiSessionID(path)
		if id == "" {
			continue
		}
		out = append(out, SessionInfo{ID: id, Path: path, ModTime: info.ModTime(), Size: info.Size(), Agent: "gemini"})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	for i := range min(5, len(out)) {
		out[i].Title = geminiFirstPrompt(out[i].Path)
	}
	return out
}

func geminiFirstPrompt(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 8*1024*1024 {
		return ""
	}
	for _, m := range geminiMessages(data) {
		if m.Type != "user" {
			continue
		}
		for _, p := range geminiParts(m.Content) {
			if strings.TrimSpace(p.Text) != "" {
				return truncate(p.Text, 80)
			}
		}
	}
	return ""
}

// findGeminiSession is the chat file of a session id: the folder's own chats first, then every project's.
func findGeminiSession(home, cwd, id string) (string, error) {
	if !text.SessionID(id) || len(id) < 8 {
		return "", os.ErrNotExist
	}
	short := id[:8]
	var globs []string
	if cwd != "" {
		globs = append(globs, filepath.Join(geminiChatsDir(home, cwd), "session-*-"+short+".json*"))
	}
	globs = append(globs, filepath.Join(SessionRoots(home)[Gemini], "*", "chats", "session-*-"+short+".json*"))
	for _, g := range globs {
		hits, _ := filepath.Glob(g)
		for _, hit := range hits {
			if isGeminiChat(filepath.Base(hit)) && GeminiSessionID(hit) == id {
				return hit, nil
			}
		}
	}
	return "", os.ErrNotExist
}

// geminiFindScript is findGeminiSession as a shell line for a remote machine: prints the path, or nothing.
func geminiFindScript(quotedHome, id string) string {
	short := id
	if len(short) > 8 {
		short = short[:8]
	}
	return `for f in $(ls -t ` + quotedHome + `/.gemini/tmp/*/chats/session-*-` + short + `.json* 2>/dev/null); do ` +
		`if head -c 65536 "$f" | grep -q '"sessionId": *"` + id + `"'; then printf %s "$f"; break; fi; done`
}
