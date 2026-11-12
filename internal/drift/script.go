// Package drift is how every machine is set up, side by side: what the vitals already know (the agents, herdr, its
// hooks, remuda, Claude Code's settings) and what one read-only script reads beside them — remuda's MCP server in each
// agent's own config file, the agents' global instruction files, herdr's plugins and every repository checkout behind
// the machine's workspaces — laid out as a table whose norm is what most machines share, the odd one out marked, with
// the update remuda already has that brings it in line where there is one. Nothing is written and no agent is run:
// the config files are read with grep and awk, and only names, versions, hashes and flags leave the machine.
package drift

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/script"
)

// RepoFiles are the files of a checkout whose copies are compared across machines.
var RepoFiles = []string{"CLAUDE.md", "AGENTS.md", ".claude/settings.json"}

// Script reads the machine's setup beyond the vitals: remuda's MCP server in Claude Code's ~/.claude.json, Codex's
// config.toml, Gemini CLI's settings.json and Kimi's ~/.kimi/mcp.json when there is one; the global instruction files
// and every RepoFiles of a checkout as sha256 and mtime; herdr's plugins; and the main checkout of each folder given,
// its remote, branch, how far it is behind its upstream as of the last fetch. One line a fact, its fields cut by US.
func Script(dirs []string) string {
	var quoted []string
	for _, dir := range dirs {
		quoted = append(quoted, script.Quote(dir))
	}
	var files []string
	for _, name := range RepoFiles {
		files = append(files, script.Quote(name))
	}
	folders := ""
	if len(quoted) > 0 {
		folders = `for d in ` + strings.Join(quoted, " ") + `; do ( cd "$d" 2>/dev/null && git rev-parse --show-toplevel >/dev/null 2>&1 || exit 0; ` + review.MainFolderScript + ` ); done | while IFS= read -r root; do [ -n "$root" ] && ( cd "$root" 2>/dev/null && pwd -P ); done | sort -u | while IFS= read -r root; do ( cd "$root" 2>/dev/null && repo "$root" ); done
`
	}
	return script.PATH + `
LC_ALL=C; export LC_ALL
u=$(printf '\037')
digest() { { sha256sum < "$1" || shasum -a 256 < "$1" || openssl dgst -sha256 -r < "$1"; } 2>/dev/null | cut -c1-64; }
mtime() { ` + script.Stat(`"$1"`, "%Y", "%m") + ` 2>/dev/null; }
fact() { if [ -f "$2" ] && [ -r "$2" ]; then printf '%s\037%s\037%s\n' "$1" "$(digest "$2")" "$(mtime "$2")"; else printf '%s\037\037\n' "$1"; fi; }
servers() { awk ` + script.Quote(serversAwk) + ` "$1" 2>/dev/null; }
mcpjson() { v=no; [ -r "$2" ] && servers "$2" | grep -qx remuda && v=yes; printf 'mcp\037%s\037%s\n' "$1" "$v"; }
mcpjson claude "$HOME/.claude.json"
v=no; f="${CODEX_HOME:-$HOME/.codex}/config.toml"; [ -r "$f" ] && grep -Eq '^[[:space:]]*\[mcp_servers\.("remuda"|remuda)\][[:space:]]*(#.*)?$' "$f" && v=yes; printf 'mcp\037codex\037%s\n' "$v"
mcpjson gemini "$HOME/.gemini/settings.json"
[ -r "$HOME/.kimi/mcp.json" ] && mcpjson kimi "$HOME/.kimi/mcp.json"
fact "global${u}claude" "$HOME/.claude/CLAUDE.md"
fact "global${u}codex" "${CODEX_HOME:-$HOME/.codex}/AGENTS.md"
fact "global${u}gemini" "$HOME/.gemini/GEMINI.md"
if command -v herdr >/dev/null 2>&1; then herdr plugin list 2>/dev/null | sed -n -E 's/^- ([^ ]+) .*\[([a-z]+):([^]@]*)[]@].*$/plugin \1 \2:\3/p'; fi
repo() {
  origin=$(git config --get remote.origin.url 2>/dev/null | sed -E 's#^([a-z+]+://)[^@/]*@#\1#')
  first=""; [ -n "$origin" ] || first=$(git rev-list --max-parents=0 HEAD 2>/dev/null | tail -1)
  branch=$({ git symbolic-ref --short -q HEAD || git rev-parse --short HEAD; } 2>/dev/null)
  up=$(git rev-parse --abbrev-ref --symbolic-full-name '@{u}' 2>/dev/null)
  counts=""; [ -n "$up" ] && counts=$(git rev-list --left-right --count 'HEAD...@{u}' 2>/dev/null)
  ahead=$(printf '%s' "$counts" | awk '{print $1}'); behind=$(printf '%s' "$counts" | awk '{print $2}')
  fetched=""; c=$(git rev-parse --git-common-dir 2>/dev/null); [ -f "$c/FETCH_HEAD" ] && fetched=$(mtime "$c/FETCH_HEAD")
  printf 'repo\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\n' "$1" "$origin" "$first" "$branch" "$up" "$behind" "$ahead" "$fetched"
  for f in ` + strings.Join(files, " ") + `; do fact "file${u}$1${u}$f" "$1/$f"; done
}
` + folders + "exit 0\n"
}

