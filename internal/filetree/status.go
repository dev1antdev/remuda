package filetree

import (
	"fmt"
	"strings"

	"github.com/dev1antdev/remuda/internal/script"
)

// Status is git's word on a checkout, keyed by path relative to the tree's root; a folder git names whole (an
// untracked or ignored one) ends with a slash.
type Status struct {
	Repo    bool
	Err     string // what git said when it could not read the checkout
	marks   map[string]string
	ignored map[string]bool
	holds   map[string]bool // the folders holding a change
	whole   string          // the mark of a folder the tree's root lies inside, "!" for an ignored one
}

// statusScript prints "repo" and the tree root's place in its checkout, then git's status NUL-separated, then what
// git said on stderr; "none" outside git. A read only: no optional locks, no fsmonitor, so the index is never touched.
func statusScript(root string) string {
	return `cd ` + script.Quote(root) + ` 2>/dev/null || exit 0
git rev-parse --is-inside-work-tree >/dev/null 2>&1 || { printf 'none\n'; exit 0; }
printf 'repo\n'; git rev-parse --show-prefix; printf '\036\n'
{ err=$( { git --no-optional-locks -c core.fsmonitor=false -c core.quotepath=off status --porcelain=v1 -z --no-renames --ignored=matching --untracked-files=normal 2>&4 | head -c ` + fmt.Sprint(statusLimit) + ` >&3; } 4>&1 ); } 3>&1
printf '\036\n%s' "$err"`
}

// ParseStatus reads statusScript's answer.
func ParseStatus(out string) Status {
	sections := script.Sections(out)
	head := strings.Split(strings.TrimRight(sections[0], "\r\n"), "\n")
	st := Status{marks: map[string]string{}, ignored: map[string]bool{}, holds: map[string]bool{}}
	if len(head) == 0 || strings.TrimSpace(head[0]) != "repo" {
		return st
	}
	st.Repo = true
	prefix := ""
	if len(head) > 1 {
		prefix = strings.TrimRight(head[1], "\r")
	}
	if len(sections) > 2 {
		st.Err = strings.TrimSpace(sections[2])
	}
	if len(sections) < 2 {
		return st
	}
	records := strings.Split(sections[1], "\x00")
	if !strings.HasSuffix(sections[1], "\x00") {
		records = records[:len(records)-1] // the stream was cut inside its last record
	}
	for _, record := range records {
		if len(record) < 4 {
			continue
		}
		xy, path := record[:2], record[3:]
		mark := markOf(xy)
		rel, inside := strings.CutPrefix(path, prefix)
		if !inside {
			if strings.HasSuffix(path, "/") && strings.HasPrefix(prefix, path) && (mark == "?" || mark == "!") {
				st.whole = mark
			}
			continue
		}
		if rel == "" {
			continue
		}
		if mark == "!" {
			st.ignored[rel] = true
			continue
		}
		st.marks[rel] = mark
		for dir := parent(strings.TrimSuffix(rel, "/")); dir != ""; dir = parent(dir) {
			st.holds[dir] = true
		}
	}
	return st
}

// markOf is the one letter a row wears for git's two status columns.
func markOf(xy string) string {
	switch {
	case xy == "??":
		return "?"
	case xy == "!!":
		return "!"
	case strings.Contains(xy, "U") || xy == "AA" || xy == "DD":
		return "U"
	case strings.Contains(xy, "D"):
		return "D"
	case xy[0] == 'A':
		return "A"
	}
	return "M"
}

// parent is the folder holding a tree-relative path, "" at the root.
func parent(rel string) string {
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[:i]
	}
	return ""
}

// Apply marks the entries of the folder at rel and adds the files git says were deleted from it — unless the
// listing was cut, when a name it lacks may only lie past the cap.
func (st Status) Apply(rel string, entries []Entry, cut bool) []Entry {
	listed := map[string]bool{}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		listed[e.Name] = true
		path := e.Name
		if rel != "" {
			path = rel + "/" + e.Name
		}
		e.Mark = st.markAt(path, e.Dir)
		e.Ignored = st.ignoredAt(path, e.Dir)
		e.Changes = e.Dir && !e.Ignored && st.holds[path]
		out = append(out, e)
	}
	if cut {
		return out
	}
	within := ""
	if rel != "" {
		within = rel + "/"
	}
	ghosts := map[string]bool{}
	for path, mark := range st.marks {
		rest, ok := strings.CutPrefix(path, within)
		if mark != "D" || !ok || rest == "" {
			continue
		}
		name, deeper, _ := strings.Cut(rest, "/")
		if listed[name] || ghosts[name] {
			continue
		}
		ghosts[name] = true
		if deeper != "" {
			out = append(out, Entry{Name: name, Dir: true, Gone: true, Changes: true})
		} else {
			out = append(out, Entry{Name: name, Gone: true, Mark: "D"})
		}
	}
	return out
}

// markAt is a path's own mark, else the mark of a new folder it lies in.
func (st Status) markAt(path string, dir bool) string {
	if mark := st.marks[path]; mark != "" {
		return mark
	}
	if dir {
		if mark := st.marks[path+"/"]; mark != "" {
			return mark
		}
	}
	for folder := parent(path); folder != ""; folder = parent(folder) {
		if st.marks[folder+"/"] == "?" {
			return "?"
		}
	}
	if st.whole == "?" {
		return "?"
	}
	return ""
}

// ignoredAt says git ignores the path or a folder it lies in.
func (st Status) ignoredAt(path string, dir bool) bool {
	if st.whole == "!" || st.ignored[path] || dir && st.ignored[path+"/"] {
		return true
	}
	for folder := parent(path); folder != ""; folder = parent(folder) {
		if st.ignored[folder+"/"] {
			return true
		}
	}
	return false
}
