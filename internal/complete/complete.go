// Package complete lists what the composer can offer at the caret: the slash commands an agent knows on a
// machine — its own built-ins and, for Claude Code, the commands and skills under ~/.claude and the folder's
// .claude — and the folder's files for an @. Read by one script per list over a shell.Host, kept a minute.
package complete

import (
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// Command is one slash command the composer can insert.
type Command struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source"` // builtin, user (under ~/.claude) or project (under the folder's .claude)
}

// Paths is a folder's files relative to it, and whether the list was cut at MaxPaths.
type Paths struct {
	Paths []string `json:"paths"`
	Cut   bool     `json:"cut,omitempty"`
}

const (
	// CacheFor is how long a machine's answer serves the page before the folder is read again.
	CacheFor = time.Minute
	// MaxPaths is the most files a folder is listed with; a bigger folder is cut and says so.
	MaxPaths = 5000
	// MaxDescription is where a command's description is cut, at a word.
	MaxDescription = 120
	headLines      = 25
	timeout        = 15 * time.Second
)

// Service reads the lists through a machine's shell and keeps each answer CacheFor.
type Service struct {
	mu    sync.Mutex
	cache map[string]cached
	now   func() time.Time
}

type cached struct {
	at       time.Time
	commands []Command
	paths    Paths
}

func New() *Service {
	return &Service{cache: map[string]cached{}, now: time.Now}
}

// Commands is what a slash offers an agent of the kind in the folder: the kind's built-ins, then — for Claude Code —
// the person's own commands and skills under ~/.claude and the folder's (and its repository's) .claude.
func (s *Service) Commands(host shell.Host, dir, agent string) ([]Command, error) {
	list := Builtins(agent)
	if agent != "claude" {
		return list, nil
	}
	key := "commands|" + host.ID() + "|" + dir
	if c, ok := s.get(key); ok {
		return append(list, c.commands...), nil
	}
	out, err := shell.Text(host, commandsScript(dir), timeout)
	if err != nil {
		return nil, err
	}
	found := ParseCommands(out)
	s.put(key, cached{commands: found})
	return append(list, found...), nil
}

// Files is what an @ offers: the folder's files as git lists them (tracked and untracked, the ignored left out),
// or every file under it outside git, at most MaxPaths.
func (s *Service) Files(host shell.Host, dir string) (Paths, error) {
	key := "paths|" + host.ID() + "|" + dir
	if c, ok := s.get(key); ok {
		return c.paths, nil
	}
	out, err := shell.Text(host, pathsScript(dir), timeout)
	if err != nil {
		return Paths{}, err
	}
	found := ParsePaths(out)
	s.put(key, cached{paths: found})
	return found, nil
}

func (s *Service) get(key string) (cached, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.cache[key]
	if !ok || s.now().Sub(c.at) > CacheFor {
		return cached{}, false
	}
	return c, true
}

func (s *Service) put(key string, c cached) {
	c.at = s.now()
	s.mu.Lock()
	s.cache[key] = c
	s.mu.Unlock()
}

// commandsScript prints, for every command file and skill of the person and of the folder, a line
// "<source> US <path>" and the file's first lines, each record ended by a GS line; the parsing is ParseCommands's.
func commandsScript(dir string) string {
	head := strconv.Itoa(headLines)
	return script.PATH + `
dump() { printf '%s\037%s\n' "$1" "$2"; head -n ` + head + ` "$2"; printf '\035\n'; }
commands() { [ -d "$2" ] || return 0; find "$2" -type f -name '*.md' 2>/dev/null | sort | while IFS= read -r f; do dump "$1" "$f"; done; }
skills() { for f in "$2"/*/SKILL.md; do [ -f "$f" ] && dump "$1" "$f"; done; }
commands user "$HOME/.claude/commands"
skills user "$HOME/.claude/skills"
dir=` + script.Quote(dir) + `
root=$(cd "$dir" 2>/dev/null && git rev-parse --show-toplevel 2>/dev/null)
commands project "$dir/.claude/commands"
skills project "$dir/.claude/skills"
if [ -n "$root" ] && [ "$root" != "$dir" ]; then commands project "$root/.claude/commands"; skills project "$root/.claude/skills"; fi
exit 0
`
}

// ParseCommands reads commandsScript's answer: a Command per record, named by its file (a skill by its folder, or
// the name its frontmatter gives), described by the frontmatter's description or the first line of the text.
func ParseCommands(out string) []Command {
	var list []Command
	for _, record := range strings.Split(out, script.GS+"\n") {
		lines := strings.Split(record, "\n")
		head := script.Fields(lines[0])
		if len(head) != 2 || head[1] == "" {
			continue
		}
		name, description := describe(head[1], lines[1:])
		if name == "" {
			continue
		}
		list = append(list, Command{Name: name, Description: description, Source: head[0]})
	}
	return list
}

// describe names a command file and finds its description.
func describe(file string, body []string) (name, description string) {
	base := path.Base(file)
	skill := base == "SKILL.md"
	if skill {
		name = path.Base(path.Dir(file))
	} else {
		name = strings.TrimSuffix(base, ".md")
	}
	if len(body) > 0 && strings.TrimSpace(body[0]) == "---" {
		end := len(body)
		for i := 1; i < len(body); i++ {
			if strings.TrimSpace(body[i]) == "---" {
				end = i
				break
			}
			key, value, ok := strings.Cut(body[i], ":")
			if !ok {
				continue
			}
			switch strings.TrimSpace(key) {
			case "description":
				description = unquote(value)
			case "name":
				if v := unquote(value); skill && v != "" {
					name = v
				}
			}
		}
		if end < len(body) {
			body = body[end+1:]
		} else {
			body = nil
		}
	}
	if description == "" {
		for _, line := range body {
			line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#"))
			if line != "" {
				description = line
				break
			}
		}
	}
	return name, cut(description, MaxDescription)
}

func unquote(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		v = v[1 : len(v)-1]
	}
	return strings.TrimSpace(v)
}

// cut ends a text at a word by max runes, with an ellipsis.
func cut(text string, max int) string {
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	cutAt := max
	for i := max; i > max/2; i-- {
		if runes[i] == ' ' {
			cutAt = i
			break
		}
	}
	return strings.TrimRight(string(runes[:cutAt]), " ,;:") + "…"
}

// pathsScript lists the folder's files, one a line, relative to it; one more than MaxPaths so a cut shows.
func pathsScript(dir string) string {
	return script.InDir(dir) + "if git rev-parse --show-toplevel >/dev/null 2>&1; then git ls-files --cached --others --exclude-standard 2>/dev/null; else find . -type f -not -path '*/.git/*' -not -path '*/node_modules/*' 2>/dev/null | sed 's|^\\./||'; fi | head -n " + strconv.Itoa(MaxPaths+1)
}

// ParsePaths reads pathsScript's answer.
func ParsePaths(out string) Paths {
	lines := script.Lines(out)
	if len(lines) > MaxPaths {
		return Paths{Paths: lines[:MaxPaths], Cut: true}
	}
	return Paths{Paths: lines}
}
