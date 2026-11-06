package transcript

import (
	"os"
	"path/filepath"
	"strings"
)

// Format is which agent wrote a session file, told by where the file lives: Claude Code's under
// ~/.claude/projects, Codex's under ~/.codex/sessions, Gemini CLI's under ~/.gemini/tmp. One Turn type
// comes out of every reader; the formats differ only in how a file is found and read.
type Format string

const (
	Claude Format = "claude"
	Codex  Format = "codex"
	Gemini Format = "gemini"
)

// FormatOf is the format of a session file by its path.
func FormatOf(path string) Format {
	switch {
	case strings.Contains(path, "/.codex/"):
		return Codex
	case strings.Contains(path, "/.gemini/"):
		return Gemini
	}
	return Claude
}

// FormatOfAgent is the format an agent kind (herdr's `agent` field) records in; "" when the kind keeps no
// transcript remuda reads, so every format is tried.
func FormatOfAgent(agent string) Format {
	switch agent {
	case "claude":
		return Claude
	case "codex":
		return Codex
	case "gemini":
		return Gemini
	}
	return ""
}

// SessionRoots are the folders a home keeps session files in, one per format.
func SessionRoots(home string) map[Format]string {
	return map[Format]string{
		Claude: filepath.Join(home, ".claude", "projects"),
		Codex:  filepath.Join(home, ".codex", "sessions"),
		Gemini: filepath.Join(home, ".gemini", "tmp"),
	}
}

func homeDir() string {
	home, _ := os.UserHomeDir()
	return home
}

// UnderRoot says whether a session path the agent's own hooks reported sits under one of the roots of this home.
func UnderRoot(home, path string) bool {
	clean := filepath.Clean(path)
	for _, root := range SessionRoots(home) {
		if strings.HasPrefix(clean, root+"/") {
			return true
		}
	}
	return false
}

// questionTools are the tools that ask the person something and wait: Claude Code's and Codex's.
var questionTools = map[string]bool{"AskUserQuestion": true, "request_user_input": true}

// IsQuestion says whether a tool call is a question to the person.
func IsQuestion(toolName string) bool { return questionTools[toolName] }

// shellTools are the tools that run a command, with the input key that holds it.
var shellTools = map[string]string{"Bash": "command", "exec_command": "cmd", "shell": "command"}

// ShellCommand is the command a tool call ran, "" for a tool that is not a shell.
func ShellCommand(p *Part) string {
	key, ok := shellTools[p.ToolName]
	if !ok {
		return ""
	}
	return InputField(p.Input, key)
}
