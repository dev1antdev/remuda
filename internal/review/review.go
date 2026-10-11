// Package review answers "what changed here since I last looked": for a
// workspace's folder, the commits and the working-tree changes since a
// baseline the viewer set the last time they reviewed it. The baseline is
// the commit that was HEAD then plus the content hashes of the files that
// were already dirty then, so a file reads as new only when its content
// moved since. Everything is one shell script per question, run locally or
// over ssh; nothing is written into the repository.
package review

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/sshx"
	"github.com/dev1antdev/remuda/internal/state"
)

const baselineFile = "review.json"

// Baseline is where the viewer last left off in a folder.
type Baseline struct {
	Head  string            `json:"head"`  // HEAD at the time
	Files map[string]string `json:"files"` // dirty files then, by blob hash
	At    time.Time         `json:"at"`
}

type Commit struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
	When    int64  `json:"when"`           // unix seconds
	Seen    bool   `json:"seen,omitempty"` // every file it brought matches what was reviewed (a commit of reviewed work, an amend, a rebase)
}

type File struct {
	Path      string `json:"path"`
	Added     int    `json:"added"`
	Removed   int    `json:"removed"`
	Untracked bool   `json:"untracked,omitempty"`
	Fresh     bool   `json:"fresh"` // changed since the baseline
}

// Summary is the review of one folder.
type Summary struct {
	Repo     string    `json:"repo,omitempty"` // "" when not a git checkout
	Branch   string    `json:"branch,omitempty"`
	Head     string    `json:"head,omitempty"`
	Baseline *Baseline `json:"baseline,omitempty"`
	Commits  []Commit  `json:"commits"` // since the baseline, newest first
	Files    []File    `json:"files"`   // changed vs the baseline commit, plus untracked
	Fresh    int       `json:"fresh"`   // how many of them moved since the viewer looked
	Error    string    `json:"error,omitempty"`
	At       time.Time `json:"at"`
}

// Badge is what the roster carries per workspace.
type Badge struct {
	Changed int `json:"changed"`
	Fresh   int `json:"fresh"`
}

type Store struct {
	dir string

	mu        sync.Mutex
	baselines map[string]Baseline // "machine|folder"
	badges    map[string]Badge
}

func key(machineID, dir string) string { return machineID + "|" + dir }

func New(dir string) *Store {
	st := &Store{dir: dir, baselines: map[string]Baseline{}, badges: map[string]Badge{}}
	if err := state.LoadJSON(dir, baselineFile, &st.baselines); err != nil {
		log.Printf("%s is unreadable (%v): every folder counts as never reviewed", baselineFile, err)
	}
	return st
}

func (st *Store) save() error {
	return state.SaveJSON(st.dir, baselineFile, st.baselines)
}

func (st *Store) baseline(machineID, dir string) (Baseline, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	b, ok := st.baselines[key(machineID, dir)]
	return b, ok
}

// Badge is the cached count for the roster.
func (st *Store) Badge(machineID, dir string) (Badge, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	b, ok := st.badges[key(machineID, dir)]
	return b, ok
}

