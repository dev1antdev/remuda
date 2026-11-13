package review

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// BaseView is the branch's whole work against the branch it was cut from: the commits it has over their merge base and
// what the working tree holds beyond them, read only. Base is "" when there is nothing to compare with.
type BaseView struct {
	Base      string   `json:"base"`
	Detected  string   `json:"detected"` // the base remuda found; the person may pin another
	MergeBase string   `json:"merge_base,omitempty"`
	Ahead     int      `json:"ahead"`
	Refs      []string `json:"refs"` // the branches a base may be pinned to, the newest first
	Files     []File   `json:"files"`
}

// baseRead is the base detected for a HEAD, kept until HEAD moves: finding it reads a few branches' histories.
type baseRead struct {
	head     string
	detected string
	refs     []string
}

// trunks are the names tried when no branch HEAD left explains the fork, the usual case: the trunk moved on after the
// branch was cut from it.
var trunks = []string{"origin/main", "origin/master", "origin/next", "origin/develop", "main", "master", "next", "develop"}

// ancestors is how many branches whose tip HEAD holds are weighed: the most recently updated one is almost always it.
const ancestors = 4

// detectScript prints what the fork base is chosen from, a fact a line: HEAD, the branch, the remotes, origin's default
// branch, the branches merged into HEAD with the commits since each, the trunks with their merge base and the commits
// since it — every count capped at 500, so a far candidate costs little — and the newest branches to pin.
var detectScript = `h=$(git rev-parse -q --verify HEAD 2>/dev/null) || exit 0; printf 'head\037%s\n' "$h"; ` +
	`printf 'branch\037%s\n' "$(git symbolic-ref --short -q HEAD 2>/dev/null)"; ` +
	`git remote 2>/dev/null | while IFS= read -r r; do printf 'remote\037%s\n' "$r"; done; ` +
	`o=$(git symbolic-ref --short -q refs/remotes/origin/HEAD 2>/dev/null); [ -n "$o" ] && printf 'originhead\037%s\n' "$o"; ` +
	`git for-each-ref --merged HEAD --sort=-committerdate --count=12 --format='%(refname:short)' refs/heads refs/remotes 2>/dev/null | ` +
	`while IFS= read -r n; do case "$n" in */HEAD) continue ;; esac; printf 'anc\037%s\037%s\n' "$n" "$(git rev-list --count --max-count=500 "$n"..HEAD 2>/dev/null)"; done; ` +
	`for n in $o ` + strings.Join(trunks, " ") + `; do git rev-parse -q --verify "$n^{commit}" >/dev/null 2>&1 || continue; ` +
	`mb=$(git merge-base HEAD "$n" 2>/dev/null) || continue; printf 'trunk\037%s\037%s\037%s\n' "$n" "$mb" "$(git rev-list --count --max-count=500 "$mb"..HEAD 2>/dev/null)"; done; ` +
	`git for-each-ref --sort=-committerdate --count=40 --format='%(refname:short)' refs/heads refs/remotes 2>/dev/null | ` +
	`while IFS= read -r n; do case "$n" in */HEAD) continue ;; esac; printf 'ref\037%s\n' "$n"; done`

// candidate is one branch the fork may have been cut from: how many commits HEAD has made since leaving it.
type candidate struct {
	name     string
	distance int
}

// forkBase picks the base from detectScript's answer the way gitview ranks it: the branch HEAD left most recently —
// the fewest commits since the divergence — ties to the conventional trunk, then by name; a trunk itself is cut from
// nothing and compares with origin's default branch, else the first trunk there is.
func forkBase(answer string) (head, base string, refs []string) {
	var branch, originHead string
	var remotes []string
	var merged, trunkRows []candidate
	present := map[string]bool{}
	for _, line := range script.Lines(answer) {
		fields := script.Fields(line)
		switch {
		case fields[0] == "head" && len(fields) == 2:
			head = fields[1]
		case fields[0] == "branch" && len(fields) == 2:
			branch = fields[1]
		case fields[0] == "remote" && len(fields) == 2:
			remotes = append(remotes, fields[1])
		case fields[0] == "originhead" && len(fields) == 2:
			originHead = fields[1]
		case fields[0] == "anc" && len(fields) == 3:
			merged = append(merged, candidate{fields[1], distance(fields[2])})
		case fields[0] == "trunk" && len(fields) == 4:
			present[fields[1]] = true
			if fields[2] != head {
				trunkRows = append(trunkRows, candidate{fields[1], distance(fields[3])})
			}
		case fields[0] == "ref" && len(fields) == 2:
			refs = append(refs, fields[1])
		}
	}
	own := func(name string) bool {
		if branch == "" {
			return false
		}
		for _, remote := range remotes {
			if name == remote+"/"+branch {
				return true
			}
		}
		return name == branch
	}
	var scored []candidate
	if conventionalRank(branch) == len(conventional) {
		kept := 0
		for _, c := range merged {
			if own(c.name) || kept == ancestors {
				continue
			}
			kept++
			if c.distance > 0 {
				scored = append(scored, c)
			}
		}
		for _, c := range trunkRows {
			if !own(c.name) && c.distance > 0 {
				scored = append(scored, c)
			}
		}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		a, b := scored[i], scored[j]
		if a.distance != b.distance {
			return a.distance < b.distance
		}
		if ra, rb := conventionalRank(a.name), conventionalRank(b.name); ra != rb {
			return ra < rb
		}
		if len(a.name) != len(b.name) {
			return len(a.name) < len(b.name)
		}
		return a.name < b.name
	})
	if len(scored) > 0 {
		return head, scored[0].name, refs
	}
	if originHead != "" && originHead != branch {
		return head, originHead, refs
	}
	for _, name := range []string{"origin/main", "origin/master", "main", "master"} {
		if present[name] && name != branch {
			return head, name, refs
		}
	}
	return head, "", refs
}

