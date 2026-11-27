package transcript

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// Format is which agent wrote a session file, told by where the file lives: Claude Code's under
// ~/.claude/projects, Codex's under ~/.codex/sessions, Gemini CLI's under ~/.gemini/tmp. One Turn type
// comes out of every reader; the formats differ only in how a file is found and read.
type Format string

const (
	Claude   Format = "claude"
	Codex    Format = "codex"
	Gemini   Format = "gemini"
	Opencode Format = "opencode" // a database, not a file: its path is the pseudo-path opencodePath makes
)

// FormatOf is the format of a session file by its path.
func FormatOf(path string) Format {
	if IsOpencode(path) {
		return Opencode
	}
	name := path
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	slash := strings.ReplaceAll(path, `\`, "/")
	switch {
	case strings.HasPrefix(name, "rollout-"):
		return Codex
	case strings.Contains(slash, "/.codex/"):
		return Codex
	case strings.Contains(slash, "/.gemini/"):
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
	case "opencode":
		return Opencode
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

// RootsFor is the session-root snapshot of a machine whose reader knows the folders its agents named for themselves.
// A bare host has only its home's defaults.
func RootsFor(host shell.Host) Roots {
	if rooted, ok := host.(interface{ TranscriptRoots() Roots }); ok {
		return rooted.TranscriptRoots()
	}
	return RootsOf(host.Home())
}

// UnderRoots says whether a session path sits under one of the folders of this machine's set.
func UnderRoots(roots Roots, home, path string) bool {
	return underRoots(roots, rpath.For(home), path)
}

// NamedRoots are a format's folders beyond the home's own defaults: the ones an agent named for itself, in the order
// they were learned.
func NamedRoots(roots Roots, home string, format Format) []string {
	style, defaults := rpath.For(home), RootsOf(home)[format]
	var named []string
	for _, dir := range roots[format] {
		own := false
		for _, known := range defaults {
			if style.Equal(known, dir) {
				own = true
				break
			}
		}
		if !own {
			named = append(named, dir)
		}
	}
	return named
}

// RootFlags names the folders agents named for themselves, beyond the home's own defaults, as command-line flags a
// remote scan is handed: " -root format=path" each.
func RootFlags(roots Roots, home string) string {
	style := rpath.For(home)
	var flags strings.Builder
	for _, format := range []Format{Claude, Codex} {
		for _, dir := range NamedRoots(roots, home, format) {
			flags.WriteString(" -root ")
			flags.WriteString(script.Quote(string(format) + "=" + style.Slash(dir)))
		}
	}
	return flags.String()
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
