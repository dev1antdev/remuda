// Package transcript reads Claude Code session files (JSONL under
// ~/.claude/projects/<encoded-cwd>/<uuid>.jsonl) into chat turns. Read-only:
// remuda never writes to Claude's files; prompts are typed into the pane.
package transcript

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

// Part is one piece of a turn, a tagged union.
type Part struct {
	Type     string          `json:"type"` // text | thinking | tool
	Text     string          `json:"text,omitempty"`
	ToolName string          `json:"tool_name,omitempty"`
	Input    json.RawMessage `json:"input,omitempty"`
	Output   string          `json:"output,omitempty"`
	IsError  bool            `json:"is_error,omitempty"`
}

// Turn is one chat turn. Adjacent assistant records merge into one turn.
type Turn struct {
	Role  string   `json:"role"` // user | assistant
	Ts    string   `json:"ts,omitempty"`
	IDs   []string `json:"ids,omitempty"` // the records merged into it, for a search jump
	Parts []Part   `json:"parts"`
}

// SessionInfo describes one session file for the picker.
type SessionInfo struct {
	ID      string    `json:"id"`
	Path    string    `json:"-"`
	ModTime time.Time `json:"mod_time"`
	Size    int64     `json:"-"`
	Title   string    `json:"title,omitempty"`
}

// SessionID is the session uuid a transcript path names.
func SessionID(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".jsonl")
}

// EncodePath mirrors Claude Code's project-dir encoding: every character
// outside [A-Za-z0-9] becomes '-'.
func EncodePath(cwd string) string {
	// Claude Code replaces per UTF-16 code unit (a JavaScript regexp), so a
	// non-ASCII letter is one dash, an astral rune two
	var b strings.Builder
	for _, r := range cwd {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			continue
		}
		n := utf16.RuneLen(r)
		if n < 1 {
			n = 1
		}
		b.WriteString(strings.Repeat("-", n))
	}
	return b.String()
}

func projectsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

