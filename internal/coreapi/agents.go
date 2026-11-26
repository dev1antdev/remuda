package coreapi

import (
	"path"
	"strings"
)

// programAgents are the agents remuda starts (sessions.Agents), by the program they run as or the app they report
// themselves as.
var programAgents = map[string]string{
	"claude": "claude", "claude-code": "claude",
	"codex":  "codex",
	"gemini": "gemini", "gemini-cli": "gemini",
	"kimi": "kimi", "kimi-code": "kimi",
	"pi": "pi", "opencode": "opencode", "cursor-agent": "cursor", "amp": "amp", "grok": "grok", "qwen": "qwen",
}

// AgentCommand is the program an agent kind starts as: the kind's own name, else the one program that runs it
// (cursor's cursor-agent); false for a kind the table lacks.
func AgentCommand(kind string) (string, bool) {
	if programAgents[kind] == kind {
		return kind, true
	}
	for program, agent := range programAgents {
		if agent == kind {
			return program, true
		}
	}
	return "", false
}

// AgentProgram is the agent a process's arguments run: its program, or the script an interpreter runs (node …/gemini,
// python …/kimi) past the interpreter's flags, a Windows path and `.exe` too; "" for anything else.
func AgentProgram(argv []string) string {
	for _, word := range argv {
		if strings.HasPrefix(word, "-") {
			continue
		}
		name := strings.ToLower(path.Base(strings.ReplaceAll(word, `\`, "/")))
		if dot := strings.IndexByte(name, '.'); dot > 0 {
			name = name[:dot]
		}
		if agent, ok := programAgents[name]; ok {
			return agent
		}
		if name != "node" && name != "nodejs" && name != "bun" && name != "deno" && !strings.HasPrefix(name, "python") {
			return ""
		}
	}
	return ""
}
