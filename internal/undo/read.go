package undo

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// MaxCheckpoint is the most of a checkpoint's patch read from a machine; past it the later files' changes are not kept.
const MaxCheckpoint = 32 << 20

// MaxCommits is the most commits a turn may make and still be reverted.
const MaxCommits = 50

// ReadTimeout bounds one read of a folder on its machine.
var ReadTimeout = 90 * time.Second

// ErrNotRepo is a folder outside git, or a repository with no commit yet: nothing to take a checkpoint of.
var ErrNotRepo = errors.New("not a git checkout with a commit")

// Side is one side of a path in git's raw diff: its mode and blob, "000000" and the null hash when it is absent.
type Side struct {
	Mode string `json:"mode"`
	Blob string `json:"blob"`
}

// Absent says the path does not exist on this side.
func (s Side) Absent() bool { return strings.Trim(s.Mode, "0") == "" }

// Entry is a path that differs between two sides: Old is HEAD's (or the earlier commit's), New the working tree's.
type Entry struct {
	Old Side `json:"old"`
	New Side `json:"new"`
}

// Commit is one commit a turn made, on the branch's first-parent line.
type Commit struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
	When    int64  `json:"when"`
	Merge   bool   `json:"merge,omitempty"`
}

// Read is a folder as one script found it: its repository, HEAD, the uncommitted work and, against a base commit,
// what the commits since it changed.
type Read struct {
	Root   string           `json:"root"`
	Head   string           `json:"head"`
	Branch string           `json:"branch,omitempty"`
	Busy   string           `json:"busy,omitempty"`  // a merge, rebase, cherry-pick or revert in progress
	At     int64            `json:"at,omitempty"`    // the machine's own clock when it read, in Unix seconds
	Files  map[string]Entry `json:"files"`           // every path the working tree holds otherwise than HEAD
	Since  string           `json:"since,omitempty"` // against the base: linear (base is in HEAD's history), rewritten, gone
	Moved  map[string]Entry `json:"moved,omitempty"` // the paths the commits from the base to HEAD changed
	Made   []Commit         `json:"made,omitempty"`  // those commits, newest first, at most MaxCommits+1
	Staged []string         `json:"-"`               // the paths the person's index stages against HEAD
	Pushed []string         `json:"-"`               // the remote-tracking branches holding the commit asked about
	Patch  string           `json:"-"`
	Cut    bool             `json:"cut,omitempty"` // the patch ran past MaxCheckpoint: its last file's section is gone
}

// Patch directions a read takes
const (
	noPatch      = iota
	forwardPatch // HEAD → the working tree: what a turn's start needs to put the work back
	reversePatch // the working tree → HEAD: what a turn's end needs to take its work out
)

type readOptions struct {
	base   string // the commit HEAD is compared with: the start's HEAD at a turn's end, the end's at an undo
	patch  int
	staged bool   // list what the person's index stages
	pushed string // list the remote-tracking branches holding this commit
	limit  int    // the patch's cap, MaxCheckpoint unless a test asks
}

// readScript reads a folder in one script, at its repository's root. The uncommitted work goes through a throwaway
// index and a throwaway object folder (the repository's own objects borrowed as an alternate), so nothing is written
// into the repository: no stash, no object, its own index left as it is. The person's index is copied as the start,
// which keeps git's stat cache (a big checkout is not hashed whole) and a sparse checkout's skipped files.
func readScript(dir string, o readOptions) string {
	limit := o.limit
	if limit <= 0 {
		limit = MaxCheckpoint
	}
	var b strings.Builder
	b.WriteString(script.PATH + "\n")
	b.WriteString("cd " + script.Quote(dir) + " 2>/dev/null || exit 0\n")
	b.WriteString(`root=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0
cd "$root" || exit 0
head=$(git rev-parse -q --verify HEAD) || exit 0
g=$(git rev-parse --git-dir); busy=
[ -e "$g/MERGE_HEAD" ] && busy=merge
[ -e "$g/CHERRY_PICK_HEAD" ] && busy=cherry-pick
[ -e "$g/REVERT_HEAD" ] && busy=revert
{ [ -d "$g/rebase-merge" ] || [ -d "$g/rebase-apply" ]; } && busy=rebase
printf '%s\n%s\n%s\n%s\n%s\n' "$root" "$head" "$(git symbolic-ref --short -q HEAD)" "$busy" "$(date +%s)"
printf '\036\n'
base=` + script.Quote(o.base) + `
if [ -n "$base" ] && [ "$base" != "$head" ]; then
  if ! git cat-file -e "$base^{commit}" 2>/dev/null; then echo gone; printf '\036\n\036\n'
  else
    if git merge-base --is-ancestor "$base" "$head"; then echo linear; else echo rewritten; fi
    printf '\036\n'
    git -c core.quotepath=off diff-tree -r -z --no-renames --no-abbrev --ignore-submodules=all "$base" "$head"
    printf '\036\n'
    git log --first-parent -n ` + strconv.Itoa(MaxCommits+1) + ` --format='%H%x1f%P%x1f%ct%x1f%s' "$base..$head" --
  fi
else printf '\036\n\036\n'; fi
printf '\036\n'
`)
	if o.staged {
		b.WriteString("git -c core.quotepath=off diff-index --cached -z --name-only --no-renames --ignore-submodules=all HEAD 2>/dev/null\n")
	}
	b.WriteString("printf '\\036\\n'\n")
	if o.pushed != "" {
		b.WriteString("git for-each-ref --contains " + script.Quote(o.pushed) + " --format='%(refname:short)' refs/remotes 2>/dev/null\n")
	}
	b.WriteString("printf '\\036\\n'\n")
	b.WriteString(`d=$(mktemp -d "${TMPDIR:-/tmp}/remuda-undo.XXXXXX") || { echo 'no temporary folder on the machine' >&2; exit 4; }
trap 'rm -rf "$d"' EXIT
objects=$(cd "$(git rev-parse --git-path objects)" && pwd -P) || exit 4
index=$(git rev-parse --git-path index)
mkdir "$d/objects" || exit 4
GIT_INDEX_FILE="$d/index"; GIT_OBJECT_DIRECTORY="$d/objects"; GIT_ALTERNATE_OBJECT_DIRECTORIES="$objects"
export GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES
{ cp "$index" "$d/index" 2>/dev/null && git add -A 2>/dev/null; } || { rm -f "$d/index"; git read-tree HEAD && git add -A; } || { echo 'git could not read the work' >&2; exit 4; }
git -c core.quotepath=off diff-index --cached -z --no-renames --no-abbrev --ignore-submodules=all HEAD
printf '\036\n'
`)
	switch o.patch {
	case forwardPatch:
		b.WriteString("git -c core.quotepath=off diff-index --cached -p --binary --full-index --no-renames --ignore-submodules=all HEAD | head -c " + strconv.Itoa(limit+1) + "\n")
	case reversePatch:
		b.WriteString("git -c core.quotepath=off diff-index --cached -R -p --binary --full-index --no-renames --ignore-submodules=all HEAD | head -c " + strconv.Itoa(limit+1) + "\n")
	}
	return b.String()
}

