package gitsync

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
)

// Read is what one machine's script found: its repositories, whether the scan found more than it lists, whether the
// folder to scan is not there.
type Read struct {
	Repos       []Repo
	More        bool
	ScanMissing bool
}

// ReadScript lists a machine's repositories and reads each one, in one script: the main folder of each folder given
// (a workspace's; for a linked worktree, its repository's; nothing for one outside git, where MainFolderScript alone
// would answer the folder itself), then the repositories the scan finds — a .git a folder down from each, -H following
// the scan's folder when it is a link and no link below it, hidden and build folders pruned — each once by its physical
// path (a Mac's /tmp is /private/tmp), a record per repository: its git status as porcelain v2, git's own line when
// that failed, whether it has a remote, and FETCH_HEAD's time.
func ReadScript(style rpath.Style, folders []string, scan *Scan) string {
	var b strings.Builder
	b.WriteString(`mtime() { ` + script.Stat(`"$1"`, "%Y", "%m") + ` 2>/dev/null; }; `)
	b.WriteString(`e=$(mktemp 2>/dev/null) || e="${TMPDIR:-/tmp}/remuda-repos.$$"; trap 'rm -f "$e"' EXIT; `)
	root := ""
	if scan != nil {
		root = script.Quote(style.Slash(scan.Folder))
		b.WriteString(`[ -d ` + root + ` ] || printf 'scan-missing\n'; `)
	}
	b.WriteString(`{ `)
	for _, folder := range folders {
		b.WriteString(`r=$(cd ` + script.Quote(style.Slash(folder)) + ` 2>/dev/null && git rev-parse --show-toplevel >/dev/null 2>&1 && { ` + review.MainFolderScript + `; }); [ -n "$r" ] && printf 'w %s\n' "$r"; `)
	}
	if scan != nil {
		prune := `-name '.*'`
		for _, name := range pruned {
			prune += ` -o -name ` + script.Quote(name)
		}
		b.WriteString(`find -H ` + root + ` -mindepth 1 -maxdepth ` + strconv.Itoa(scan.Depth+1) + ` \( -name .git -print -prune \) -o \( -type d \( ` + prune + ` \) -prune \) 2>/dev/null | ` +
			`sed -n 's|/\.git$||p' | head -n ` + strconv.Itoa(MaxScan+1) + ` | sed 's|^|s |'; `)
	}
	b.WriteString(`:; } | awk '!seen[substr($0, 3)]++' | { nl='
'; seen=$nl; while IFS= read -r line; do p=${line#??}; case $line in w\ *) k=w ;; *) k=s ;; esac; ` +
		`real=$(cd "$p" 2>/dev/null && pwd -P) || continue; case $seen in *"$nl$real$nl"*) continue ;; esac; seen=$seen$real$nl; ( cd "$p" 2>/dev/null || exit 0; ` +
		`printf '\035%s %s\036' "$k" "$p"; ` +
		read + ` status --porcelain=v2 --branch -z --no-renames --untracked-files=normal 2>"$e" </dev/null; rc=$?; printf '\036%s\037' "$rc"; ` +
		`if [ "$rc" -ne 0 ]; then { grep -m 1 -e '^fatal:' -e '^error:' "$e" || tail -n 1 "$e"; } | tr -d '\n'; fi; printf '\037'; ` +
		`git remote 2>/dev/null | head -n 1 | tr -d '\n'; printf '\037'; ` +
		`f=$(git rev-parse --git-path FETCH_HEAD 2>/dev/null); if [ -f "$f" ]; then mtime "$f" | tr -d '\n'; fi; printf '\n' ); done; }`)
	return b.String()
}

// ParseRead reads the script's answer: a record a repository, those the scan found past MaxScan left out, sorted by
// the folder they sit in and then by name.
func ParseRead(style rpath.Style, home, output string) Read {
	var result Read
	records := strings.Split(output, "\x1d")
	result.ScanMissing = strings.Contains(records[0], "scan-missing")
	scanned := 0
	for _, record := range records[1:] {
		header, rest, ok := strings.Cut(record, "\x1e")
		end := strings.LastIndex(rest, "\x1e")
		if !ok || end < 0 || len(header) < 3 {
			continue
		}
		fields := strings.Split(strings.TrimRight(rest[end+1:], "\n"), "\x1f")
		for len(fields) < 4 {
			fields = append(fields, "")
		}
		path := style.Clean(header[2:])
		if listed(style, result.Repos, path) {
			continue // a Windows machine's pwd (/c/Users/me/app) and find (C:/Users/me/app) spell one folder two ways
		}
		repo := Repo{Path: path, Name: style.Base(path), Parent: homeWords(style, home, style.Dir(path)), Workspace: header[0] == 'w', Remote: fields[2] != ""}
		if !repo.Workspace {
			if scanned++; scanned > MaxScan {
				result.More = true
				continue
			}
		}
		if fields[0] != "0" {
			repo.Error = strings.TrimSpace(fields[1])
			if repo.Error == "" {
				repo.Error = "git status failed"
			}
		} else {
			repo.Status(rest[:end])
		}
		if seconds, err := strconv.ParseInt(strings.TrimSpace(fields[3]), 10, 64); err == nil && seconds > 0 {
			repo.Fetched = time.Unix(seconds, 0)
		}
		result.Repos = append(result.Repos, repo)
	}
	sort.SliceStable(result.Repos, func(i, j int) bool {
		a, b := result.Repos[i], result.Repos[j]
		if !strings.EqualFold(a.Parent, b.Parent) {
			return strings.ToLower(a.Parent) < strings.ToLower(b.Parent)
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return result
}

// listed says the repository at path is among those read already.
func listed(style rpath.Style, repos []Repo, path string) bool {
	for _, repo := range repos {
		if style.Equal(repo.Path, path) {
			return true
		}
	}
	return false
}

// homeWords is a folder with the machine's home said as ~.
func homeWords(style rpath.Style, home, folder string) string {
	if home == "" {
		return folder
	}
	rel, ok := style.Rel(home, folder)
	switch {
	case !ok:
		return folder
	case rel == ".":
		return "~"
	}
	return "~/" + rel
}