// Sessions lists the session files for a working directory, newest first.
func Sessions(cwd string) ([]SessionInfo, error) {
	dir := filepath.Join(projectsDir(), EncodePath(cwd))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SessionInfo{}, nil
		}
		return nil, err
	}
	var out []SessionInfo
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, SessionInfo{
			ID:      SessionID(name),
			Path:    filepath.Join(dir, name),
			ModTime: fi.ModTime(),
			Size:    fi.Size(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	for i := range min(5, len(out)) { // titles need a read; only for the sessions the picker shows first
		out[i].Title = sessionTitle(out[i].Path, out[i].Size)
	}
	return out, nil
}

// Titles are cached per path. Lines already scanned never change (the file
// is append-only), so a title once found — or a scan that exhausted its
// window — is final; only a short file that had no title yet is re-read
// once it has grown.
var (
	titleMu    sync.Mutex
	titleCache = map[string]titleEntry{}
)

type titleEntry struct {
	title string
	final bool
	size  int64 // size when last scanned, for non-final entries
}

const titleScanLines = 400 // the ai-title record, when there is one, comes early

// sessionTitle returns the ai-title record if present, else the first words of
// the first real user prompt.
func sessionTitle(path string, size int64) string {
	titleMu.Lock()
	e, ok := titleCache[path]
	titleMu.Unlock()
	if ok && (e.final || e.size == size) {
		return e.title
	}

	title, final := scanTitle(path)

	titleMu.Lock()
	if len(titleCache) > 256 {
		titleCache = map[string]titleEntry{}
	}
	titleCache[path] = titleEntry{title: title, final: final, size: size}
	titleMu.Unlock()
	return title
}

// scanTitle reads up to titleScanLines; final says whether more of the file
// could still change the answer.
func scanTitle(path string) (title string, final bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	first, title := "", ""
	n := 0
	_ = Lines(f, 8*1024*1024, func(line []byte) {
		if title != "" || n >= titleScanLines {
			return
		}
		n++
		var rec record
		if json.Unmarshal(line, &rec) != nil {
			return
		}
		if rec.AiTitle != "" {
			title = rec.AiTitle
			return
		}
		if first == "" && rec.Type == "user" && !rec.IsMeta {
			if t := plainUserText(rec.Message); t != "" {
				first = truncate(t, 80)
			}
		}
	})
	if title != "" {
		return title, true
	}
	return first, n >= titleScanLines
}

// FindSessionByID locates <uuid>.jsonl: first in the project dir encoded from
// cwd, then by scanning every project dir (the UUID guarantees at most one hit).
func FindSessionByID(cwd, id string) (string, error) {
	if cwd != "" {
		p := filepath.Join(projectsDir(), EncodePath(cwd), id+".jsonl")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	entries, err := os.ReadDir(projectsDir())
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(projectsDir(), e.Name(), id+".jsonl")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", os.ErrNotExist
}

// Version is a cheap change token for a transcript: its size and mtime.
type Version struct {
	Size  int64
	MTime int64 // unix nanoseconds locally, seconds from a remote stat
}

// String is the HTTP ETag form.
func (v Version) String() string { return fmt.Sprintf("%d-%d", v.Size, v.MTime) }

// Time is the modification time: nanoseconds from a local stat, seconds
// from a remote one.
func (v Version) Time() time.Time {
	if v.MTime > 1e12 {
		return time.Unix(0, v.MTime)
	}
	return time.Unix(v.MTime, 0)
}

// Lines calls fn for every line of r (without the newline). A line longer
// than max is dropped whole — and the rest of the file still arrives,
// where a bufio.Scanner would have stopped at it in silence.
func Lines(r io.Reader, max int, fn func(line []byte)) error {
	br := bufio.NewReaderSize(r, 64*1024)
	var acc []byte
	skipping := false
	for {
		chunk, err := br.ReadSlice('\n')
		if len(chunk) > 0 && !skipping {
			if len(acc)+len(chunk) > max {
				skipping = true
				acc = acc[:0]
			} else {
				acc = append(acc, chunk...)
			}
		}
		switch {
		case err == nil:
			if !skipping {
				fn(bytes.TrimSuffix(acc, []byte("\n")))
			}
			acc, skipping = acc[:0], false
		case err == bufio.ErrBufferFull:
			continue
		case err == io.EOF:
			if len(acc) > 0 && !skipping {
				fn(acc)
			}
			return nil
		default:
			return err
		}
	}
}

// Stat reads the local file's version.
func Stat(path string) (Version, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return Version{}, err
	}
	return Version{Size: fi.Size(), MTime: fi.ModTime().UnixNano()}, nil
}

// record is the JSONL envelope; message holds Anthropic API content blocks.
type record struct {
	UUID        string          `json:"uuid"`
	Cwd         string          `json:"cwd"`
	Type        string          `json:"type"`
	IsMeta      bool            `json:"isMeta"`
	IsSidechain bool            `json:"isSidechain"`
	IsAPIError  bool            `json:"isApiErrorMessage"`
	Timestamp   string          `json:"timestamp"`
	Message     json.RawMessage `json:"message"`
	AiTitle     string          `json:"aiTitle"`
}

type apiMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"` // tool_result payload
	IsError   bool            `json:"is_error"`
}

const tailWindow = 16 * 1024 * 1024

// Parse reads the tail of a session file into turns. Torn last lines (Claude
// mid-append) are skipped, never fatal.
func Parse(path string) ([]Turn, error) {
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
	return parseStream(f)
}

// ParseBytes parses transcript content already in memory (remote machines
// fetch the file tail over SSH).
func ParseBytes(data []byte) ([]Turn, error) {
	if len(data) > tailWindow {
		data = data[len(data)-tailWindow:]
	}
	return parseStream(bytes.NewReader(data))
}

