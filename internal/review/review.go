// Package review answers "what changed here since I last looked": a folder's
// uncommitted work against HEAD, the commits since the viewer's baseline, and
// the verdict the agent was told. Nothing is written into the repository.
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
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/state"
)

const baselineFile = "review.json"

// Verdicts a review can end with; "" is a look without a word to the agent.
const (
	VerdictApproved = "approved"
	VerdictChanges  = "changes"
)

// VerdictOK reports whether verdict is one of the three.
func VerdictOK(verdict string) bool {
	return verdict == "" || verdict == VerdictApproved || verdict == VerdictChanges
}

// Baseline is where the viewer last left off in a folder.
type Baseline struct {
	Head    string            `json:"head"`
	Blobs   map[string]string `json:"files"` // blob hash by path of every file uncommitted then; "gone" for a deleted one
	Verdict string            `json:"verdict,omitempty"`
	At      time.Time         `json:"at"`
}

// Commit is one commit made since the baseline.
type Commit struct {
	SHA      string `json:"sha"`
	Subject  string `json:"subject"`
	WhenUnix int64  `json:"when"`
	Seen     bool   `json:"seen,omitempty"` // every blob it brought is one the viewer reviewed
}

// File is one uncommitted file: changed against HEAD, or untracked.
type File struct {
	Path      string `json:"path"`
	Added     int    `json:"added"`
	Removed   int    `json:"removed"`
	Untracked bool   `json:"untracked,omitempty"`
	Fresh     bool   `json:"fresh"` // changed since the viewer looked
}

// Summary is the review of one folder.
type Summary struct {
	Repo     string    `json:"repo,omitempty"`
	Branch   string    `json:"branch,omitempty"`
	Head     string    `json:"head,omitempty"`
	Baseline *Baseline `json:"baseline,omitempty"`
	Commits  []Commit  `json:"commits"`
	Files    []File    `json:"files"`
	Fresh    int       `json:"fresh"`
	Error    string    `json:"error,omitempty"`
	At       time.Time `json:"at"`
}

// Badge is the roster's count for one workspace.
type Badge struct {
	Changed int `json:"changed"`
	Fresh   int `json:"fresh"`
}

// Store keeps every folder's baseline on disk and the roster's badges in memory.
type Store struct {
	dir string

	mu        sync.Mutex
	baselines map[string]Baseline // by machine|folder
	badges    map[string]Badge
}

func key(machineID, dir string) string { return machineID + "|" + dir }

// New loads the baselines kept in dir.
func New(dir string) *Store {
	store := &Store{dir: dir, baselines: map[string]Baseline{}, badges: map[string]Badge{}}
	if err := state.LoadJSON(dir, baselineFile, &store.baselines); err != nil {
		log.Printf("%s is unreadable (%v): every folder counts as never reviewed", baselineFile, err)
	}
	return store
}

func (store *Store) save() error {
	return state.SaveJSON(store.dir, baselineFile, store.baselines)
}

func (store *Store) baseline(machineID, dir string) (Baseline, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	baseline, ok := store.baselines[key(machineID, dir)]
	return baseline, ok
}

// Badge is the cached count for the roster.
func (store *Store) Badge(machineID, dir string) (Badge, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	badge, ok := store.badges[key(machineID, dir)]
	return badge, ok
}

// summaryScript gathers everything Summary needs in one shell, sections separated by a line of \x1e and fields by \x1f:
// repo/branch/HEAD, the first-parent commits since the baseline, the numstat against HEAD, the untracked files, every uncommitted file's blob, and per commit the blobs it left (none for a merge).
func summaryScript(dir, baselineHead string) string {
	return script.InRepo(dir) +
		`printf '%s\n' "$root"; git rev-parse --abbrev-ref HEAD 2>/dev/null; git rev-parse HEAD 2>/dev/null; printf '\036\n'; ` +
		`base=` + script.Quote(baselineHead) + `; if [ -n "$base" ] && git cat-file -e "$base^{commit}" 2>/dev/null; then :; else base=HEAD; fi; ` +
		`if [ "$base" != HEAD ]; then git log --first-parent --format='%h%x1f%s%x1f%ct' "$base"..HEAD 2>/dev/null; fi; printf '\036\n'; ` +
		`git -c core.quotepath=off diff --no-renames --relative --numstat HEAD 2>/dev/null; printf '\036\n'; ` +
		`git -c core.quotepath=off ls-files --others --exclude-standard 2>/dev/null; printf '\036\n'; ` +
		blobsScript() + `; printf '\036\n'; ` +
		`if [ "$base" != HEAD ]; then for c in $(git rev-list --first-parent "$base"..HEAD 2>/dev/null); do printf '%s\n' "$c"; git -c core.quotepath=off diff-tree --no-commit-id -r --relative --no-renames "$c" 2>/dev/null | while IFS= read -r l; do printf '\037%s\n' "$l"; done; done; fi`
}

