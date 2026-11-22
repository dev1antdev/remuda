package transcript

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/dev1antdev/remuda/internal/rpath"
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
// A Windows machine's home (C:\Users\me) gives them in its own spelling.
func SessionRoots(home string) map[Format]string {
	join := filepath.Join
	if style := rpath.For(home); style.Windows {
		join = style.Join
	}
	return map[Format]string{
		Claude: join(home, ".claude", "projects"),
		Codex:  join(home, ".codex", "sessions"),
		Gemini: join(home, ".gemini", "tmp"),
	}
}

func homeDir() string {
	home, _ := os.UserHomeDir()
	return home
}

// Roots are the folders one machine keeps session files in, per format: its home's own, and any an agent named for
// itself — Claude Code's CLAUDE_CONFIG_DIR, Codex's CODEX_HOME — learned from the process that writes there.
type Roots map[Format][]string

// RootsOf is a home's own folders, the ones an agent writes to unless it says otherwise.
func RootsOf(home string) Roots {
	out := make(Roots, len(SessionRoots(home)))
	for f, dir := range SessionRoots(home) {
		out[f] = []string{dir}
	}
	return out
}

// With is the set with one folder added for a format, once: the machine's own set, never the one lent.
func (r Roots) With(f Format, dir string) Roots {
	if dir == "" || slices.Contains(r[f], dir) {
		return r
	}
	out := make(Roots, len(r)+1)
	for k, v := range r {
		out[k] = append([]string(nil), v...)
	}
	out[f] = append(out[f], dir)
	return out
}

// learnedRoots are the folders agents named for themselves, by the home of the machine they run on: what the readers,
// the search and the usage counter of that machine look in beside its home's own.
var learnedRoots = struct {
	sync.Mutex
	m map[string]Roots
}{m: map[string]Roots{}}

// RootsFor is a home's own folders and the ones its agents named for themselves, learned from their processes.
func RootsFor(home string) Roots {
	learnedRoots.Lock()
	extra := learnedRoots.m[home]
	learnedRoots.Unlock()
	all := RootsOf(home)
	for f, dirs := range extra {
		for _, dir := range dirs {
			all = all.With(f, dir)
		}
	}
	return all
}

// RememberRoot keeps a folder an agent named for itself, for the machine whose home this is.
func RememberRoot(home string, f Format, dir string) {
	if dir == "" {
		return
	}
	learnedRoots.Lock()
	defer learnedRoots.Unlock()
	learnedRoots.m[home] = learnedRoots.m[home].With(f, dir)
}

// UnderRoot says whether a session path the agent's own hooks reported sits under one of the roots of this home.
func UnderRoot(home, path string) bool {
	return underRoots(RootsOf(home), rpath.For(home), path)
}

// underRoots says whether a path sits under one of the folders of the set.
func underRoots(roots Roots, style rpath.Style, path string) bool {
	for _, dir := range roots {
		for _, root := range dir {
			if rel, ok := style.Rel(root, path); ok && rel != "." {
				return true
			}
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
