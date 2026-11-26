// opencode's conversations: unlike the other agents it keeps no session file, but a row in its own SQLite database
// (opencode.db, in XDG_DATA_HOME). A session is named by a pseudo-path, opencodePath, and read through sqlite3 on a
// remote machine or the driver here; its turns, tokens and tool calls come out as every other agent's do.

package transcript

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // the driver, pure Go: cross-compiles with CGO_ENABLED=0

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// opencodeSep joins the folder and the session id in a pseudo-path: neither may hold it.
const opencodeSep = "\x1f"

const opencodePrefix = "opencode" + opencodeSep

// IsOpencode says whether a path names an opencode conversation rather than a file.
func IsOpencode(path string) bool { return strings.HasPrefix(path, opencodePrefix) }

// opencodePath is the pseudo-path a session is read at: its folder, for the version and the list, and its id.
func opencodePath(dir, id string) string { return opencodePrefix + dir + opencodeSep + id }

// opencodePathParts is the folder and id a pseudo-path names.
func opencodePathParts(path string) (dir, id string, ok bool) {
	rest, found := strings.CutPrefix(path, opencodePrefix)
	if !found {
		return "", "", false
	}
	dir, id, found = strings.Cut(rest, opencodeSep)
	if !found || dir == "" || id == "" {
		return "", "", false
	}
	return dir, id, true
}

// OpencodeUsage is a live opencode session's meter: its last response's context and the session's sums, as its own
// database keeps them.
type OpencodeUsage struct {
	Model      string
	Context    int64 // input, cache read and cache write of the last response
	Input      int64
	Output     int64
	CacheRead  int64
	CacheWrite int64
	Cost       float64
	Turns      int
	Updated    int64 // unix ms of the session's last write
}

// opencodeDB is the database's path on the machine: XDG_DATA_HOME, else ~/.local/share.
func (r *Reader) opencodeDB() (string, error) {
	if r.host.Local() {
		if data := os.Getenv("XDG_DATA_HOME"); data != "" {
			return filepath.Join(data, "opencode", "opencode.db"), nil
		}
		return filepath.Join(r.host.Home(), ".local", "share", "opencode", "opencode.db"), nil
	}
	out, err := shell.Text(r.host, `printf '%s\n' "${XDG_DATA_HOME:-$HOME/.local/share}/opencode/opencode.db"`, 10*time.Second)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// opencodeRows runs a read-only query on the machine: the driver here, sqlite3 there. Values come back as strings,
// columns by name.
func (r *Reader) opencodeRows(query string, args ...any) ([]map[string]string, error) {
	db, err := r.opencodeDB()
	if err != nil {
		return nil, err
	}
	if r.host.Local() {
		return r.opencodeLocalRows(db, query, args...)
	}
	// values are part of the query there: quote them as SQLite does, a number as it is
	for _, arg := range args {
		query = strings.Replace(query, "?", sqlValue(arg), 1)
	}
	out, err := shell.Text(r.host, `sqlite3 -readonly -json `+script.Quote(db)+` `+script.Quote(query), 30*time.Second)
	if err != nil {
		if strings.Contains(err.Error(), "sqlite3") && strings.Contains(err.Error(), "not found") {
			return nil, fmt.Errorf("opencode's sessions cannot be read on %s: sqlite3 is not installed there", r.host.Label())
		}
		return nil, err
	}
	var rows []map[string]any
	if strings.TrimSpace(out) == "" || strings.TrimSpace(out) == "[]" {
		return nil, nil
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		return nil, fmt.Errorf("opencode's database answered something else: %w", err)
	}
	got := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		values := make(map[string]string, len(row))
		for k, v := range row {
			switch x := v.(type) {
			case nil:
				values[k] = ""
			case string:
				values[k] = x
			case float64:
				values[k] = strconv.FormatFloat(x, 'f', -1, 64)
			default:
				values[k] = fmt.Sprint(x)
			}
		}
		got = append(got, values)
	}
	return got, nil
}

