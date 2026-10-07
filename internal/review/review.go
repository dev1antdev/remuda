// Package review answers "what changed here since I last looked": for a
// workspace's folder, the commits and the working-tree changes since a
// baseline the viewer set the last time they reviewed it. The baseline is
// the commit that was HEAD then plus the content hashes of the files that
// were already dirty then, so a file reads as new only when its content
// moved since. Everything is one shell script per question, run locally or
// over ssh; nothing is written into the repository.
package review

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/herdr-ui/internal/machines"
	"github.com/dev1antdev/herdr-ui/internal/sshx"
	"github.com/dev1antdev/herdr-ui/internal/state"
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
	When    int64  `json:"when"` // unix seconds
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
	if b, err := os.ReadFile(filepath.Join(dir, baselineFile)); err == nil {
		_ = json.Unmarshal(b, &st.baselines)
	}
	return st
}

func (st *Store) save() {
	b, _ := json.MarshalIndent(st.baselines, "", "  ")
	_ = state.WriteFile(filepath.Join(st.dir, baselineFile), b)
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
		`git -c core.quotepath=off diff --numstat "$base" 2>/dev/null; printf '\036\n'; ` +
		`git -c core.quotepath=off ls-files --others --exclude-standard 2>/dev/null; printf '\036\n'; ` +
		`for f in $(git -c core.quotepath=off diff --name-only "$base" 2>/dev/null; git -c core.quotepath=off ls-files --others --exclude-standard 2>/dev/null); do ` +
		`[ -f "$f" ] && printf '%s\037%s\n' "$f" "$(git hash-object "$f" 2>/dev/null)"; done`
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
	st.mu.Lock()
	st.badges[key(mc.Cfg.ID, dir)] = Badge{Changed: len(sum.Files), Fresh: sum.Fresh}
	st.mu.Unlock()
	return sum
}

// Diff is one file's change against the baseline commit (HEAD without one);
// an untracked file comes back whole, as additions.
func (st *Store) Diff(mc *machines.Machine, dir, path string) (string, error) {
	base, _ := st.baseline(mc.Cfg.ID, dir)
	q := sshx.Quote(path)
	script := `cd ` + sshx.Quote(dir) + ` 2>/dev/null || exit 0; base=` + sshx.Quote(base.Head) + `; ` +
		`if [ -n "$base" ] && git cat-file -e "$base^{commit}" 2>/dev/null; then :; else base=HEAD; fi; ` +
		`if git ls-files --error-unmatch ` + q + ` >/dev/null 2>&1 || git cat-file -e "$base:` + strings.Trim(q, "'") + `" 2>/dev/null; then ` +
		`git -c color.ui=never -c core.quotepath=off diff "$base" -- ` + q + ` | head -c 600000; ` +
		`else git -c color.ui=never diff --no-index /dev/null ` + q + ` | head -c 600000; fi; exit 0`
	return mc.Shell(script, 60*time.Second)
}

// MarkSeen sets the folder's baseline to now: HEAD, and the hash of every
// file that is changed or untracked at this moment.
func (st *Store) MarkSeen(mc *machines.Machine, dir string) (Baseline, error) {
	script := `cd ` + sshx.Quote(dir) + ` 2>/dev/null || exit 0; git rev-parse HEAD 2>/dev/null; printf '\036\n'; ` +
		`for f in $(git -c core.quotepath=off diff --name-only HEAD 2>/dev/null; git -c core.quotepath=off ls-files --others --exclude-standard 2>/dev/null); do ` +
		`[ -f "$f" ] && printf '%s\037%s\n' "$f" "$(git hash-object "$f" 2>/dev/null)"; done`
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
	st.save()
	st.mu.Unlock()
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
	for _, d := range dirs {
		base, _ := st.baseline(mc.Cfg.ID, d)
		sb.WriteString(`( cd ` + sshx.Quote(d) + ` 2>/dev/null || exit 0; git rev-parse --show-toplevel >/dev/null 2>&1 || exit 0; base=` + sshx.Quote(base.Head) + `; ` +
			`if [ -n "$base" ] && git cat-file -e "$base^{commit}" 2>/dev/null; then :; else base=HEAD; fi; ` +
			`printf '%s\036' ` + sshx.Quote(d) + `; for f in $(git -c core.quotepath=off diff --name-only "$base" 2>/dev/null; git -c core.quotepath=off ls-files --others --exclude-standard 2>/dev/null); do ` +
			`[ -f "$f" ] && printf '%s\037%s\n' "$f" "$(git hash-object "$f" 2>/dev/null)"; done; printf '\035\n' ); `)
	}
	out, err := mc.Shell(sb.String(), 90*time.Second)
	if err != nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, rec := range strings.Split(out, "\x1d\n") {
		dir, body, ok := strings.Cut(rec, "\x1e")
		if !ok {
			continue
		}
		base, hasBase := st.baselines[key(mc.Cfg.ID, dir)]
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
		st.badges[key(mc.Cfg.ID, dir)] = badge
	}
}
