package concierge

import (
	"bytes"
	"encoding/json"
)

// How the concierge is held to reading and filing tasks, in Claude Code's own terms (read off 2.1.322's settings
// schema and CLI): --settings, a JSON string merged over the person's own settings with the higher rank, whose
// permissions allow remuda's tools by name and deny every built-in that reads or writes a file, runs a command, goes on
// the web, starts another agent or schedules something, the mode dontAsk denying whatever was not allowed, and
// denyInsteadOfPrompting so nothing ever waits on a prompt; --strict-mcp-config with --mcp-config naming remuda's own
// server, so none of the person's other MCP servers, nor claude.ai's connectors, are there at all.

// Tools are remuda's tools the concierge may call: the reads of the agents and the task list, a new task and a comment.
var Tools = []string{"agents_list", "agent_recap", "standup", "tasks_list", "task_get", "projects_list", "labels_list", "task_create", "task_comment"}

// denied is every tool the concierge never gets, by Claude Code's names: a deny rule wins over an allow rule the person's
// own settings may hold, and over any permission mode.
var denied = []string{
	"mcp__remuda__task_update", // a task's column is the person's to move
	"Bash", "BashOutput", "KillShell", "KillBash", "PowerShell", "REPL", "Monitor",
	"Read", "Write", "Edit", "MultiEdit", "NotebookRead", "NotebookEdit", "Glob", "Grep", "LS", "LSP",
	"WebFetch", "WebSearch",
	"Agent", "Task", "TaskOutput", "TaskStop", "SendMessage", "TeamCreate", "Skill", "Workflow",
	"EnterWorktree", "ExitWorktree", "CronCreate", "CronDelete", "ScheduleWakeup", "RemoteTrigger", "Artifact",
}

// Server is the name remuda's MCP server goes by in the concierge, the prefix of its tools' names.
const Server = "remuda"

// Settings is the --settings value the concierge starts with.
func Settings() string {
	allow := make([]string, len(Tools))
	for i, tool := range Tools {
		allow[i] = "mcp__" + Server + "__" + tool
	}
	return compact(map[string]any{"permissions": map[string]any{
		"allow":                  allow,
		"deny":                   denied,
		"defaultMode":            "dontAsk",
		"denyInsteadOfPrompting": true,
	}})
}

// MCPConfig is the --mcp-config value: remuda's own server, the binary given run as `remuda mcp`.
func MCPConfig(remuda string) string {
	return compact(map[string]any{"mcpServers": map[string]any{Server: map[string]any{"type": "stdio", "command": remuda, "args": []string{"mcp"}}}})
}

// Args are the concierge's arguments after any resume: its settings, then its one MCP server. --mcp-config takes
// several values, so it comes last.
func Args(remuda string) []string {
	return []string{"--settings", Settings(), "--strict-mcp-config", "--mcp-config", MCPConfig(remuda)}
}

// compact is the value as one line of JSON with its characters as they are.
func compact(v any) string {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(v)
	return string(bytes.TrimSuffix(buf.Bytes(), []byte("\n")))
}