// blobsScript prints "<path>\x1f<blob hash>" per uncommitted file, one per line, "gone" for a deleted one.
// Paths are relative to the folder; a rename is a deletion plus an addition.
func blobsScript() string {
	return `{ git -c core.quotepath=off diff --no-renames --relative --name-only HEAD 2>/dev/null; git -c core.quotepath=off ls-files --others --exclude-standard 2>/dev/null; } | ` +
		`while IFS= read -r f; do if [ -f "$f" ]; then printf '%s\037%s\n' "$f" "$(git hash-object -- "$f" 2>/dev/null)"; else printf '%s\037gone\n' "$f"; fi; done`
}

// Summary reviews one folder against its baseline.
func (store *Store) Summary(machine *machines.Machine, dir string) Summary {
	summary := Summary{At: time.Now(), Commits: []Commit{}, Files: []File{}}
	baseline, hasBaseline := store.baseline(machine.Cfg.ID, dir)
	if hasBaseline {
		copied := baseline
		summary.Baseline = &copied
	}
	output, err := machine.Shell(summaryScript(dir, baseline.Head), 60*time.Second)
	if err != nil {
		summary.Error = err.Error()
		return summary
	}
	sections := strings.Split(output, "\x1e\n")
	if len(sections) < 5 || strings.TrimSpace(sections[0]) == "" {
		return summary
	}
	headLines := strings.Split(strings.TrimRight(sections[0], "\n"), "\n")
	summary.Repo = headLines[0]
	if len(headLines) > 1 {
		summary.Branch = headLines[1]
	}
	if len(headLines) > 2 {
		summary.Head = headLines[2]
	}
	for _, line := range strings.Split(strings.TrimRight(sections[1], "\n"), "\n") {
		fields := strings.Split(line, "\x1f")
		if len(fields) == 3 {
			whenUnix, _ := strconv.ParseInt(fields[2], 10, 64)
			summary.Commits = append(summary.Commits, Commit{SHA: fields[0], Subject: fields[1], WhenUnix: whenUnix})
		}
	}
	blobs := map[string]string{}
	for _, line := range strings.Split(strings.TrimRight(sections[4], "\n"), "\n") {
		if path, blob, ok := strings.Cut(line, "\x1f"); ok {
			blobs[path] = blob
		}
	}
	isFresh := func(path string) bool {
		if !hasBaseline {
			return true
		}
		if reviewed, ok := baseline.Blobs[path]; ok {
			return blobs[path] != reviewed
		}
		return true
	}
	if hasBaseline && len(sections) > 5 {
		seenCommits := map[string]bool{}
		var commitSHA string
		var allReviewed, anyFile bool
		closeCommit := func() {
			if commitSHA != "" && anyFile && allReviewed {
				seenCommits[commitSHA] = true
			}
		}
		for _, line := range strings.Split(strings.TrimRight(sections[5], "\n"), "\n") {
			if line == "" {
				continue
			}
			if !strings.HasPrefix(line, "\x1f") {
				closeCommit()
				commitSHA, allReviewed, anyFile = line, true, false
				continue
			}
			// a diff-tree line: ":100644 100644 <old blob> <new blob> M\t<path>"
			modes, path, ok := strings.Cut(line[1:], "\t")
			fields := strings.Fields(modes)
			if !ok || len(fields) < 4 {
				continue
			}
			anyFile = true
			if reviewed, ok := baseline.Blobs[path]; !ok || reviewed != fields[3] {
				allReviewed = false
			}
		}
		closeCommit()
		for i := range summary.Commits {
			for fullSHA := range seenCommits {
				if strings.HasPrefix(fullSHA, summary.Commits[i].SHA) {
					summary.Commits[i].Seen = true
				}
			}
		}
	}
	for _, line := range strings.Split(strings.TrimRight(sections[2], "\n"), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			continue
		}
		added, _ := strconv.Atoi(fields[0])
		removed, _ := strconv.Atoi(fields[1])
		summary.Files = append(summary.Files, File{Path: fields[2], Added: added, Removed: removed, Fresh: isFresh(fields[2])})
	}
	for _, path := range strings.Split(strings.TrimRight(sections[3], "\n"), "\n") {
		if path = strings.TrimSpace(path); path != "" {
			summary.Files = append(summary.Files, File{Path: path, Untracked: true, Fresh: isFresh(path)})
		}
	}
	sort.Slice(summary.Files, func(i, j int) bool {
		if summary.Files[i].Fresh != summary.Files[j].Fresh {
			return summary.Files[i].Fresh
		}
		return summary.Files[i].Path < summary.Files[j].Path
	})
	for _, file := range summary.Files {
		if file.Fresh {
			summary.Fresh++
		}
	}
	store.setBadge(key(machine.Cfg.ID, dir), baseline, Badge{Changed: len(summary.Files), Fresh: summary.Fresh})
	return summary
}

// setBadge keeps a badge computed against a baseline, unless the baseline
// moved meanwhile: a stale count would bring a cleared badge back.
func (store *Store) setBadge(storeKey string, against Baseline, badge Badge) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if !store.baselines[storeKey].At.Equal(against.At) {
		return
	}
	store.badges[storeKey] = badge
}