// the script: everything the summary needs, in one round trip. Fields are
// separated by \x1f, records by lines; sections by a line of one \x1e.
func summaryScript(dir, base string) string {
	q := sshx.Quote(dir)
	b := sshx.Quote(base)
	return `cd ` + q + ` 2>/dev/null || exit 0; root=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0; ` +
		`printf '%s\n' "$root"; git rev-parse --abbrev-ref HEAD 2>/dev/null; git rev-parse HEAD 2>/dev/null; printf '\036\n'; ` +
		`base=` + b + `; if [ -n "$base" ] && git cat-file -e "$base^{commit}" 2>/dev/null; then :; else base=HEAD; fi; ` +
		`if [ "$base" != HEAD ]; then git log --format='%h%x1f%s%x1f%ct' "$base"..HEAD 2>/dev/null; fi; printf '\036\n'; ` +
		`git -c core.quotepath=off diff --no-renames --relative --numstat "$base" 2>/dev/null; printf '\036\n'; ` +
		`git -c core.quotepath=off ls-files --others --exclude-standard 2>/dev/null; printf '\036\n'; ` +
		hashLoop("$base") + `; printf '\036\n'; ` +
		// per commit since the baseline: the blob every file ends up with, so
		// a commit of work already reviewed can be told from new work
		`if [ "$base" != HEAD ]; then for c in $(git rev-list "$base"..HEAD 2>/dev/null); do printf '%s\n' "$c"; git -c core.quotepath=off diff-tree --no-commit-id -r --relative --no-renames "$c" 2>/dev/null | while IFS= read -r l; do printf '\037%s\n' "$l"; done; done; fi`
}

// hashLoop prints "<path>\x1f<blob hash>" for every file changed against
// base or untracked — one path per line, so a space in a name survives; a
// deleted file is recorded as "gone", so its deletion can be looked at and
// marked seen like any other change. Renames are a deletion plus an
// addition throughout, so paths stay plain, and every path is relative to
// the folder (a workspace may sit below its repository's root).
func hashLoop(base string) string {
	return `{ git -c core.quotepath=off diff --no-renames --relative --name-only ` + base + ` 2>/dev/null; git -c core.quotepath=off ls-files --others --exclude-standard 2>/dev/null; } | ` +
		`while IFS= read -r f; do if [ -f "$f" ]; then printf '%s\037%s\n' "$f" "$(git hash-object -- "$f" 2>/dev/null)"; else printf '%s\037gone\n' "$f"; fi; done`
}

// Summary reviews one folder against its baseline.
func (st *Store) Summary(mc *machines.Machine, dir string) Summary {
	sum := Summary{At: time.Now(), Commits: []Commit{}, Files: []File{}}
	base, hasBase := st.baseline(mc.Cfg.ID, dir)
	if hasBase {
		b := base
		sum.Baseline = &b
	}
	out, err := mc.Shell(summaryScript(dir, base.Head), 60*time.Second)
	if err != nil {
		sum.Error = err.Error()
		return sum
	}
	sections := strings.Split(out, "\x1e\n")
	if len(sections) < 5 || strings.TrimSpace(sections[0]) == "" {
		return sum // not a git checkout
	}
	head := strings.Split(strings.TrimRight(sections[0], "\n"), "\n")
	sum.Repo = head[0]
	if len(head) > 1 {
		sum.Branch = head[1]
	}
	if len(head) > 2 {
		sum.Head = head[2]
	}
	for _, line := range strings.Split(strings.TrimRight(sections[1], "\n"), "\n") {
		f := strings.Split(line, "\x1f")
		if len(f) == 3 {
			when, _ := strconv.ParseInt(f[2], 10, 64)
			sum.Commits = append(sum.Commits, Commit{SHA: f[0], Subject: f[1], When: when})
		}
	}
	hashes := map[string]string{}
	for _, line := range strings.Split(strings.TrimRight(sections[4], "\n"), "\n") {
		if p, h, ok := strings.Cut(line, "\x1f"); ok {
			hashes[p] = h
		}
	}
	fresh := func(p string) bool {
		if !hasBase {
			return true
		}
		if old, ok := base.Files[p]; ok {
			return hashes[p] != old
		}
		return true
	}
	// a commit is seen when every blob it brought is the one reviewed
	if hasBase && len(sections) > 5 {
		seen := map[string]bool{}
		var sha string
		var all, any bool
		flush := func() {
			if sha != "" && any && all {
				seen[sha] = true
			}
		}
		for _, line := range strings.Split(strings.TrimRight(sections[5], "\n"), "\n") {
			if line == "" {
				continue
			}
			if !strings.HasPrefix(line, "\x1f") {
				flush()
				sha, all, any = line, true, false
				continue
			}
			// :100644 100644 <old> <new> M\tpath
			meta, path, ok := strings.Cut(line[1:], "\t")
			f := strings.Fields(meta)
			if !ok || len(f) < 4 {
				continue
			}
			any = true
			if old, ok := base.Files[path]; !ok || old != f[3] {
				all = false
			}
		}
		flush()
		for i := range sum.Commits {
			for full := range seen {
				if strings.HasPrefix(full, sum.Commits[i].SHA) {
					sum.Commits[i].Seen = true
				}
			}
		}
	}
	for _, line := range strings.Split(strings.TrimRight(sections[2], "\n"), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			continue
		}
		added, _ := strconv.Atoi(f[0])
		removed, _ := strconv.Atoi(f[1])
		sum.Files = append(sum.Files, File{Path: f[2], Added: added, Removed: removed, Fresh: fresh(f[2])})
	}
	for _, p := range strings.Split(strings.TrimRight(sections[3], "\n"), "\n") {
		if p = strings.TrimSpace(p); p != "" {
			sum.Files = append(sum.Files, File{Path: p, Untracked: true, Fresh: fresh(p)})
		}
	}
	sort.Slice(sum.Files, func(i, j int) bool {
		if sum.Files[i].Fresh != sum.Files[j].Fresh {
			return sum.Files[i].Fresh
		}
		return sum.Files[i].Path < sum.Files[j].Path
	})
	for _, f := range sum.Files {
		if f.Fresh {
			sum.Fresh++
		}
	}
	st.setBadge(key(mc.Cfg.ID, dir), base, Badge{Changed: len(sum.Files), Fresh: sum.Fresh})
	return sum
}