func parseStream(r io.Reader) ([]Turn, error) {
	var turns []Turn
	// tool_use id -> where its part sits; indexes, not pointers, because a
	// later append to the same turn reallocates Parts
	type partRef struct{ turn, part int }
	pending := map[string]partRef{}

	appendPart := func(role, ts, uuid string, p Part) {
		n := len(turns)
		// Merge consecutive same-role records into one turn; a user turn that
		// only carried tool_results never opens a bubble.
		if n > 0 && turns[n-1].Role == role {
			t := &turns[n-1]
			t.Parts = append(t.Parts, p)
			if uuid != "" && t.IDs[len(t.IDs)-1] != uuid {
				t.IDs = append(t.IDs, uuid)
			}
			return
		}
		turns = append(turns, Turn{Role: role, Ts: ts, IDs: []string{uuid}, Parts: []Part{p}})
	}

	err := Lines(r, 16*1024*1024, func(line []byte) {
		if len(line) == 0 {
			return
		}
		var rec record
		if json.Unmarshal(line, &rec) != nil {
			return
		}
		if rec.IsSidechain || rec.IsMeta || rec.IsAPIError {
			return
		}
		switch rec.Type {
		case "user":
			var msg apiMessage
			if json.Unmarshal(rec.Message, &msg) != nil {
				return
			}
			for _, b := range blocks(msg.Content) {
				switch b.Type {
				case "text", "":
					t := b.Text
					if t == "" && b.Type == "" {
						continue
					}
					if skipUserText(t) {
						continue
					}
					appendPart("user", rec.Timestamp, rec.UUID, Part{Type: "text", Text: userText(t)})
				case "tool_result":
					if ref, ok := pending[b.ToolUseID]; ok {
						p := &turns[ref.turn].Parts[ref.part]
						p.Output = truncate(resultText(b.Content), 16*1024)
						p.IsError = b.IsError
						delete(pending, b.ToolUseID)
					}
				case "image":
					appendPart("user", rec.Timestamp, rec.UUID, Part{Type: "text", Text: "[image]"})
				}
			}
			// string-content user messages
			if s := stringContent(msg.Content); s != "" && !skipUserText(s) {
				appendPart("user", rec.Timestamp, rec.UUID, Part{Type: "text", Text: userText(s)})
			}
		case "assistant":
			var msg apiMessage
			if json.Unmarshal(rec.Message, &msg) != nil {
				return
			}
			for _, b := range blocks(msg.Content) {
				switch b.Type {
				case "text":
					if strings.TrimSpace(b.Text) == "" {
						continue
					}
					appendPart("assistant", rec.Timestamp, rec.UUID, Part{Type: "text", Text: b.Text})
				case "thinking":
					if strings.TrimSpace(b.Thinking) == "" {
						continue
					}
					appendPart("assistant", rec.Timestamp, rec.UUID, Part{Type: "thinking", Text: b.Thinking})
				case "tool_use":
					appendPart("assistant", rec.Timestamp, rec.UUID, Part{
						Type: "tool", ToolName: b.Name,
						Input: truncateJSON(b.Input, 8*1024),
					})
					last := len(turns) - 1
					pending[b.ID] = partRef{turn: last, part: len(turns[last].Parts) - 1}
				}
			}
		}
	})
	return turns, err
}

// ---- search ----

// Match is one record that contains a searched text.
type Match struct {
	UUID    string
	Ts      time.Time
	Role    string // user | assistant
	Cwd     string
	Snippet string // the text around the first occurrence, one line
}

// MatchLine parses one raw record and says whether a user or assistant
// text in it contains q, case-folded — the same texts the chat shows: no
// sidechains, no meta, no injected prompts, no tool output.
func MatchLine(line []byte, q string) (Match, bool) {
	var rec record
	if json.Unmarshal(line, &rec) != nil || rec.IsSidechain || rec.IsMeta || rec.IsAPIError {
		return Match{}, false
	}
	if rec.Type != "user" && rec.Type != "assistant" {
		return Match{}, false
	}
	var msg apiMessage
	if json.Unmarshal(rec.Message, &msg) != nil {
		return Match{}, false
	}
	var texts []string
	if t := stringContent(msg.Content); t != "" {
		texts = append(texts, t)
	}
	for _, b := range blocks(msg.Content) {
		if b.Type == "text" || (b.Type == "" && b.Text != "") {
			texts = append(texts, b.Text)
		}
	}
	needle := strings.ToLower(q)
	for _, t := range texts {
		if rec.Type == "user" {
			if skipUserText(t) {
				continue
			}
			t = userText(t)
		}
		folded := strings.ToLower(t)
		i := strings.Index(folded, needle)
		if i < 0 {
			continue
		}
		if len(folded) != len(t) { // folding changed byte lengths; show the folded text then
			t = folded
		}
		ts, _ := time.Parse(time.RFC3339Nano, rec.Timestamp)
		return Match{UUID: rec.UUID, Ts: ts, Role: rec.Type, Cwd: rec.Cwd, Snippet: snippet(t, i, len(needle))}, true
	}
	return Match{}, false
}

