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

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
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
	Ready   time.Time         `json:"ready,omitzero"` // when the agent said the rejected work is ready for another look
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
// maxCommits is how many commits since the baseline a summary lists; the rest are counted in CommitsTotal.
const maxCommits = 300

type Summary struct {
	Repo         string          `json:"repo,omitempty"`
	Branch       string          `json:"branch,omitempty"`
	Head         string          `json:"head,omitempty"`
	Baseline     *Baseline       `json:"baseline,omitempty"`
	Commits      []Commit        `json:"commits"`
	CommitsTotal int             `json:"commits_total,omitempty"` // every commit since the baseline, when more than the listed ones
	Files        []File          `json:"files"`
	Fresh        int             `json:"fresh"`
	Settings     []SettingsAudit `json:"settings,omitempty"` // Claude Code's settings files there, audited for secrets and tracking
	Error        string          `json:"error,omitempty"`
	At           time.Time       `json:"at"`
}

// Badge is the roster's count for one workspace.
type Badge struct {
	Changed int `json:"changed"`
	Fresh   int `json:"fresh"`
}

// Store keeps every folder's baseline on disk and the roster's badges and branches in memory.
type Store struct {
	dir string

	mu        sync.Mutex
	baselines map[string]Baseline // by machine|folder
	badges    map[string]Badge
	branches  map[string]string     // the branch each folder has checked out, a short hash when detached
	roots     map[string]string     // the repository's main folder for each folder, a worktree's too
	sweeps    map[string]badgeSweep // by machine
	history   []Verdict             // every verdict given, the oldest first, at most HistoryKeep (review-verdicts.json)
	works     map[string]work       // each folder's uncommitted work as the sweeps weigh it, for the queue's risk
	remotes   map[string]string     // each folder's origin, which every checkout of one repository shares
	scans     map[string]scanNote   // the last secret check of each folder
	syncs     map[string]Sync       // each folder's branch against its upstream, as of the last fetch
}

// badgeSweep is a machine's last Badges run: when it began, and whether it is still going.
type badgeSweep struct {
	at   time.Time
	busy bool
}

func key(machineID, dir string) string { return machineID + "|" + dir }