// conventional is how trunk-like a branch's own name is, the most conventional first; a feature branch ranks past them.
var conventional = []string{"main", "master", "next", "develop"}

func conventionalRank(name string) int {
	short := name[strings.LastIndex(name, "/")+1:]
	if short == "development" {
		short = "develop"
	}
	for i, trunk := range conventional {
		if short == trunk {
			return i
		}
	}
	return len(conventional)
}

// distance is a capped count, as far as can be when git could not say.
func distance(count string) int {
	n, err := strconv.Atoi(strings.TrimSpace(count))
	if err != nil {
		return 1 << 30
	}
	return n
}

// baseFilesScript prints, when HEAD is still the one given ("" for any), the merge base with the base and the commits
// since it, then the numstat from it to the working tree and the untracked files, NUL-separated; "moved" when HEAD moved.
func baseFilesScript(head, base string) string {
	guard := ""
	if head != "" {
		guard = `[ "$(git rev-parse -q --verify HEAD 2>/dev/null)" = ` + script.Quote(head) + ` ] || { echo moved; exit 0; }; `
	}
	return guard + `mb=$(git merge-base HEAD ` + script.Quote(base) + ` 2>/dev/null) || { echo none; exit 0; }; printf '%s\n' "$mb"; ` +
		`git rev-list --count "$mb"..HEAD 2>/dev/null; printf '\036\n'; ` +
		readGit + ` diff --no-ext-diff --no-textconv --no-renames --relative --numstat -z "$mb" 2>/dev/null; printf '\n\036\n'; ` +
		readGit + ` ls-files --others --exclude-standard -z 2>/dev/null`
}

// Base reads the branch's work against its base: pinned, when the person named one, else the fork base detected and
// kept for this HEAD.
func (store *Store) Base(host shell.Host, dir, pinned string) (BaseView, error) {
	if pinned != "" && !RefOK(pinned) {
		return BaseView{}, errors.New("not a branch name")
	}
	storeKey := key(host.ID(), dir)
	store.mu.Lock()
	known := store.bases[storeKey]
	store.mu.Unlock()
	for attempt := 0; attempt < 2; attempt++ {
		if known.head == "" {
			answer, err := shell.Text(host, script.InRepo(dir)+detectScript, 60*time.Second)
			if err != nil {
				return BaseView{}, err
			}
			head, detected, refs := forkBase(answer)
			if head == "" {
				return BaseView{Refs: []string{}, Files: []File{}}, nil
			}
			known = baseRead{head: head, detected: detected, refs: refs}
			store.mu.Lock()
			store.bases[storeKey] = known
			store.mu.Unlock()
		}
		view := BaseView{Base: known.detected, Detected: known.detected, Refs: append([]string{}, known.refs...), Files: []File{}}
		if pinned != "" {
			view.Base = pinned
		}
		if view.Base == "" {
			return view, nil
		}
		output, err := shell.Text(host, script.InRepo(dir)+baseFilesScript(known.head, view.Base), 60*time.Second)
		if err != nil {
			return BaseView{}, err
		}
		switch strings.TrimSpace(output) {
		case "moved":
			known = baseRead{}
			continue
		case "none":
			return view, errors.New(view.Base + " shares no history with this branch")
		}
		sections := script.Sections(output)
		heads := script.Lines(sections[0])
		if len(heads) > 0 {
			view.MergeBase = heads[0]
		}
		if len(heads) > 1 {
			view.Ahead, _ = strconv.Atoi(heads[1])
		}
		if len(sections) > 1 {
			for path, stat := range readNumstat(sections[1]) {
				view.Files = append(view.Files, File{Path: path, Added: stat.Added, Removed: stat.Removed})
			}
		}
		if len(sections) > 2 {
			for _, path := range strings.Split(strings.Trim(sections[2], "\n"), "\x00") {
				if path != "" {
					view.Files = append(view.Files, File{Path: path, Untracked: true})
				}
			}
		}
		sort.Slice(view.Files, func(i, j int) bool { return view.Files[i].Path < view.Files[j].Path })
		return view, nil
	}
	return BaseView{}, errors.New("the branch kept moving while it was read")
}

// BaseDiff is one file's change from the merge base to the working tree; an untracked file whole.
func BaseDiff(host shell.Host, dir, mergeBase, path string) (string, error) {
	if !CommitOK(mergeBase) {
		return "", errors.New("not a commit")
	}
	quoted := script.Quote(path)
	return shell.Text(host, script.InDir(dir)+
		`if `+readGit+` ls-files --error-unmatch -- `+quoted+` >/dev/null 2>&1 || git cat-file -e `+mergeBase+`:./`+quoted+` 2>/dev/null; then `+
		diffGit+` `+mergeBase+` -- `+quoted+` | head -c `+strconv.Itoa(maxDiff)+`; `+
		`else git -c color.ui=never diff --no-ext-diff --no-index -- /dev/null `+quoted+` | head -c `+strconv.Itoa(maxDiff)+`; fi; exit 0`, 60*time.Second)
}

// RefOK reports whether name may reach git as a branch's name: no option, no range, no blank or control character.
func RefOK(name string) bool {
	if name == "" || strings.HasPrefix(name, "-") || strings.Contains(name, "..") || strings.ContainsAny(name, "~^:?*[\\") {
		return false
	}
	for _, r := range name {
		if r <= ' ' || r == 0x7f {
			return false
		}
	}
	return true
}