// setBadge keeps a count computed against a baseline — unless the baseline
// moved meanwhile ("mark reviewed" during a sweep that takes seconds over
// ssh): a stale count would bring a cleared badge back.
func (st *Store) setBadge(k string, against Baseline, b Badge) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if !st.baselines[k].At.Equal(against.At) {
		return
	}
	st.badges[k] = b
}

// Diff is one file's change since the viewer looked: from the blob the
// baseline recorded for it when there is one (so a commit of reviewed work
// and the edits after it show only the edits), else against the baseline
// commit (HEAD without one); an untracked file comes back whole.
func (st *Store) Diff(mc *machines.Machine, dir, path string) (string, error) {
	base, _ := st.baseline(mc.Cfg.ID, dir)
	q := sshx.Quote(path)
	seen := base.Files[path]
	if seen != "" && seen != "gone" && hexRe.MatchString(seen) {
		// the seen blob, written out, against the file as it is now; the
		// temp file's name is reported first and taken out of the headers
		script := `cd ` + sshx.Quote(dir) + ` 2>/dev/null || exit 0; t=$(mktemp "${TMPDIR:-/tmp}/remuda-seen.XXXXXX") || exit 0; printf '%s\n' "$t"; ` +
			`if git cat-file -p ` + seen + ` > "$t" 2>/dev/null; then git -c color.ui=never -c core.quotepath=off diff --no-index -- "$t" ` + q + ` | head -c 600000; fi; rm -f "$t"; exit 0`
		out, err := mc.Shell(script, 60*time.Second)
		if err != nil {
			return out, err
		}
		tmp, rest, ok := strings.Cut(out, "\n")
		if !ok {
			return "", nil
		}
		return strings.ReplaceAll(rest, "a"+tmp, "a/"+path), nil
	}
	script := `cd ` + sshx.Quote(dir) + ` 2>/dev/null || exit 0; base=` + sshx.Quote(base.Head) + `; ` +
		`if [ -n "$base" ] && git cat-file -e "$base^{commit}" 2>/dev/null; then :; else base=HEAD; fi; ` +
		`if git ls-files --error-unmatch -- ` + q + ` >/dev/null 2>&1 || git cat-file -e "$base:./"` + q + ` 2>/dev/null; then ` +
		`git -c color.ui=never -c core.quotepath=off diff --relative "$base" -- ` + q + ` | head -c 600000; ` +
		`else git -c color.ui=never diff --no-index /dev/null ` + q + ` | head -c 600000; fi; exit 0`
	return mc.Shell(script, 60*time.Second)
}