// New loads the baselines kept in dir.
func New(dir string) *Store {
	store := &Store{dir: dir, baselines: map[string]Baseline{}, badges: map[string]Badge{}, branches: map[string]string{}, roots: map[string]string{}, sweeps: map[string]badgeSweep{},
		works: map[string]work{}, remotes: map[string]string{}, scans: map[string]scanNote{}, syncs: map[string]Sync{}}
	if err := state.LoadJSON(dir, baselineFile, &store.baselines); err != nil {
		log.Printf("%s is unreadable (%v): every folder counts as never reviewed", baselineFile, err)
	}
	store.loadHistory()
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

// Branch is the branch the folder has checked out as the last look read it, "" outside git or before the first sweep.
func (store *Store) Branch(machineID, dir string) string {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.branches[key(machineID, dir)]
}

// Root is the main folder of the repository the folder belongs to — for a linked worktree, its repository's — as the
// last sweep read it; "" outside git or before the first sweep.
func (store *Store) Root(machineID, dir string) string {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.roots[key(machineID, dir)]
}

func (store *Store) setRoot(storeKey, root string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if root == "" {
		delete(store.roots, storeKey)
		return
	}
	store.roots[storeKey] = root
}

func (store *Store) setBranch(storeKey, branch string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if branch == "" || branch == "HEAD" {
		delete(store.branches, storeKey)
		return
	}
	store.branches[storeKey] = branch
}

// summaryScript gathers everything Summary needs in one shell, sections separated by a line of \x1e and fields by \x1f:
// repo/branch/HEAD, the newest maxCommits first-parent commits since the baseline, the numstat against HEAD, the untracked files, every uncommitted file's blob, per commit the blobs it left (none for a merge) — one git log, never a process per commit: a repository pulled after weeks has hundreds — how many commits there are in all, and Claude Code's settings files there (settingsScript).
func summaryScript(dir, baselineHead string) string {
	return script.InRepo(dir) +
		`printf '%s\n' "$root"; git rev-parse --abbrev-ref HEAD 2>/dev/null; git rev-parse HEAD 2>/dev/null; printf '\036\n'; ` +
		`base=` + script.Quote(baselineHead) + `; if [ -n "$base" ] && git cat-file -e "$base^{commit}" 2>/dev/null; then :; else base=HEAD; fi; ` +
		`if [ "$base" != HEAD ]; then git log --first-parent -n ` + strconv.Itoa(maxCommits) + ` --format='%h%x1f%s%x1f%ct' "$base"..HEAD 2>/dev/null; fi; printf '\036\n'; ` +
		`git -c core.quotepath=off diff --no-renames --relative --numstat HEAD 2>/dev/null; printf '\036\n'; ` +
		`git -c core.quotepath=off ls-files --others --exclude-standard 2>/dev/null; printf '\036\n'; ` +
		blobsScript() + `; printf '\036\n'; ` +
		`if [ "$base" != HEAD ]; then git -c core.quotepath=off log --first-parent -n ` + strconv.Itoa(maxCommits) + ` --raw --no-renames --no-abbrev --relative --format='%H' "$base"..HEAD 2>/dev/null; fi; printf '\036\n'; ` +
		`if [ "$base" != HEAD ]; then git rev-list --count --first-parent "$base"..HEAD 2>/dev/null; fi; printf '\036\n'; ` +
		settingsScript
}

// blobsScript prints "<path>\x1f<blob hash>" per uncommitted file, one per line, "gone" for a deleted one.
// Paths are relative to the folder; a rename is a deletion plus an addition.
func blobsScript() string {
	return `{ git -c core.quotepath=off diff --no-renames --relative --name-only HEAD 2>/dev/null; git -c core.quotepath=off ls-files --others --exclude-standard 2>/dev/null; } | ` +
		`{ t=$(mktemp 2>/dev/null) || t=/tmp/remuda-blobs.$$; : > "$t"; while IFS= read -r f; do if [ -f "$f" ]; then printf '%s\n' "$f" >> "$t"; else printf '%s\037gone\n' "$f"; fi; done; ` +
		`if [ -s "$t" ]; then git -c core.quotepath=off hash-object --stdin-paths < "$t" 2>/dev/null | paste -d "$(printf '\037')" "$t" -; fi; rm -f "$t"; }`
}

// Summary reviews one folder against its baseline.
func (store *Store) Summary(host shell.Host, dir string) Summary {
	summary := Summary{At: time.Now(), Commits: []Commit{}, Files: []File{}}
	baseline, hasBaseline := store.baseline(host.ID(), dir)
	if hasBaseline {
		copied := baseline
		summary.Baseline = &copied
	}
	output, err := shell.Text(host, summaryScript(dir, baseline.Head), 60*time.Second)
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
		if summary.Branch != "HEAD" { // a detached HEAD keeps the short hash the sweep read for the sidebar
			store.setBranch(key(host.ID(), dir), summary.Branch)
		}
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
			if !strings.HasPrefix(line, ":") {
				closeCommit()
				commitSHA, allReviewed, anyFile = line, true, false
				continue
			}
			// a raw line of git log: ":100644 100644 <old blob> <new blob> M\t<path>"
			modes, path, ok := strings.Cut(line, "\t")
			fields := strings.Fields(modes)
			if !ok || len(fields) < 4 {
				continue
			}
			anyFile = true
			left := fields[3]
			if strings.Trim(left, "0") == "" { // a deletion leaves the null blob, which a look recorded as gone
				left = "gone"
			}
			if reviewed, ok := baseline.Blobs[path]; !ok || reviewed != left {
				allReviewed = false
			}
		}
		closeCommit()
		if len(sections) > 6 {
			if total, err := strconv.Atoi(strings.TrimSpace(sections[6])); err == nil && total > len(summary.Commits) {
				summary.CommitsTotal = total
			}
		}
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
	if len(sections) > 7 {
		summary.Settings = readSettings(sections[7])
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
	store.setBadge(key(host.ID(), dir), baseline, Badge{Changed: len(summary.Files), Fresh: summary.Fresh})
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
func (store *Store) Diff(host shell.Host, dir, path string) (string, error) {
	baseline, _ := store.baseline(host.ID(), dir)
	quotedPath := script.Quote(path)
	reviewedBlob := baseline.Blobs[path]
	if reviewedBlob != "" && reviewedBlob != "gone" && hexRe.MatchString(reviewedBlob) {
		// the script prints the temp file's name first; the diff headers carry it
		fromBlob := script.InDir(dir) + `t=$(mktemp "${TMPDIR:-/tmp}/remuda-seen.XXXXXX") || exit 0; printf '%s\n' "$t"; ` +
			`p=` + quotedPath + `; [ -e "$p" ] || p=/dev/null; ` + // a reviewed file the agent deleted since: the diff is its removal
			`if git cat-file -p ` + reviewedBlob + ` > "$t" 2>/dev/null; then git -c color.ui=never -c core.quotepath=off diff --no-ext-diff --no-index -- "$t" "$p" | head -c 600000; fi; rm -f "$t"; exit 0`
		output, err := shell.Text(host, fromBlob, 60*time.Second)
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
		`git -c color.ui=never -c core.quotepath=off diff --no-ext-diff --relative HEAD -- ` + quotedPath + ` | head -c 600000; ` +
		`else git -c color.ui=never diff --no-ext-diff --no-index -- /dev/null ` + quotedPath + ` | head -c 600000; fi; exit 0`
	return shell.Text(host, fromHead, 60*time.Second)
}

var hexRe = regexp.MustCompile(`^[0-9a-f]{40,64}$`)

// Snapshot is the folder's HEAD and the blob of every uncommitted file ("gone" for a deleted one): what a look saw.
func Snapshot(host shell.Host, dir string) (string, map[string]string, error) {
	snapshot := script.InDir(dir) + `git rev-parse HEAD 2>/dev/null; printf '\036\n'; ` + blobsScript()
	output, err := shell.Text(host, snapshot, 60*time.Second)
	if err != nil {
		return "", nil, err
	}
	head, blobLines, _ := strings.Cut(output, "\x1e\n")
	blobs := map[string]string{}
	for _, line := range strings.Split(strings.TrimRight(blobLines, "\n"), "\n") {
		if path, blob, ok := strings.Cut(line, "\x1f"); ok {
			blobs[path] = blob
		}
	}
	return strings.TrimSpace(head), blobs, nil
}

// MarkSeen moves the folder's baseline to now: HEAD, the blob of every
// uncommitted file, and the verdict the agent was told, which the history keeps too.
func (store *Store) MarkSeen(host shell.Host, dir, verdict string) (Baseline, error) {
	if !VerdictOK(verdict) {
		return Baseline{}, fmt.Errorf("no such verdict: %q", verdict)
	}
	head, blobs, err := Snapshot(host, dir)
	if err != nil {
		return Baseline{}, err
	}
	baseline := Baseline{Head: head, Blobs: blobs, Verdict: verdict, At: time.Now()}
	store.mu.Lock()
	store.baselines[key(host.ID(), dir)] = baseline
	store.badges[key(host.ID(), dir)] = Badge{}
	err = store.save()
	noted := store.noteVerdict(Verdict{Machine: host.ID(), Dir: dir, Verdict: verdict, At: baseline.At})
	store.mu.Unlock()
	if err != nil {
		return baseline, fmt.Errorf("the baseline could not be kept: %w", err)
	}
	if noted != nil {
		log.Printf("%s: the verdict could not be kept in %s: %v", dir, historyFile, noted)
	}
	return baseline, nil
}

// RefreshBadges runs Badges in the background, a minute apart per machine and never two at once:
// the sweep is seconds of git, over ssh for a remote, and the poll that asked must not wait for it.
func (store *Store) RefreshBadges(host shell.Host, dirs []string) {
	store.mu.Lock()
	sweep := store.sweeps[host.ID()]
	if sweep.busy || time.Since(sweep.at) < time.Minute {
		store.mu.Unlock()
		return
	}
	began := time.Now()
	store.sweeps[host.ID()] = badgeSweep{at: began, busy: true}
	store.mu.Unlock()
	go func() {
		store.Badges(host, dirs)
		store.mu.Lock()
		store.sweeps[host.ID()] = badgeSweep{at: began}
		store.mu.Unlock()
	}()
}

// branchScript prints four lines: the branch checked out, or the short hash of a detached HEAD; the repository's main
// folder (MainFolderScript) — a submodule's git dir sits under its parent's .git/modules, never beside it; the
// address of its origin (remoteScript), "" for none; and the branch against its upstream (syncScript), "" for none.
const branchScript = `b=$({ git symbolic-ref --short -q HEAD || git rev-parse --short HEAD; } 2>/dev/null); printf '%s\n' "$b"; ` +
	`r=$(` + MainFolderScript + `); printf '%s\n' "$r"; ` + remoteScript + `; ` + syncScript

// MainFolderScript prints, run inside a checkout, the repository's main folder: for a linked worktree the parent of the
// common git dir it shares with its repository, else the checkout's own top folder.
const MainFolderScript = `gd=$(cd "$(git rev-parse --git-dir 2>/dev/null)" 2>/dev/null && pwd -P); ` +
	`common=$(cd "$(git rev-parse --git-common-dir 2>/dev/null)" 2>/dev/null && pwd -P); ` +
	`if [ -n "$gd" ] && [ "$gd" = "$common" ]; then (cd "./$(git rev-parse --show-cdup 2>/dev/null)" 2>/dev/null && pwd); ` +
	`elif [ -n "$common" ]; then (cd "$common/.." 2>/dev/null && pwd); fi`

// Badges refreshes the roster's counts and branches for many folders of one machine in a single round trip: per
// folder, the folder, its branch, main folder and remote, the uncommitted blobs, git's line counts and the untracked
// files' size — what the review queue's risk weighs.
func (store *Store) Badges(host shell.Host, dirs []string) {
	if len(dirs) == 0 {
		return
	}
	began := time.Now()
	var sweep strings.Builder
	baselines := map[string]Baseline{}
	for _, dir := range dirs {
		baseline, _ := store.baseline(host.ID(), dir)
		baselines[dir] = baseline
		sweep.WriteString(`( ` + script.InRepo(dir) +
			`printf '%s\036' ` + script.Quote(dir) + `; ` + branchScript + `; printf '\036'; ` + blobsScript() + `; printf '\036\n'; ` +
			numstatScript + `; printf '\036\n'; ` + newBytesScript + `; printf '\035\n' ); `)
	}
	output, err := shell.Text(host, sweep.String(), 90*time.Second)
	if err != nil {
		return
	}
	answered := map[string]bool{}
	for _, record := range strings.Split(output, "\x1d\n") {
		dir, rest, ok := strings.Cut(record, "\x1e")
		if !ok {
			continue
		}
		heads, rest, ok := strings.Cut(rest, "\x1e")
		if !ok {
			continue
		}
		sections := strings.Split(rest, "\x1e\n")
		baseline, ok := baselines[dir]
		if !ok || len(sections) < 3 {
			continue
		}
		answered[dir] = true
		storeKey := key(host.ID(), dir)
		lines := strings.SplitN(strings.TrimRight(heads, "\n"), "\n", 4)
		for len(lines) < 4 {
			lines = append(lines, "")
		}
		store.setBranch(storeKey, strings.TrimSpace(lines[0]))
		store.setRoot(storeKey, rpath.For(host.Home()).Clean(strings.TrimSpace(lines[1]))) // Git Bash says /c/Users/me/app for C:\Users\me\app
		store.setRemote(storeKey, strings.TrimSpace(lines[2]))
		store.setSync(storeKey, lines[3])
		blobLines := sections[0]
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
		store.setBadge(storeKey, baseline, badge)
		profile, fingerprint := readWork(blobLines, sections[1], sections[2])
		store.setWork(storeKey, profile, fingerprint, began)
	}
	for _, dir := range dirs {
		if !answered[dir] {
			store.setBranch(key(host.ID(), dir), "") // no longer a repository: no branch or changes to show
			store.setRoot(key(host.ID(), dir), "")
			store.setSync(key(host.ID(), dir), "")
			store.forgetWork(key(host.ID(), dir))
		}
	}
}
