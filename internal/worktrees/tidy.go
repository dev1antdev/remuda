package worktrees

// A closed workspace's worktree taken down after it, and the worktrees closes left behind found again. Only a worktree
// herdr or remuda made — under the machine's ~/.herdr/worktrees — is ever touched, and only once nothing in it would be
// lost: its folder when git status is clean, its branch when git sees it merged or the caller knows its request was.

import (
	"errors"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// LeftoverTimeout is how long the look over a machine's worktrees may take: du walks every folder.
var LeftoverTimeout = 90 * time.Second

// Root is where herdr and remuda make worktrees on a machine with this home.
func Root(home string) string {
	return strings.TrimRight(home, "/") + "/.herdr/worktrees"
}

// Made says whether a folder is a worktree herdr or remuda made: inside the root, a repository's folder and its own below it.
func Made(home, folder string) bool {
	if home == "" || folder == "" {
		return false
	}
	rest, ok := strings.CutPrefix(path.Clean(folder), Root(home)+"/")
	return ok && strings.Contains(rest, "/")
}

// Outcome is what became of a worktree after its workspace closed.
type Outcome struct {
	Path          string `json:"path"`
	Branch        string `json:"branch,omitempty"`
	Removed       bool   `json:"removed"`
	BranchDeleted bool   `json:"branch_deleted"`
	Kept          string `json:"kept,omitempty"`        // why the folder stays: uncommitted changes, git's refusal
	BranchKept    string `json:"branch_kept,omitempty"` // why the branch stays, the folder gone: git's line, not merged
}

// Tidy takes a worktree down once nothing in it would be lost; what stays, and why, is in the outcome.
func Tidy(host shell.Shell, folder string, merged bool) Outcome {
	out := Outcome{Path: folder}
	info, err := Inspect(host, folder)
	if err != nil {
		out.Kept = err.Error()
		return out
	}
	out.Branch = info.Branch
	if info.Dirty != "" {
		out.Kept = "uncommitted changes (" + info.Dirty + ")"
		return out
	}
	text, err := shell.Text(host, RemoveScript(info, merged), Timeout)
	var exit *shell.ExitError
	switch {
	case err == nil:
		out.Removed, out.BranchDeleted = true, info.Branch != ""
	case errors.As(err, &exit) && exit.Code == 5:
		out.Removed, out.BranchKept = true, lastLine(text, err.Error())
	default:
		out.Kept = lastLine(text, err.Error())
	}
	return out
}

// Leftover is a worktree under the root that no workspace has open.
type Leftover struct {
	Path   string `json:"path"`
	Repo   string `json:"repo"` // the repository's folder under the root
	Name   string `json:"name"`
	Branch string `json:"branch,omitempty"`
	Dirty  string `json:"dirty,omitempty"`  // the first uncommitted entry, "" when clean
	Broken string `json:"broken,omitempty"` // why it is no worktree git knows any more; such a folder is only named
	KB     int64  `json:"kb"`
}

// LeftoversScript prints a line per folder two levels under the root: the path, the branch (! when the folder holds no
// linked worktree's .git file, ? when git cannot read it), the first line of its status, its size in KB
func LeftoversScript(root string) string {
	return script.PATH + "\nroot=" + script.Quote(root) + "\n" +
		"[ -d \"$root\" ] || exit 0\n" +
		"for dir in \"$root\"/*/*/; do\n" +
		"  [ -d \"$dir\" ] && [ ! -L \"${dir%/}\" ] || continue\n" +
		"  dir=${dir%/}\n" +
		"  if [ ! -f \"$dir/.git\" ]; then branch='!'; dirty=''\n" +
		"  else branch=$(git -C \"$dir\" rev-parse --abbrev-ref HEAD 2>/dev/null) || branch='?'; dirty=$(git -C \"$dir\" status --porcelain --untracked-files=normal 2>/dev/null | head -1); fi\n" +
		"  kb=$(du -sk \"$dir\" 2>/dev/null | cut -f1)\n" +
		"  printf '%s\\037%s\\037%s\\037%s\\n' \"$dir\" \"$branch\" \"$dirty\" \"${kb:-0}\"\n" +
		"done\n"
}

// ParseLeftovers reads the script's lines, leaving out every folder an open workspace or pane sits in.
func ParseLeftovers(root, out string, open []string) []Leftover {
	list := []Leftover{}
	for _, line := range script.Lines(out) {
		fields := script.Fields(line)
		if len(fields) != 4 || !strings.HasPrefix(fields[0], root+"/") || inUse(fields[0], open) {
			continue
		}
		rest := strings.TrimPrefix(fields[0], root+"/")
		repo, name, _ := strings.Cut(rest, "/")
		kb, _ := strconv.ParseInt(strings.TrimSpace(fields[3]), 10, 64)
		left := Leftover{Path: fields[0], Repo: repo, Name: name, Branch: fields[1], Dirty: strings.TrimSpace(fields[2]), KB: kb}
		switch left.Branch {
		case "!":
			left.Branch, left.Broken = "", "no linked worktree here any more"
		case "?":
			left.Branch, left.Broken = "", "git cannot read it"
		case "HEAD":
			left.Branch = ""
		}
		list = append(list, left)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Path < list[j].Path })
	return list
}

// inUse says whether an open folder is the worktree or lies inside it.
func inUse(folder string, open []string) bool {
	for _, dir := range open {
		dir = path.Clean(dir)
		if dir == folder || strings.HasPrefix(dir, folder+"/") {
			return true
		}
	}
	return false
}

// Leftovers lists the worktrees under the machine's root that none of the open folders is in.
func Leftovers(host shell.Host, open []string) ([]Leftover, error) {
	root := Root(host.Home())
	out, err := shell.Text(host, LeftoversScript(root), LeftoverTimeout)
	if err != nil {
		return nil, errors.New(lastLine(out, err.Error()))
	}
	return ParseLeftovers(root, out, open), nil
}