// snippet is the text around [at, at+n): up to 90 runes before and 160
// after, whitespace collapsed, with an ellipsis where it was cut.
func snippet(t string, at, n int) string {
	start, end := at, at+n
	for i := 0; i < 90 && start > 0; i++ {
		_, size := utf8.DecodeLastRuneInString(t[:start])
		start -= size
	}
	for i := 0; i < 160 && end < len(t); i++ {
		_, size := utf8.DecodeRuneInString(t[end:])
		end += size
	}
	out := strings.Join(strings.Fields(t[start:end]), " ")
	if start > 0 {
		out = "…" + out
	}
	if end < len(t) {
		out += "…"
	}
	return out
}

// blocks decodes an array-form content field; nil for string form.
func blocks(raw json.RawMessage) []contentBlock {
	var bs []contentBlock
	if json.Unmarshal(raw, &bs) != nil {
		return nil
	}
	return bs
}

func stringContent(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}

// skipUserText hides runtime-injected envelopes that aren't the user speaking.
func skipUserText(t string) bool {
	tt := strings.TrimSpace(t)
	return strings.HasPrefix(tt, "<command-name>") ||
		strings.HasPrefix(tt, "<local-command-stdout>") ||
		strings.HasPrefix(tt, "<system-reminder>") ||
		strings.HasPrefix(tt, "Caveat: The messages below")
}

var (
	commandNameRe = regexp.MustCompile(`<command-name>([^<]*)</command-name>`)
	commandArgsRe = regexp.MustCompile(`(?s)<command-args>(.*?)</command-args>`)
	commandTagRe  = regexp.MustCompile(`(?s)<command-message>.*?</command-message>\s*`)
)

// userText is what a user record reads as: a slash command, which Claude
// Code logs wrapped in tags, becomes "/name args"; the rest is as typed.
func userText(t string) string {
	if !strings.Contains(t, "<command-message>") {
		return t
	}
	name := ""
	if m := commandNameRe.FindStringSubmatch(t); m != nil {
		name = strings.TrimSpace(m[1])
	}
	args := ""
	if m := commandArgsRe.FindStringSubmatch(t); m != nil {
		args = strings.TrimSpace(m[1])
	}
	if name == "" {
		return strings.TrimSpace(commandTagRe.ReplaceAllString(t, ""))
	}
	return strings.TrimSpace(name + " " + args)
}

func plainUserText(raw json.RawMessage) string {
	var msg apiMessage
	if json.Unmarshal(raw, &msg) != nil {
		return ""
	}
	if s := stringContent(msg.Content); s != "" && !skipUserText(s) {
		return s
	}
	for _, b := range blocks(msg.Content) {
		if b.Type == "text" && !skipUserText(b.Text) {
			return b.Text
		}
	}
	return ""
}