// Diff is one file's unified diff since the viewer looked: from the blob
// the baseline recorded for it, else against HEAD; an untracked file whole.
func (store *Store) Diff(machine *machines.Machine, dir, path string) (string, error) {
	baseline, _ := store.baseline(machine.Cfg.ID, dir)
	quotedPath := script.Quote(path)
	reviewedBlob := baseline.Blobs[path]
	if reviewedBlob != "" && reviewedBlob != "gone" && hexRe.MatchString(reviewedBlob) {
		// the script prints the temp file's name first; the diff headers carry it
		fromBlob := script.InDir(dir) + `t=$(mktemp "${TMPDIR:-/tmp}/remuda-seen.XXXXXX") || exit 0; printf '%s\n' "$t"; ` +
			`if git cat-file -p ` + reviewedBlob + ` > "$t" 2>/dev/null; then git -c color.ui=never -c core.quotepath=off diff --no-index -- "$t" ` + quotedPath + ` | head -c 600000; fi; rm -f "$t"; exit 0`
		output, err := machine.Shell(fromBlob, 60*time.Second)
		if err != nil {
			return output, err
		}
		tempFile, diff, ok := strings.Cut(output, "\n")
		if !ok {
			return "", nil
		}
		return strings.ReplaceAll(diff, "a"+tempFile, "a/"+path), nil
	}
	fromHead := script.InDir(dir) +
		`if git ls-files --error-unmatch -- ` + quotedPath + ` >/dev/null 2>&1 || git cat-file -e "HEAD:./"` + quotedPath + ` 2>/dev/null; then ` +
		`git -c color.ui=never -c core.quotepath=off diff --relative HEAD -- ` + quotedPath + ` | head -c 600000; ` +
		`else git -c color.ui=never diff --no-index -- /dev/null ` + quotedPath + ` | head -c 600000; fi; exit 0`
	return machine.Shell(fromHead, 60*time.Second)
}

var hexRe = regexp.MustCompile(`^[0-9a-f]{40,64}$`)

// MarkSeen moves the folder's baseline to now: HEAD, the blob of every
// uncommitted file, and the verdict the agent was told.
func (store *Store) MarkSeen(machine *machines.Machine, dir, verdict string) (Baseline, error) {
	if !VerdictOK(verdict) {
		return Baseline{}, fmt.Errorf("no such verdict: %q", verdict)
	}
	snapshot := script.InDir(dir) + `git rev-parse HEAD 2>/dev/null; printf '\036\n'; ` + blobsScript()
	output, err := machine.Shell(snapshot, 60*time.Second)
	if err != nil {
		return Baseline{}, err
	}
	head, blobLines, _ := strings.Cut(output, "\x1e\n")
	baseline := Baseline{Head: strings.TrimSpace(head), Blobs: map[string]string{}, Verdict: verdict, At: time.Now()}
	for _, line := range strings.Split(strings.TrimRight(blobLines, "\n"), "\n") {
		if path, blob, ok := strings.Cut(line, "\x1f"); ok {
			baseline.Blobs[path] = blob
		}
	}
	store.mu.Lock()
	store.baselines[key(machine.Cfg.ID, dir)] = baseline
	store.badges[key(machine.Cfg.ID, dir)] = Badge{}
	err = store.save()
	store.mu.Unlock()
	if err != nil {
		return baseline, fmt.Errorf("the baseline could not be kept: %w", err)
	}
	return baseline, nil
}

// Badges refreshes the roster's counts for many folders of one machine in
// a single round trip.
func (store *Store) Badges(machine *machines.Machine, dirs []string) {
	if len(dirs) == 0 {
		return
	}
	var sweep strings.Builder
	baselines := map[string]Baseline{}
	for _, dir := range dirs {
		baseline, _ := store.baseline(machine.Cfg.ID, dir)
		baselines[dir] = baseline
		sweep.WriteString(`( ` + script.InRepo(dir) +
			`printf '%s\036' ` + script.Quote(dir) + `; ` + blobsScript() + `; printf '\035\n' ); `)
	}
	output, err := machine.Shell(sweep.String(), 90*time.Second)
	if err != nil {
		return
	}
	for _, record := range strings.Split(output, "\x1d\n") {
		dir, blobLines, ok := strings.Cut(record, "\x1e")
		if !ok {
			continue
		}
		baseline, ok := baselines[dir]
		if !ok {
			continue
		}
		hasBaseline := !baseline.At.IsZero()
		badge := Badge{}
		for _, line := range strings.Split(strings.TrimRight(blobLines, "\n"), "\n") {
			path, blob, ok := strings.Cut(line, "\x1f")
			if !ok {
				continue
			}
			badge.Changed++
			if reviewed, wasReviewed := baseline.Blobs[path]; !hasBaseline || !wasReviewed || reviewed != blob {
				badge.Fresh++
			}
		}
		store.setBadge(key(machine.Cfg.ID, dir), baseline, badge)
	}
}
