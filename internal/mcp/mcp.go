// Package mcp is `remuda mcp`: the task list as a Model Context Protocol server over stdio — JSON-RPC 2.0, one
// message per line — so any coding agent that speaks MCP can read, create, move and comment on the person's tasks
// through the tools in tools.go. The wire format is the protocol's stdio transport; nothing else is spoken.
package mcp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Version is the protocol revision answered to a client whose own is unknown here.
const Version = "2025-06-18"

var knownVersions = map[string]bool{"2024-11-05": true, "2025-03-26": true, "2025-06-18": true, "2025-11-25": true}

// the JSON-RPC error codes the server uses
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// maxLine is the longest message read from the client: a task description a few megabytes long still fits.
const maxLine = 16 << 20

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Server answers one client over a pair of streams.
type Server struct {
	tools   *Tools
	version string
}

// New is a server over the tools; version names this remuda to the client.
func New(tools *Tools, version string) *Server {
	return &Server{tools: tools, version: version}
}

// Serve reads messages until the input ends; every request gets an answer, a notification none.
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64<<10), maxLine)
	w := bufio.NewWriter(out)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.reply(w, response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: codeParse, Message: "parse error: " + err.Error()}})
			continue
		}
		if !isRequest(req) {
			continue // a notification: nothing is owed
		}
		result, rpcErr := s.handle(req)
		res := response{JSONRPC: "2.0", ID: req.ID, Result: result}
		if rpcErr != nil {
			res.Result, res.Error = nil, rpcErr
		}
		s.reply(w, res)
	}
	return scanner.Err()
}

func isRequest(req request) bool {
	return len(req.ID) > 0 && string(req.ID) != "null"
}

func (s *Server) reply(w *bufio.Writer, res response) {
	b, err := json.Marshal(res)
	if err != nil {
		b, _ = json.Marshal(response{JSONRPC: "2.0", ID: res.ID, Error: &rpcError{Code: -32603, Message: "internal error: " + err.Error()}})
	}
	w.Write(b)
	w.WriteByte('\n')
	w.Flush()
}

// handle is one request's result or error.
func (s *Server) handle(req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := Version
		if knownVersions[p.ProtocolVersion] {
			version = p.ProtocolVersion
		}
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "remuda", "version": s.version},
			"instructions":    instructions,
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": Specs()}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
			return nil, &rpcError{Code: codeInvalidParams, Message: "tools/call needs a name and arguments"}
		}
		text, err := s.tools.Call(p.Name, p.Arguments)
		if errors.Is(err, errNoTool) {
			return nil, &rpcError{Code: codeInvalidParams, Message: err.Error()}
		}
		if err != nil {
			return toolResult(err.Error(), true), nil
		}
		return toolResult(text, false), nil
	case "resources/list":
		return map[string]any{"resources": []any{}}, nil
	case "resources/templates/list":
		return map[string]any{"resourceTemplates": []any{}}, nil
	case "prompts/list":
		return map[string]any{"prompts": []any{}}, nil
	}
	if req.Method == "" {
		return nil, &rpcError{Code: codeInvalidRequest, Message: "a request needs a method"}
	}
	return nil, &rpcError{Code: codeMethodNotFound, Message: fmt.Sprintf("method not found: %s", req.Method)}
}

func toolResult(text string, isError bool) map[string]any {
	out := map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}
	if isError {
		out["isError"] = true
	}
	return out
}

// instructions is what the client shows its model about this server.
const instructions = `remuda keeps the person's task list: projects, each with workflow columns (New → In progress → Testing → Done by default) and fields, tasks with labels, due dates, priorities and comments.
Use tasks_list to see what is open (today, upcoming or everything, by project, searched), task_get for one task in full, task_create for every new piece of work you propose or are asked to record, task_update to move a task to another column or mark it done, and task_comment to leave a note on a task.
Write descriptions in Markdown: what is wrong or missing, where in the code, what done looks like. One task per improvement, titled as an imperative sentence. Check tasks_list before creating a task so nothing is filed twice.`
