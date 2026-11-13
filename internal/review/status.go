package review

import (
	"strconv"
	"strings"
)

// Stat is git's line counts for one side of a file: what the index holds against HEAD, or the working tree against the
// index. Binary is a file git counts no lines of.
type Stat struct {
	Added   int  `json:"added"`
	Removed int  `json:"removed"`
	Binary  bool `json:"binary,omitempty"`
}

// readGit is how every read here calls git: no optional lock, so a read never writes the index, and no fsmonitor helper.
const readGit = `git --no-optional-locks -c core.fsmonitor=false -c core.quotepath=off`

// statusScript prints the folder's place in the repository (its prefix), then, apart by lines of GS, git's porcelain v2
// status of the folder, the staged numstat (HEAD against the index) and the unstaged one (the index against the
// working tree), all NUL-separated so no name is quoted or cut.
const statusScript = `git rev-parse --show-prefix 2>/dev/null; printf '\035\n'; ` +
	readGit + ` status --porcelain=v2 -z --no-renames --untracked-files=no -- . 2>/dev/null; printf '\n\035\n'; ` +
	readGit + ` diff --cached --no-ext-diff --no-textconv --no-renames --relative --numstat -z 2>/dev/null; printf '\n\035\n'; ` +
	readGit + ` diff --no-ext-diff --no-textconv --no-renames --relative --numstat -z 2>/dev/null`

// gitStatus is one file as git status reads it: the index's letter and the working tree's ("" for unchanged), and an
// unmerged file's two letters, UU both edited and the rest.
type gitStatus struct {
	index, work, conflict string
}

// readStatus reads statusScript's answer into each path's status and its two sides' counts, the paths relative to
// the folder.
func readStatus(section string) (map[string]gitStatus, map[string]Stat, map[string]Stat) {
	parts := strings.SplitN(section, "\x1d\n", 4)
	for len(parts) < 4 {
		parts = append(parts, "")
	}
	prefix := strings.TrimSpace(parts[0])
	statuses := map[string]gitStatus{}
	records := strings.Split(strings.TrimSuffix(parts[1], "\n"), "\x00")
	for i := 0; i < len(records); i++ {
		record := records[i]
		if record == "" {
			continue
		}
		var fields []string
		switch record[0] {
		case '1':
			fields = strings.SplitN(record, " ", 9)
		case '2': // a rename, which --no-renames keeps out; its original path is the next field
			fields = strings.SplitN(record, " ", 10)
			i++
		case 'u':
			fields = strings.SplitN(record, " ", 11)
		default:
			continue
		}
		if len(fields) < 9 || len(fields[1]) != 2 {
			continue
		}
		path, ok := strings.CutPrefix(fields[len(fields)-1], prefix)
		if !ok || path == "" {
			continue
		}
		xy := fields[1]
		if record[0] == 'u' {
			statuses[path] = gitStatus{conflict: xy}
			continue
		}
		statuses[path] = gitStatus{index: letter(xy[0]), work: letter(xy[1])}
	}
	return statuses, readNumstat(parts[2]), readNumstat(parts[3])
}

// letter is porcelain v2's status letter, "" for the dot that means unchanged.
func letter(b byte) string {
	if b == '.' || b == ' ' {
		return ""
	}
	return string(b)
}

// readNumstat reads `git diff --numstat -z`: "added\tremoved\tpath" a record, "-" for a binary file's counts.
func readNumstat(section string) map[string]Stat {
	stats := map[string]Stat{}
	for _, record := range strings.Split(strings.TrimSuffix(section, "\n"), "\x00") {
		added, rest, ok := strings.Cut(record, "\t")
		if !ok {
			continue
		}
		removed, path, ok := strings.Cut(rest, "\t")
		if !ok || path == "" {
			continue
		}
		if added == "-" {
			stats[path] = Stat{Binary: true}
			continue
		}
		a, _ := strconv.Atoi(added)
		r, _ := strconv.Atoi(removed)
		stats[path] = Stat{Added: a, Removed: r}
	}
	return stats
}

// withStatus marks the files as git status reads them — what is staged, what is not, what is in conflict — and adds the
// ones the diff against HEAD does not show: a file staged and changed back, every file of a repository with no commit.
func withStatus(files []File, statuses map[string]gitStatus, staged, unstaged map[string]Stat) []File {
	listed := map[string]bool{}
	for i := range files {
		listed[files[i].Path] = true
		markStatus(&files[i], statuses, staged, unstaged)
	}
	for path := range statuses {
		if listed[path] {
			continue
		}
		file := File{Path: path}
		markStatus(&file, statuses, staged, unstaged)
		if file.Staged != nil {
			file.Added, file.Removed = file.Staged.Added, file.Staged.Removed
		}
		files = append(files, file)
	}
	return files
}

func markStatus(file *File, statuses map[string]gitStatus, staged, unstaged map[string]Stat) {
	if file.Untracked {
		return
	}
	status, ok := statuses[file.Path]
	if !ok {
		return
	}
	if status.conflict != "" {
		file.Conflict = status.conflict
		return
	}
	file.Index, file.Work = status.index, status.work
	if status.index != "" {
		stat := staged[file.Path]
		file.Staged = &stat
	}
	if status.work != "" {
		stat := unstaged[file.Path]
		file.Unstaged = &stat
	}
}