// serversAwk prints the names under the top-level "mcpServers" of a pretty-printed JSON file, as Claude Code and Gemini
// CLI write theirs: the top level is the indent of the first key, a server the indent of the first key inside it, and
// the block ends at a line back at the top level. A project's own mcpServers in ~/.claude.json sits deeper and is not read.
const serversAwk = `BEGIN { top = -1; inside = 0; child = -1 }
/^[ \t]*$/ { next }
{ match($0, /^[ \t]*/); n = RLENGTH; c = substr($0, n + 1, 1) }
top < 0 && c == "\"" { top = n }
!inside { if (n == top && $0 ~ /^[ \t]*"mcpServers"[ \t]*:[ \t]*\{[ \t]*$/) inside = 1; next }
n <= top { exit }
child < 0 && c == "\"" { child = n }
n == child && c == "\"" { name = substr($0, n + 2); sub(/".*/, "", name); print name }`

// File is a file as the script read it: absent, or present with its sha256 (empty when no tool there could take it)
// and when it was last changed.
type File struct {
	Present  bool   `json:"present"`
	Hash     string `json:"hash,omitempty"`
	Modified int64  `json:"modified,omitempty"`
}

// value is what is compared across machines: "" for absent, the hash, or "present" when no hash could be taken.
func (f File) value() string {
	switch {
	case !f.Present:
		return ""
	case f.Hash != "":
		return f.Hash
	}
	return "present"
}

// Checkout is a repository's main checkout on a machine.
type Checkout struct {
	Root     string          `json:"root"`
	Origin   string          `json:"origin,omitempty"` // the remote's address, without a user or token in it
	First    string          `json:"first,omitempty"`  // the root commit, read only when there is no origin
	Branch   string          `json:"branch,omitempty"`
	Upstream string          `json:"upstream,omitempty"`
	Behind   int             `json:"behind"`
	Ahead    int             `json:"ahead"`
	Fetched  int64           `json:"fetched,omitempty"` // when the remote was last fetched there
	Files    map[string]File `json:"files"`
}

// Facts is what the script read on one machine.
type Facts struct {
	MCP          map[string]bool   // by agent, whether remuda's server is registered; an agent whose config was not read is absent
	Instructions map[string]File   // the agents' global instruction files, by agent
	Plugins      map[string]string // herdr's plugins by id, each its source: github:owner/repo, or the kind of a local one
	Checkouts    []Checkout
}

var (
	hashRe     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	pluginIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	knownAgent = map[string]bool{"claude": true, "codex": true, "gemini": true, "kimi": true}
)

// Parse reads the script's answer; a line it does not know is left out.
func Parse(out string) Facts {
	facts := Facts{MCP: map[string]bool{}, Instructions: map[string]File{}, Plugins: map[string]string{}}
	byRoot := map[string]int{}
	for _, line := range script.Lines(out) {
		if rest, ok := strings.CutPrefix(line, "plugin "); ok {
			id, source, _ := strings.Cut(rest, " ")
			kind, where, _ := strings.Cut(source, ":")
			if !pluginIDRe.MatchString(id) || kind == "" {
				continue
			}
			if kind == "github" && where != "" {
				facts.Plugins[id] = "github:" + where
			} else {
				facts.Plugins[id] = kind // a local plugin's path is the machine's own, never the same elsewhere
			}
			continue
		}
		fields := script.Fields(line)
		switch {
		case fields[0] == "mcp" && len(fields) == 3 && knownAgent[fields[1]]:
			facts.MCP[fields[1]] = fields[2] == "yes"
		case fields[0] == "global" && len(fields) == 4 && knownAgent[fields[1]]:
			facts.Instructions[fields[1]] = fileOf(fields[2], fields[3])
		case fields[0] == "repo" && len(fields) == 9 && strings.HasPrefix(fields[1], "/"):
			if _, seen := byRoot[fields[1]]; seen {
				continue
			}
			behind, _ := strconv.Atoi(fields[6])
			ahead, _ := strconv.Atoi(fields[7])
			fetched, _ := strconv.ParseInt(fields[8], 10, 64)
			byRoot[fields[1]] = len(facts.Checkouts)
			facts.Checkouts = append(facts.Checkouts, Checkout{Root: fields[1], Origin: fields[2], First: fields[3], Branch: fields[4], Upstream: fields[5], Behind: behind, Ahead: ahead, Fetched: fetched, Files: map[string]File{}})
		case fields[0] == "file" && len(fields) == 5:
			if i, ok := byRoot[fields[1]]; ok && isRepoFile(fields[2]) {
				facts.Checkouts[i].Files[fields[2]] = fileOf(fields[3], fields[4])
			}
		}
	}
	return facts
}

// fileOf is a fact's hash and mtime: no mtime means the file was not there.
func fileOf(hash, modified string) File {
	at, err := strconv.ParseInt(modified, 10, 64)
	if err != nil {
		return File{}
	}
	if !hashRe.MatchString(hash) {
		hash = ""
	}
	return File{Present: true, Hash: hash, Modified: at}
}

func isRepoFile(name string) bool {
	for _, f := range RepoFiles {
		if f == name {
			return true
		}
	}
	return false
}
