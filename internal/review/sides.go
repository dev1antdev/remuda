package review

import (
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// The sides of a file the lens shows apart: what is staged (HEAD against the index), what is not (the index against the
// working tree), a commit's own change, and the branch's whole work against the base it was cut from.
const (
	SideStaged   = "staged"
	SideUnstaged = "unstaged"
)

// maxDiff is how much of one diff a read sends.
const maxDiff = 600000

// diffGit is a diff as the lens reads it: git's own, no external driver or text conversion, no colour.
const diffGit = readGit + ` -c color.ui=never diff --no-ext-diff --no-textconv --relative`

// DiffSide is one file's diff on one side of the index; no side is the lens's usual diff since the last look.
func (store *Store) DiffSide(host shell.Host, dir, path, side string) (string, error) {
	quoted := script.Quote(path)
	switch side {
	case "":
		return store.Diff(host, dir, path)
	case SideStaged:
		return shell.Text(host, script.InDir(dir)+diffGit+` --cached -- `+quoted+` | head -c `+strconv.Itoa(maxDiff)+`; exit 0`, 60*time.Second)
	case SideUnstaged:
		return shell.Text(host, script.InDir(dir)+diffGit+` -- `+quoted+` | head -c `+strconv.Itoa(maxDiff)+`; exit 0`, 60*time.Second)
	}
	return "", errors.New("the side is staged, unstaged, or none")
}

var commitRe = regexp.MustCompile(`^[0-9a-f]{4,64}$`)

// CommitOK reports whether sha reads as a commit's hash, whole or short.
func CommitOK(sha string) bool { return commitRe.MatchString(sha) }

// CommitDiff is one file's change in one commit, against its first parent (none for a root commit).
func CommitDiff(host shell.Host, dir, sha, path string) (string, error) {
	if !CommitOK(sha) {
		return "", errors.New("not a commit")
	}
	return shell.Text(host, script.InDir(dir)+readGit+` -c color.ui=never show --format= -m --first-parent --no-ext-diff --no-textconv --no-renames --relative `+sha+` -- `+script.Quote(path)+` | head -c `+strconv.Itoa(maxDiff)+`; exit 0`, 60*time.Second)
}

// CommitView is one commit as the lens opens it: who made it and when, its message, and the files it changed.
type CommitView struct {
	SHA      string       `json:"sha"`
	Author   string       `json:"author"`
	WhenUnix int64        `json:"when"`
	Subject  string       `json:"subject"`
	Body     string       `json:"body,omitempty"`
	Files    []CommitFile `json:"files"`
}

// CommitFile is one file a commit changed, and how: A added, D deleted, M changed, T its type.
type CommitFile struct {
	Path    string `json:"path"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Binary  bool   `json:"binary,omitempty"`
	Status  string `json:"status"`
}

// ReadCommit reads one commit: its head line, then, apart by RS lines, its numstat and its name-status, both against its
// first parent, NUL-separated.
func ReadCommit(host shell.Host, dir, sha string) (CommitView, error) {
	if !CommitOK(sha) {
		return CommitView{}, errors.New("not a commit")
	}
	show := readGit + ` show --format= -m --first-parent --no-ext-diff --no-textconv --no-renames --relative -z ` + sha
	output, err := shell.Text(host, script.InDir(dir)+
		readGit+` log -1 --format='%H%x1f%an%x1f%ct%x1f%s%x1f%b' `+sha+` 2>/dev/null; printf '\036\n'; `+
		show+` --numstat 2>/dev/null; printf '\n\036\n'; `+show+` --name-status 2>/dev/null`, 60*time.Second)
	if err != nil {
		return CommitView{}, err
	}
	sections := script.Sections(output)
	head := script.Fields(strings.TrimRight(sections[0], "\n"))
	if len(head) < 5 || head[0] == "" {
		return CommitView{}, errors.New("no such commit here")
	}
	when, _ := strconv.ParseInt(head[2], 10, 64)
	view := CommitView{SHA: head[0], Author: head[1], WhenUnix: when, Subject: head[3], Body: strings.TrimSpace(strings.Join(head[4:], script.US)), Files: []CommitFile{}}
	if len(sections) < 3 {
		return view, nil
	}
	statuses := map[string]string{}
	fields := strings.Split(strings.Trim(sections[2], "\n"), "\x00")
	for i := 0; i+1 < len(fields); i += 2 {
		if status := strings.TrimSpace(fields[i]); status != "" {
			statuses[fields[i+1]] = status[:1]
		}
	}
	for path, stat := range readNumstat(sections[1]) {
		status := statuses[path]
		if status == "" {
			status = "M"
		}
		view.Files = append(view.Files, CommitFile{Path: path, Added: stat.Added, Removed: stat.Removed, Binary: stat.Binary, Status: status})
	}
	sort.Slice(view.Files, func(i, j int) bool { return view.Files[i].Path < view.Files[j].Path })
	return view, nil
}

// maxConflict is how much of a conflicted file the lens reads, as much as the file viewer shows.
const maxConflict = 512 * 1024

// ConflictRead is a conflicted file as the lens shows it: which of git's stages it has — 1 the common base, 2 yours,
// 3 incoming — and its text with the markers git wrote, cut at maxConflict.
type ConflictRead struct {
	Stages    string `json:"stages"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated,omitempty"`
	Binary    bool   `json:"binary,omitempty"`
	Missing   bool   `json:"missing,omitempty"`
}

// Conflict reads a conflicted file's stages and its text.
func Conflict(host shell.Host, dir, path string) (ConflictRead, error) {
	quoted := script.Quote(path)
	output, err := shell.Text(host, script.InDir(dir)+
		readGit+` ls-files -u -z -- `+quoted+` 2>/dev/null; printf '\n\036\n'; `+
		`if [ -f `+quoted+` ]; then head -c `+strconv.Itoa(maxConflict+1)+` < `+quoted+`; else printf '\035'; fi`, 60*time.Second)
	if err != nil {
		return ConflictRead{}, err
	}
	stages, text, _ := strings.Cut(output, "\n"+script.RS+"\n")
	read := ConflictRead{}
	for _, entry := range strings.Split(stages, "\x00") {
		// "<mode> <object> <stage>\t<path>"
		meta, _, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if ok && len(fields) == 3 && !strings.Contains(read.Stages, fields[2]) {
			read.Stages += fields[2]
		}
	}
	switch {
	case text == script.GS:
		read.Missing = true
	case strings.Contains(text, "\x00"):
		read.Binary = true
	default:
		if len(text) > maxConflict {
			text, read.Truncated = text[:maxConflict], true
		}
		read.Text = text
	}
	return read, nil
}