// read runs readScript on the folder's machine and parses its answer.
func read(host shell.Host, dir string, o readOptions) (Read, error) {
	out, err := shell.Text(host, readScript(dir, o), ReadTimeout)
	if err != nil {
		return Read{}, fmt.Errorf("%s could not be read on %s: %w", dir, host.Label(), err)
	}
	limit := o.limit
	if limit <= 0 {
		limit = MaxCheckpoint
	}
	return parseRead(out, limit)
}

// parseRead reads readScript's eight sections; the patch is the last, taken whole, since a file's bytes may hold the
// separator.
func parseRead(out string, limit int) (Read, error) {
	if strings.TrimSpace(out) == "" {
		return Read{}, ErrNotRepo
	}
	sections := strings.SplitN(out, script.RS+"\n", 8)
	if len(sections) < 7 {
		return Read{}, fmt.Errorf("the folder's answer was cut short")
	}
	lines := strings.Split(strings.TrimRight(sections[0], "\n"), "\n")
	for len(lines) < 5 {
		lines = append(lines, "")
	}
	r := Read{Root: lines[0], Head: strings.TrimSpace(lines[1]), Branch: strings.TrimSpace(lines[2]), Busy: strings.TrimSpace(lines[3])}
	r.At, _ = strconv.ParseInt(strings.TrimSpace(lines[4]), 10, 64)
	r.Since = strings.TrimSpace(sections[1])
	r.Moved = parseRaw(sections[2])
	for _, line := range script.Lines(sections[3]) {
		fields := script.Fields(line)
		if len(fields) < 4 {
			continue
		}
		when, _ := strconv.ParseInt(fields[2], 10, 64)
		r.Made = append(r.Made, Commit{SHA: fields[0], Merge: len(strings.Fields(fields[1])) > 1, When: when, Subject: strings.Join(fields[3:], script.US)})
	}
	for _, path := range strings.Split(sections[4], "\x00") {
		if path = strings.TrimPrefix(path, "\n"); path != "" {
			r.Staged = append(r.Staged, path)
		}
	}
	r.Pushed = script.Lines(sections[5])
	r.Files = parseRaw(sections[6])
	if len(sections) == 8 {
		r.Patch = sections[7]
		if len(r.Patch) > limit {
			r.Patch, r.Cut = r.Patch[:limit], true
		}
	}
	return r, nil
}

// parseRaw reads git's raw diff in its -z form, ":<old mode> <new mode> <old blob> <new blob> <status>\0<path>\0" a
// path, into its entries by path.
func parseRaw(section string) map[string]Entry {
	entries := map[string]Entry{}
	parts := strings.Split(section, "\x00")
	for i := 0; i+1 < len(parts); i += 2 {
		fields := strings.Fields(strings.TrimPrefix(strings.TrimLeft(parts[i], "\n"), ":"))
		if len(fields) < 5 || parts[i+1] == "" {
			continue
		}
		entries[parts[i+1]] = Entry{Old: Side{Mode: fields[0], Blob: fields[2]}, New: Side{Mode: fields[1], Blob: fields[3]}}
	}
	return entries
}