// resultText flattens a tool_result content (string or blocks) to text.
func resultText(raw json.RawMessage) string {
	if s := stringContent(raw); s != "" {
		return s
	}
	var parts []string
	for _, b := range blocks(raw) {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := s[:n]
	// don't split a UTF-8 sequence
	for len(cut) > 0 && cut[len(cut)-1]&0xC0 == 0x80 {
		cut = cut[:len(cut)-1]
	}
	return cut + "…"
}

func truncateJSON(raw json.RawMessage, n int) json.RawMessage {
	if len(raw) <= n {
		return raw
	}
	b, _ := json.Marshal(truncate(string(raw), n))
	return b
}

// ---- continuations ----
//
// When Claude Code compacts a conversation it goes on in a NEW session file
// whose first message points, through parentUuid, at a record of the old
// one — while herdr keeps reporting the session id it saw at start. So a
// resolved file may be stale: Continuation finds the newer file in the same
// project folder that carries on from it, if any.

var parentRe = regexp.MustCompile(`"parentUuid":"([0-9a-f-]{36})"`)

// firstParent is the first parentUuid in the head of a session file.
func firstParent(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	head := make([]byte, 64*1024)
	n, _ := io.ReadFull(f, head)
	m := parentRe.FindSubmatch(head[:n])
	if m == nil {
		return ""
	}
	return string(m[1])
}

// hasRecord says whether the file holds a record with this uuid. The parent
// of a continuation is near the old file's end, so the tail is read first.
func hasRecord(path, uuid string) bool {
	needle := []byte(`"uuid":"` + uuid + `"`)
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return false
	}
	const tail = 4 << 20
	if st.Size() > tail {
		buf := make([]byte, tail)
		if n, _ := f.ReadAt(buf, st.Size()-tail); bytes.Contains(buf[:n], needle) {
			return true
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return false
		}
	}
	found := false
	_ = Lines(f, 16*1024*1024, func(line []byte) {
		if !found && bytes.Contains(line, needle) {
			found = true
		}
	})
	return found
}

// Continuation returns the newer session file in path's folder that
// continues it, or "" when there is none. One step; callers loop.
func Continuation(path string) string {
	st, err := os.Stat(path)
	if err != nil {
		return ""
	}
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	type cand struct {
		path string
		mod  time.Time
	}
	var cands []cand
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") || filepath.Join(dir, e.Name()) == path {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.ModTime().After(st.ModTime()) {
			continue
		}
		cands = append(cands, cand{filepath.Join(dir, e.Name()), info.ModTime()})
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].mod.Before(cands[j].mod) })
	for _, c := range cands {
		if p := firstParent(c.path); p != "" && hasRecord(path, p) {
			return c.path
		}
	}
	return ""
}

// Predecessor is the older file in path's folder that path continues —
// the one holding the record path's first parentUuid names — or "".
func Predecessor(path string) string {
	parent := firstParent(path)
	if parent == "" {
		return ""
	}
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") || p == path {
			continue
		}
		if hasRecord(p, parent) {
			return p
		}
	}
	return ""
}

// PredecessorScript is Predecessor as a POSIX shell one-liner, for a
// remote machine: prints the older file, or nothing.
func PredecessorScript(quotedPath string) string {
	return `new=` + quotedPath + `; dir=$(dirname "$new"); ` +
		`p=$(head -c 65536 "$new" | grep -o '"parentUuid":"[0-9a-f-]*"' | head -1 | cut -d'"' -f4); [ -n "$p" ] || exit 0; ` +
		`for f in "$dir"/*.jsonl; do [ "$f" = "$new" ] && continue; if grep -q "\"uuid\":\"$p\"" "$f" 2>/dev/null; then printf %s "$f"; exit 0; fi; done`
}

// ContinuationScript is the same search as a POSIX shell one-liner, for a
// remote machine: prints the continuing file, or nothing.
func ContinuationScript(quotedPath string) string {
	return `old=` + quotedPath + `; dir=$(dirname "$old"); for f in $(ls -t "$dir"/*.jsonl 2>/dev/null); do ` +
		`[ "$f" = "$old" ] && break; [ "$f" -nt "$old" ] || continue; ` +
		`p=$(head -c 65536 "$f" | grep -o '"parentUuid":"[0-9a-f-]*"' | head -1 | cut -d'"' -f4); [ -n "$p" ] || continue; ` +
		`if grep -q "\"uuid\":\"$p\"" "$old"; then printf %s "$f"; exit 0; fi; done`
}