var hexRe = regexp.MustCompile(`^[0-9a-f]{40,64}$`)

// MarkSeen sets the folder's baseline to now: HEAD, and the blob of every
// file the review showed — changed since the previous baseline's commit
// (committed meanwhile or not) and untracked. A later commit of that very
// content, an amend or a rebase then brings nothing new.
func (st *Store) MarkSeen(mc *machines.Machine, dir string) (Baseline, error) {
	prev, _ := st.baseline(mc.Cfg.ID, dir)
	script := `cd ` + sshx.Quote(dir) + ` 2>/dev/null || exit 0; git rev-parse HEAD 2>/dev/null; printf '\036\n'; ` +
		`base=` + sshx.Quote(prev.Head) + `; if [ -n "$base" ] && git cat-file -e "$base^{commit}" 2>/dev/null; then :; else base=HEAD; fi; ` + hashLoop("$base")
	out, err := mc.Shell(script, 60*time.Second)
	if err != nil {
		return Baseline{}, err
	}
	head, rest, _ := strings.Cut(out, "\x1e\n")
	b := Baseline{Head: strings.TrimSpace(head), Files: map[string]string{}, At: time.Now()}
	for _, line := range strings.Split(strings.TrimRight(rest, "\n"), "\n") {
		if p, h, ok := strings.Cut(line, "\x1f"); ok {
			b.Files[p] = h
		}
	}
	st.mu.Lock()
	st.baselines[key(mc.Cfg.ID, dir)] = b
	st.badges[key(mc.Cfg.ID, dir)] = Badge{}
	err = st.save()
	st.mu.Unlock()
	if err != nil {
		return b, fmt.Errorf("the baseline could not be kept: %w", err)
	}
	return b, nil
}

// Badges refreshes the counts for many folders of one machine in a single
// round trip (for the roster): changed files vs each folder's baseline,
// and how many of those moved since.
func (st *Store) Badges(mc *machines.Machine, dirs []string) {
	if len(dirs) == 0 {
		return
	}
	var sb strings.Builder
	against := map[string]Baseline{}
	for _, d := range dirs {
		base, _ := st.baseline(mc.Cfg.ID, d)
		against[d] = base
		sb.WriteString(`( cd ` + sshx.Quote(d) + ` 2>/dev/null || exit 0; git rev-parse --show-toplevel >/dev/null 2>&1 || exit 0; base=` + sshx.Quote(base.Head) + `; ` +
			`if [ -n "$base" ] && git cat-file -e "$base^{commit}" 2>/dev/null; then :; else base=HEAD; fi; ` +
			`printf '%s\036' ` + sshx.Quote(d) + `; ` + hashLoop("$base") + `; printf '\035\n' ); `)
	}
	out, err := mc.Shell(sb.String(), 90*time.Second)
	if err != nil {
		return
	}
	for _, rec := range strings.Split(out, "\x1d\n") {
		dir, body, ok := strings.Cut(rec, "\x1e")
		if !ok {
			continue
		}
		base, ok := against[dir]
		if !ok {
			continue
		}
		hasBase := !base.At.IsZero()
		badge := Badge{}
		for _, line := range strings.Split(strings.TrimRight(body, "\n"), "\n") {
			p, h, ok := strings.Cut(line, "\x1f")
			if !ok {
				continue
			}
			badge.Changed++
			if old, had := base.Files[p]; !hasBase || !had || old != h {
				badge.Fresh++
			}
		}
		st.setBadge(key(mc.Cfg.ID, dir), base, badge)
	}
}