// opencodeLocalRows is opencodeRows in process. The database is opened read-only and kept: the chat asks every couple
// of seconds.
func (r *Reader) opencodeLocalRows(db string, query string, args ...any) ([]map[string]string, error) {
	if _, err := os.Stat(db); err != nil {
		return nil, fmt.Errorf("opencode's database is not there: %w", err)
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(db)}
	u.RawQuery = "mode=ro&_pragma=busy_timeout(2000)&_pragma=query_only(1)"
	handle, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	rows, err := handle.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []map[string]string
	for rows.Next() {
		values := make([]sql.NullString, len(columns))
		dest := make([]any, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		row := make(map[string]string, len(columns))
		for i, column := range columns {
			row[column] = values[i].String
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// sqlString is a value in a SQLite query.
func sqlString(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// sqlValue is a value of any of the kinds a query here takes: a number bare, anything else quoted.
func sqlValue(v any) string {
	switch x := v.(type) {
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return sqlString(fmt.Sprint(v))
}

// opencodeSessions lists a folder's top-level conversations, newest first.
func (r *Reader) opencodeSessions(cwd string) ([]SessionInfo, error) {
	rows, err := r.opencodeRows(`SELECT id, directory, title, time_updated FROM session_v2 WHERE parent_id IS NULL ORDER BY time_updated DESC LIMIT 500`)
	if err != nil {
		return nil, err
	}
	style := rpath.For(r.host.Home())
	out := []SessionInfo{}
	for _, row := range rows {
		if !style.Equal(row["directory"], cwd) {
			continue
		}
		ms, _ := strconv.ParseInt(row["time_updated"], 10, 64)
		out = append(out, SessionInfo{
			ID:      row["id"],
			Path:    opencodePath(cwd, row["id"]),
			ModTime: time.UnixMilli(ms),
			Title:   row["title"],
			Agent:   "opencode",
		})
	}
	return out, nil
}

// opencodeResolve is the pseudo-path of a folder's session: the one an id names, else the newest.
func (r *Reader) opencodeResolve(cwd, value string) (string, error) {
	sessions, err := r.opencodeSessions(cwd)
	if err != nil {
		return "", err
	}
	if value != "" {
		for _, session := range sessions {
			if session.ID == value {
				return session.Path, nil
			}
		}
	}
	if len(sessions) == 0 {
		return "", fmt.Errorf("no opencode conversation in %s", cwd)
	}
	return sessions[0].Path, nil
}

// opencodeVersion is when a session's row was last written, in the nanoseconds a Version carries.
func (r *Reader) opencodeVersion(path string) (Version, error) {
	_, id, ok := opencodePathParts(path)
	if !ok {
		return Version{}, fmt.Errorf("%q is not an opencode session", path)
	}
	rows, err := r.opencodeRows(`SELECT time_updated FROM session_v2 WHERE id = ` + sqlString(id))
	if err != nil {
		return Version{}, err
	}
	if len(rows) == 0 {
		return Version{}, fmt.Errorf("opencode session %s is gone", id)
	}
	ms, _ := strconv.ParseInt(rows[0]["time_updated"], 10, 64)
	return Version{MTime: ms * int64(time.Millisecond)}, nil
}

// opencodeTurns reads the session's messages and parts as of the version, into the chat's turns.
func (r *Reader) opencodeTurns(path string, v Version) ([]Turn, error) {
	_, id, ok := opencodePathParts(path)
	if !ok {
		return nil, fmt.Errorf("%q is not an opencode session", path)
	}
	cutoff := v.Time().UnixMilli()
	rows, err := r.opencodeRows(`SELECT m.id AS message_id, m.type AS message_type, m.seq AS message_seq, m.time_created AS message_time, m.data AS message_data, `+
		`p.ordinal AS part_ordinal, p.type AS part_type, p.data AS part_data `+
		`FROM session_message m LEFT JOIN session_part p ON p.message_id = m.id `+
		`WHERE m.session_id = ? AND m.time_created <= ? ORDER BY m.seq, p.ordinal`, id, cutoff)
	if err != nil {
		return nil, err
	}
	return parseOpencode(rows)
}

// OpencodeSessionID says an id is one opencode names a session with: ses_ and its letters and digits.
func OpencodeSessionID(id string) bool { return opencodeSessionID.MatchString(id) }

// opencodeSpawn is a subagent call in the parent: its tool id, the child session it started, and what it was told.
type opencodeSpawn struct {
	ToolID      string
	Child       string
	Description string
	Kind        string
}

var (
	opencodeSessionID   = regexp.MustCompile(`^ses_[A-Za-z0-9]+$`)
	opencodeChildInText = regexp.MustCompile(`sessionID:\s*(ses_[A-Za-z0-9]+)`)
)

// opencodeSpawns maps the parent's subagent calls by the child session they name and by their description: the output
// text says "sessionID: ses_…", and a child's title is the description the call was given.
func (r *Reader) opencodeSpawns(parent string) (byChild, byDescription map[string]opencodeSpawn) {
	byChild, byDescription = map[string]opencodeSpawn{}, map[string]opencodeSpawn{}
	note := func(part opencodePart) {
		if part.Name != "subagent" && part.Name != "task" {
			return
		}
		var input struct {
			Description string `json:"description"`
			Agent       string `json:"agent"`
		}
		_ = json.Unmarshal(part.State.Input, &input)
		spawn := opencodeSpawn{ToolID: part.ID, Description: input.Description, Kind: input.Agent}
		for _, content := range part.State.Content {
			if match := opencodeChildInText.FindStringSubmatch(content.Text); match != nil {
				spawn.Child = match[1]
			}
		}
		if spawn.Child != "" {
			byChild[spawn.Child] = spawn
		}
		if spawn.Description != "" {
			byDescription[spawn.Description] = spawn
		}
	}
	if rows, err := r.opencodeRows(`SELECT call_id, data FROM session_part WHERE session_id = ? AND type = 'tool'`, parent); err == nil {
		for _, row := range rows {
			var part opencodePart
			if json.Unmarshal([]byte(row["data"]), &part) == nil {
				if part.ID == "" {
					part.ID = row["call_id"]
				}
				note(part)
			}
		}
	}
	// a session from before opencode kept its parts apart carries them in the answer's content
	if rows, err := r.opencodeRows(`SELECT data FROM session_message WHERE session_id = ? AND type = 'assistant'`, parent); err == nil {
		for _, row := range rows {
			var data opencodeData
			if json.Unmarshal([]byte(row["data"]), &data) != nil {
				continue
			}
			for _, part := range data.Content {
				note(part)
			}
		}
	}
	return byChild, byDescription
}

// opencodeSubagents is SubagentsFor for opencode: the child sessions of the conversation, as they stand.
func (r *Reader) opencodeSubagents(path string, spawnedBy map[string]bool, now time.Time) ([]Subagent, error) {
	_, parent, ok := opencodePathParts(path)
	if !ok {
		return nil, nil
	}
	byChild, byDescription := r.opencodeSpawns(parent)
	rows, err := r.opencodeRows(`SELECT id, title, model, time_created, time_updated, time_idle FROM session_v2 WHERE parent_id = ? ORDER BY time_created`, parent)
	if err != nil {
		return nil, err
	}
	var out []Subagent
	for _, row := range rows {
		child := row["id"]
		spawn, ok := byChild[child]
		if !ok {
			spawn = byDescription[row["title"]]
		}
		updated, _ := strconv.ParseInt(row["time_updated"], 10, 64)
		created, _ := strconv.ParseInt(row["time_created"], 10, 64)
		idle, _ := strconv.ParseInt(row["time_idle"], 10, 64)
		at := time.UnixMilli(updated)
		fresh := now.Sub(at) <= QuietAfter
		if spawn.ToolID == "" || (!spawnedBy[spawn.ToolID] && !fresh) {
			continue
		}
		status := SubagentWorking
		switch {
		case idle > 0 && idle >= updated:
			status = SubagentDone
		case now.Sub(at) > QuietAfter:
			status = SubagentQuiet
		}
		tools, failed := r.opencodeToolCounts(child)
		description := row["title"]
		if description == "" {
			description = spawn.Description
		}
		out = append(out, Subagent{
			ID: child, ToolID: spawn.ToolID, Description: description, Kind: spawn.Kind,
			Model: opencodeModel(row["model"]).ID, Status: status,
			Tools: tools, Failed: failed, Doing: r.opencodeDoing(child), Report: r.opencodeReport(child),
			Started: time.UnixMilli(created).UTC().Format(time.RFC3339),
			Ended:   at.UTC().Format(time.RFC3339),
			Took:    float64(updated-created) / 1000,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Started != out[j].Started {
			return out[i].Started < out[j].Started
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// opencodeToolCounts is how many tools a child ran and how many failed: its own parts, else the old answers' content.
func (r *Reader) opencodeToolCounts(child string) (tools, failed int) {
	rows, err := r.opencodeRows(`SELECT count(*) AS n, coalesce(sum(case when json_extract(data, '$.state.status') = 'error' then 1 else 0 end), 0) AS failed `+
		`FROM session_part WHERE session_id = ? AND type = 'tool'`, child)
	if err == nil && len(rows) > 0 {
		tools, _ = strconv.Atoi(rows[0]["n"])
		failed, _ = strconv.Atoi(rows[0]["failed"])
		if tools > 0 {
			return tools, failed
		}
	}
	rows, err = r.opencodeRows(`SELECT count(*) AS n, coalesce(sum(case when json_extract(c.value, '$.state.status') = 'error' then 1 else 0 end), 0) AS failed `+
		`FROM session_message m, json_each(m.data, '$.content') c `+
		`WHERE m.session_id = ? AND m.type = 'assistant' AND json_extract(c.value, '$.type') = 'tool'`, child)
	if err == nil && len(rows) > 0 {
		tools, _ = strconv.Atoi(rows[0]["n"])
		failed, _ = strconv.Atoi(rows[0]["failed"])
	}
	return tools, failed
}

// opencodeDoing is the tool call a child is on, if one is open.
func (r *Reader) opencodeDoing(child string) *Part {
	rows, err := r.opencodeRows(`SELECT data FROM session_part WHERE session_id = ? AND type = 'tool' AND json_extract(data, '$.state.status') NOT IN ('completed', 'error') ORDER BY time_updated DESC LIMIT 1`, child)
	if err != nil || len(rows) == 0 {
		return nil
	}
	var part opencodePart
	if json.Unmarshal([]byte(rows[0]["data"]), &part) != nil {
		return nil
	}
	doing, ok := opencodePartOf(part, "", "")
	if !ok {
		return nil
	}
	return &doing
}

// opencodeReport is the start of the text a child's last answer ended with.
func (r *Reader) opencodeReport(child string) string {
	rows, err := r.opencodeRows(`SELECT id, data FROM session_message WHERE session_id = ? AND type = 'assistant' ORDER BY seq DESC LIMIT 1`, child)
	if err != nil || len(rows) == 0 {
		return ""
	}
	var data opencodeData
	_ = json.Unmarshal([]byte(rows[0]["data"]), &data)
	var text string
	for i := len(data.Content) - 1; i >= 0; i-- {
		if data.Content[i].Type == "text" {
			text = data.Content[i].Text
			break
		}
	}
	if text == "" {
		if parts, err := r.opencodeRows(`SELECT data FROM session_part WHERE message_id = ? AND type = 'text' ORDER BY ordinal DESC LIMIT 1`, rows[0]["id"]); err == nil && len(parts) > 0 {
			var part opencodePart
			if json.Unmarshal([]byte(parts[0]["data"]), &part) == nil {
				text = part.Text
			}
		}
	}
	return cutRunes(strings.TrimSpace(text), reportMax)
}

// OpencodeSubagent reads one child session on its own, for the transcript viewer: found false when it is no child of
// the conversation.
func (r *Reader) OpencodeSubagent(parentPath, id string) (turns []Turn, version Version, found bool, err error) {
	dir, parent, ok := opencodePathParts(parentPath)
	if !ok || !OpencodeSessionID(id) {
		return nil, Version{}, false, nil
	}
	rows, err := r.opencodeRows(`SELECT id FROM session_v2 WHERE id = ? AND parent_id = ?`, id, parent)
	if err != nil || len(rows) == 0 {
		return nil, Version{}, false, err
	}
	child := opencodePath(dir, id)
	version, err = r.opencodeVersion(child)
	if err != nil {
		return nil, version, true, err
	}
	turns, err = r.opencodeTurns(child, version)
	return turns, version, true, err
}

// opencodeUsage is a session's meter, from its row and its last answer's tokens.
func (r *Reader) OpencodeUsage(path string) (OpencodeUsage, error) {
	_, id, ok := opencodePathParts(path)
	if !ok {
		return OpencodeUsage{}, fmt.Errorf("%q is not an opencode session", path)
	}
	rows, err := r.opencodeRows(`SELECT cost, tokens_input, tokens_output, tokens_cache_read, tokens_cache_write, time_updated, model FROM session_v2 WHERE id = ` + sqlString(id))
	if err != nil {
		return OpencodeUsage{}, err
	}
	if len(rows) == 0 {
		return OpencodeUsage{}, fmt.Errorf("opencode session %s is gone", id)
	}
	row := rows[0]
	usage := OpencodeUsage{}
	usage.Cost, _ = strconv.ParseFloat(row["cost"], 64)
	usage.Input, _ = strconv.ParseInt(row["tokens_input"], 10, 64)
	usage.Output, _ = strconv.ParseInt(row["tokens_output"], 10, 64)
	usage.CacheRead, _ = strconv.ParseInt(row["tokens_cache_read"], 10, 64)
	usage.CacheWrite, _ = strconv.ParseInt(row["tokens_cache_write"], 10, 64)
	usage.Updated, _ = strconv.ParseInt(row["time_updated"], 10, 64)
	usage.Model = opencodeModel(row["model"]).ID
	last, err := r.opencodeRows(`SELECT data FROM session_message WHERE session_id = ? AND type = 'assistant' AND json_extract(data, '$.time.completed') IS NOT NULL ORDER BY seq DESC LIMIT 1`, id)
	if err == nil && len(last) > 0 {
		var data opencodeData
		if json.Unmarshal([]byte(last[0]["data"]), &data) == nil {
			usage.Context = data.Tokens.Input + data.Tokens.Cache.Read + data.Tokens.Cache.Write
			if data.Model.ID != "" {
				usage.Model = data.Model.ID
			}
		}
	}
	count, err := r.opencodeRows(`SELECT count(*) AS n FROM session_message WHERE session_id = ? AND type = 'assistant'`, id)
	if err == nil && len(count) > 0 {
		usage.Turns, _ = strconv.Atoi(count[0]["n"])
	}
	return usage, nil
}

// OpencodeResponse is one of a machine's opencode answers since a moment, for the usage report.
type OpencodeResponse struct {
	MessageID  string
	Session    string
	Dir        string
	Title      string
	Model      string
	At         time.Time
	Input      int64
	Output     int64
	CacheRead  int64
	CacheWrite int64
	Cost       float64
}

// OpencodeSince is every opencode answer on the machine since a moment, oldest first, as its database keeps them.
func (r *Reader) OpencodeSince(since time.Time) ([]OpencodeResponse, error) {
	rows, err := r.opencodeRows(`SELECT m.id AS message_id, m.session_id AS session_id, m.time_created AS at, m.data AS data, s.directory AS dir, s.title AS title `+
		`FROM session_message m JOIN session_v2 s ON s.id = m.session_id `+
		`WHERE m.type = 'assistant' AND m.time_created >= ? ORDER BY m.time_created`, since.UnixMilli())
	if err != nil {
		return nil, err
	}
	out := make([]OpencodeResponse, 0, len(rows))
	for _, row := range rows {
		var data opencodeData
		if json.Unmarshal([]byte(row["data"]), &data) != nil {
			continue
		}
		at, _ := strconv.ParseInt(row["at"], 10, 64)
		model := data.Model.ID
		if model == "" {
			model = "opencode"
		}
		out = append(out, OpencodeResponse{
			MessageID: row["message_id"], Session: row["session_id"], Dir: row["dir"], Title: row["title"],
			Model: model, At: time.UnixMilli(at),
			Input: data.Tokens.Input, Output: data.Tokens.Output,
			CacheRead: data.Tokens.Cache.Read, CacheWrite: data.Tokens.Cache.Write, Cost: data.Cost,
		})
	}
	return out, nil
}

// opencodeModel is the model a session row names, stored as JSON.
func opencodeModel(raw string) opencodeModelJSON {
	var model opencodeModelJSON
	_ = json.Unmarshal([]byte(raw), &model)
	return model
}

// opencodeData is one message's data JSON.
type opencodeData struct {
	Time    opencodeTime      `json:"time"`
	Text    string            `json:"text"`
	Agent   string            `json:"agent"`
	Model   opencodeModelJSON `json:"model"`
	Content []opencodePart    `json:"content"`
	Cost    float64           `json:"cost"`
	Tokens  struct {
		Input     int64 `json:"input"`
		Output    int64 `json:"output"`
		Reasoning int64 `json:"reasoning"`
		Cache     struct {
			Read  int64 `json:"read"`
			Write int64 `json:"write"`
		} `json:"cache"`
	} `json:"tokens"`
	Finish string `json:"finish"`
}

type opencodeModelJSON struct {
	ID         string `json:"id"`
	ProviderID string `json:"providerID"`
	Variant    string `json:"variant"`
}

type opencodeTime struct {
	Created    int64 `json:"created"`
	Completed  int64 `json:"completed"`
	FirstToken int64 `json:"firstToken"`
	Streamed   int64 `json:"streamed"`
}

// opencodePart is one part of an answer: text, a thought, or a tool call with what it came to.
type opencodePart struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	State struct {
		Status  string          `json:"status"`
		Input   json.RawMessage `json:"input"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Time struct {
			Created   int64 `json:"created"`
			Ran       int64 `json:"ran"`
			Completed int64 `json:"completed"`
		} `json:"time"`
	} `json:"state"`
	Time opencodeTime `json:"time"`
}

// opencodeTools maps opencode's tool names to the ones the chat's cards, links and diffs read.
var opencodeTools = map[string]string{
	"shell": "Bash", "edit": "Edit", "write": "Write", "read": "Read",
	"grep": "Grep", "glob": "Glob", "webfetch": "WebFetch", "task": "Agent",
	"todowrite": "TodoWrite", "todoread": "TodoRead",
}

// opencodeInput gives a tool's input the names the page reads (Claude Code's), keeping opencode's own too.
func opencodeInput(name string, raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var fields map[string]any
	if json.Unmarshal(raw, &fields) != nil {
		return raw
	}
	alias := func(to, from string) {
		if v, ok := fields[from]; ok {
			fields[to] = v
		}
	}
	switch name {
	case "edit":
		alias("file_path", "path")
		alias("old_string", "oldString")
		alias("new_string", "newString")
		alias("replace_all", "replaceAll")
	case "write", "read":
		alias("file_path", "path")
	}
	out, err := json.Marshal(fields)
	if err != nil {
		return raw
	}
	return out
}

// parseOpencode builds turns from the flat message-and-part rows: consecutive answers merge, a question starts one,
// and the answer's tokens are its responses.
func parseOpencode(rows []map[string]string) ([]Turn, error) {
	var turns []Turn
	var promptAt int64
	var ended int64
	finish := func() {
		if len(turns) == 0 || turns[len(turns)-1].Role != "assistant" {
			return
		}
		if promptAt > 0 && ended > promptAt {
			turns[len(turns)-1].Took = float64(ended-promptAt) / 1000
		}
	}
	parts := map[string][]opencodePart{}
	for _, row := range rows {
		if row["part_data"] == "" {
			continue
		}
		var part opencodePart
		if json.Unmarshal([]byte(row["part_data"]), &part) == nil {
			parts[row["message_id"]] = append(parts[row["message_id"]], part)
		}
	}
	seen := map[string]bool{}
	for _, row := range rows {
		id := row["message_id"]
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		var data opencodeData
		if err := json.Unmarshal([]byte(row["message_data"]), &data); err != nil {
			continue
		}
		at := data.Time.Created
		if at == 0 {
			at, _ = strconv.ParseInt(row["message_time"], 10, 64)
		}
		ts := time.UnixMilli(at).UTC().Format(time.RFC3339Nano)
		switch row["message_type"] {
		case "user":
			finish()
			promptAt, ended = at, 0
			var out []Part
			if data.Text != "" {
				out = append(out, Part{Type: "text", Text: data.Text, At: ts})
			}
			// the prompt itself starts the answer, even with no words: an image alone is still a turn
			turns = append(turns, Turn{Role: "user", Ts: ts, IDs: []string{id}, Parts: out, ended: ts})
		case "assistant":
			if len(turns) == 0 || turns[len(turns)-1].Role != "assistant" {
				turns = append(turns, Turn{Role: "assistant", Ts: ts, ended: ts})
			}
			turn := &turns[len(turns)-1]
			turn.IDs = append(turn.IDs, id)
			turn.ended = ts
			if data.Time.Completed > 0 {
				ended = data.Time.Completed
			} else if data.Time.Created > 0 {
				ended = data.Time.Created
			}
			list := parts[id]
			if len(list) == 0 {
				list = data.Content
			}
			for _, raw := range list {
				if part, ok := opencodePartOf(raw, id, ts); ok {
					turn.Parts = append(turn.Parts, part)
				}
			}
			turn.Tokens += data.Tokens.Input + data.Tokens.Output + data.Tokens.Reasoning + data.Tokens.Cache.Read + data.Tokens.Cache.Write
			turn.Output += data.Tokens.Output
			turn.Responses = append(turn.Responses, UsageRecord{
				MessageID:    id,
				Model:        data.Model.ID,
				At:           time.UnixMilli(at),
				Input:        data.Tokens.Input,
				Output:       data.Tokens.Output,
				CacheRead:    data.Tokens.Cache.Read,
				CacheWrite5m: data.Tokens.Cache.Write,
			})
		}
	}
	finish()
	return turns, nil
}

// opencodePartOf is one part as a chat Part: text, a thought, or a tool call with its input and what it returned.
func opencodePartOf(raw opencodePart, messageID, messageTs string) (Part, bool) {
	at := raw.Time.Created
	if at == 0 && raw.State.Time.Created > 0 {
		at = raw.State.Time.Created
	}
	ts := messageTs
	if at > 0 {
		ts = time.UnixMilli(at).UTC().Format(time.RFC3339Nano)
	}
	switch raw.Type {
	case "text":
		if strings.TrimSpace(raw.Text) == "" {
			return Part{}, false
		}
		return Part{Type: "text", Text: raw.Text, At: ts}, true
	case "reasoning":
		if strings.TrimSpace(raw.Text) == "" {
			return Part{}, false
		}
		return Part{Type: "thinking", Text: raw.Text, At: ts}, true
	case "tool":
		output := make([]string, 0, len(raw.State.Content))
		for _, content := range raw.State.Content {
			if content.Type == "text" && content.Text != "" {
				output = append(output, content.Text)
			}
		}
		part := Part{
			Type:     "tool",
			ToolName: opencodeTools[raw.Name],
			ToolID:   raw.ID,
			At:       ts,
			Input:    opencodeInput(raw.Name, raw.State.Input),
			Output:   strings.Join(output, "\n"),
			IsError:  raw.State.Status == "error",
		}
		if part.ToolName == "" {
			part.ToolName = raw.Name
		}
		if raw.State.Time.Created > 0 && raw.State.Time.Completed > raw.State.Time.Created {
			part.Took = float64(raw.State.Time.Completed-raw.State.Time.Created) / 1000
		}
		return part, true
	}
	return Part{}, false
}

// opencodeResolver is the part of the Reader the usage tracker asks for OpenCodeUsage; declared here so a reader
// that cannot read opencode is no error.
var _ interface {
	OpencodeUsage(string) (OpencodeUsage, error)
} = (*Reader)(nil)
