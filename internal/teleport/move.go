package teleport

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

const (
	packTimeout = 5 * time.Minute
	putTimeout  = 10 * time.Minute
	landTimeout = 10 * time.Minute
)

// packScript leaves the work of the checkout as one tar on stdout: meta (the branch and HEAD), commits.bundle (the
// branch's commits on no remote-tracking branch, or all of them when full or when there is no remote; none when every
// commit is on a remote already), staged.patch, unstaged.patch and untracked.tar (the new files git does not ignore).
// Nothing in the checkout changes.
func packScript(dir string, full bool) string {
	scope := `refs/heads/"$branch" --not --remotes`
	if full {
		scope = `refs/heads/"$branch"`
	}
	return script.InRepo(dir) + `cd "$root" || exit 1; ` +
		`t=$(mktemp -d "${TMPDIR:-/tmp}/remuda-teleport.XXXXXX") || exit 1; trap 'rm -rf "$t"' EXIT; ` +
		`branch=$(git symbolic-ref --short -q HEAD) || { echo "the checkout is on no branch" >&2; exit 1; }; ` +
		`printf '%s\n%s\n' "$branch" "$(git rev-parse HEAD)" > "$t/meta"; ` +
		`if [ -z "$(git for-each-ref --count=1 refs/remotes)" ]; then git bundle create -q "$t/commits.bundle" refs/heads/"$branch" 2> "$t/err" || { cat "$t/err" >&2; exit 1; }; ` +
		`else git bundle create -q "$t/commits.bundle" ` + scope + ` 2> "$t/err" || { grep -qi 'empty bundle' "$t/err" && rm -f "$t/commits.bundle" || { cat "$t/err" >&2; exit 1; }; }; fi; ` +
		`rm -f "$t/err"; git diff --binary --cached > "$t/staged.patch" && git diff --binary > "$t/unstaged.patch" || exit 1; ` +
		`git ls-files -z --others --exclude-standard > "$t/untracked.list"; ` +
		`if [ -s "$t/untracked.list" ]; then tar -cf "$t/untracked.tar" --null -T "$t/untracked.list" || { echo "the new files could not be packed" >&2; exit 1; }; fi; rm -f "$t/untracked.list"; ` +
		`kb=$(du -sk "$t" | cut -f1); [ "$kb" -le ` + strconv.Itoa(MaxPack/1024) + ` ] || { echo "the work weighs $kb KB, more than remuda carries (` + strconv.Itoa(MaxPack>>20) + ` MB)" >&2; exit 1; }; ` +
		`tar -cf - -C "$t" .`
}

// Pack reads the work out of the checkout on its machine as one archive.
func Pack(host shell.Host, dir string, full bool) ([]byte, error) {
	data, err := host.Run(packScript(dir, full), packTimeout)
	if err != nil {
		return nil, fmt.Errorf("the work could not be packed on %s: %w", host.Label(), err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%s on %s is not a git checkout", dir, host.Label())
	}
	return data, nil
}

// needFull is what the landing says when the bundle's base is not in the target's history even after a fetch:
// the whole branch has to travel.
const needFull = "remuda-teleport: need the whole branch"

// landScript puts the packed work into a fresh worktree of the target's checkout: the branch fetched from the bundle
// (under its own name, or with a suffix when the target already has one so named), a worktree for it under worktrees,
// the staged changes applied to the index and the files, the unstaged ones to the files, the new files unpacked. It
// prints what it did a line at a time, among them "branch <name>" and "worktree <path>"; the pack is removed.
func landScript(dir, pack, worktrees, suffix string) string {
	fail := func(what string) string { return `{ echo ` + script.Quote("error: "+what) + `; exit 1; }` }
	named := `"$branch-"` + script.Quote(suffix)
	return `cd ` + script.Quote(dir) + ` 2>/dev/null && root=$(git rev-parse --show-toplevel 2>/dev/null) || ` + fail("the folder is not a git checkout") + `; cd "$root" || exit 1; ` +
		`t=$(mktemp -d "${TMPDIR:-/tmp}/remuda-teleport.XXXXXX") || exit 1; trap 'rm -rf "$t"; rm -f ` + script.Quote(pack) + `' EXIT; ` +
		`tar -xf ` + script.Quote(pack) + ` -C "$t" || ` + fail("the work did not unpack") + `; ` +
		`branch=$(sed -n 1p "$t/meta"); head=$(sed -n 2p "$t/meta"); ` +
		`if [ -f "$t/commits.bundle" ]; then ` +
		`git bundle verify -q "$t/commits.bundle" > /dev/null 2>&1 || { echo "fetching what this checkout lacks from its remotes"; git fetch -q --all 2>&1 | tail -n 3; }; ` +
		`git bundle verify -q "$t/commits.bundle" > /dev/null 2>&1 || { echo ` + script.Quote(needFull) + `; exit 3; }; ` +
		`else git cat-file -e "$head^{commit}" 2> /dev/null || { echo "fetching the branch's commits from this checkout's remotes"; git fetch -q --all 2>&1 | tail -n 3; }; ` +
		`git cat-file -e "$head^{commit}" 2> /dev/null || { echo ` + script.Quote(needFull) + `; exit 3; }; fi; ` +
		`nb="$branch"; n=1; while git show-ref -q --verify "refs/heads/$nb"; do if [ $n = 1 ]; then nb=` + named + `; else nb=` + named + `"-$n"; fi; n=$((n+1)); done; ` +
		`if [ -f "$t/commits.bundle" ]; then git fetch -q "$t/commits.bundle" "refs/heads/$branch:refs/heads/$nb" || ` + fail("the commits did not come over") + `; ` +
		`else git branch -q "$nb" "$head" || ` + fail("the branch could not be made") + `; fi; ` +
		`echo "branch $nb"; base=` + script.Quote(worktrees) + `/$(printf '%s' "$nb" | tr '/' '-'); p="$base"; n=2; while [ -e "$p" ]; do p="$base-$n"; n=$((n+1)); done; ` +
		`{ mkdir -p "$(dirname "$p")" && git worktree add -q "$p" "$nb"; } || ` + fail("git could not add the worktree") + `; echo "worktree $p"; cd "$p" || exit 1; ` +
		`if [ -s "$t/staged.patch" ]; then git apply --index --binary "$t/staged.patch" || { echo "error: the staged changes did not apply"; exit 1; }; echo "the staged changes applied"; fi; ` +
		`if [ -s "$t/unstaged.patch" ]; then git apply --binary "$t/unstaged.patch" || { echo "error: the unstaged changes did not apply"; exit 1; }; echo "the unstaged changes applied"; fi; ` +
		`if [ -f "$t/untracked.tar" ]; then tar -xf "$t/untracked.tar" -C "$p" || { echo "error: the new files did not unpack"; exit 1; }; echo "the new files copied"; fi`
}

// Landed is where the work arrived.
type Landed struct {
	Dir    string `json:"dir"`
	Branch string `json:"branch"`
}

// errNeedFull is a landing that needs the whole branch to travel.
var errNeedFull = errors.New(needFull)

// Land puts the pack (already on the target, at pack) into a fresh worktree of the checkout in dir, telling say what
// it does; a failure after the worktree was made leaves it in place and names it.
func Land(target shell.Host, dir, pack, worktrees, suffix string, say func(string)) (Landed, error) {
	var landed Landed
	var pending bytes.Buffer
	var full bool
	line := func(text string) {
		switch {
		case text == needFull:
			full = true
		case strings.HasPrefix(text, "branch "):
			landed.Branch = strings.TrimPrefix(text, "branch ")
			say("on branch " + landed.Branch)
		case strings.HasPrefix(text, "worktree "):
			landed.Dir = strings.TrimPrefix(text, "worktree ")
			say("a worktree at " + landed.Dir)
		case text != "":
			say(text)
		}
	}
	err := target.Stream(landScript(dir, pack, worktrees, suffix), landTimeout, func(chunk []byte) {
		pending.Write(chunk)
		for {
			text, rest, ok := strings.Cut(pending.String(), "\n")
			if !ok {
				return
			}
			pending.Reset()
			pending.WriteString(rest)
			line(strings.TrimRight(text, "\r"))
		}
	})
	line(strings.TrimSpace(pending.String()))
	if full {
		return landed, errNeedFull
	}
	if err != nil {
		if landed.Dir != "" {
			return landed, fmt.Errorf("the work did not land whole; the worktree stays at %s on %s for you to look at", landed.Dir, target.Label())
		}
		return landed, fmt.Errorf("the work did not land on %s: %w", target.Label(), err)
	}
	if landed.Dir == "" || landed.Branch == "" {
		return landed, fmt.Errorf("the landing on %s said nothing of its worktree", target.Label())
	}
	return landed, nil
}

// Move is the whole way: the source checked and packed, the pack put on the target, landed in a fresh worktree of the
// target's checkout (the whole branch sent again when the target's history lacks the bundle's base), then Start called
// with the worktree to start the agent there. Every step is told; the first failure ends it.
type Move struct {
	Source    Host
	SourceDir string
	Target    Host
	TargetDir string                                // a checkout of the same repository on the target
	Start     func(landed Landed, src Source) error // the agent started in the landed worktree
}

// Run carries the work over, answering where it landed.
func (m Move) Run(say func(string)) (Landed, error) {
	say(fmt.Sprintf("reading the work in %s on %s", m.SourceDir, m.Source.Label()))
	src, err := Inspect(m.Source, m.SourceDir)
	if err != nil {
		return Landed{}, err
	}
	say(src.Words())
	say(fmt.Sprintf("checking %s on %s", m.TargetDir, m.Target.Label()))
	targetRoot, err := Fits(m.Target, m.TargetDir, src)
	if err != nil {
		return Landed{}, err
	}
	if m.Source.ID() == m.Target.ID() && targetRoot == src.Root {
		return Landed{}, errors.New("the target is the very checkout the work is in: pick another clone or another machine")
	}
	worktrees := path.Join(m.Target.Home(), ".herdr", "worktrees", path.Base(targetRoot))
	suffix := "from-" + Slug(m.Source.Label())
	landed, err := m.carry(src, targetRoot, worktrees, suffix, false, say)
	if errors.Is(err, errNeedFull) {
		say("the target's history lacks the commits under the branch: sending the whole branch")
		landed, err = m.carry(src, targetRoot, worktrees, suffix, true, say)
		if errors.Is(err, errNeedFull) {
			return landed, errors.New("the target could not take the branch's history even whole")
		}
	}
	if err != nil {
		return landed, err
	}
	say("starting the agent there")
	if err := m.Start(landed, src); err != nil {
		return landed, fmt.Errorf("the work landed at %s, but the agent did not start: %w", landed.Dir, err)
	}
	return landed, nil
}

// carry packs, copies and lands once.
func (m Move) carry(src Source, targetRoot, worktrees, suffix string, full bool, say func(string)) (Landed, error) {
	say("packing the work")
	data, err := Pack(m.Source, src.Root, full)
	if err != nil {
		return Landed{}, err
	}
	say(fmt.Sprintf("copying %s to %s", Size(len(data)), m.Target.Label()))
	tmp, err := shell.Text(m.Target, `mktemp "${TMPDIR:-/tmp}/remuda-teleport.XXXXXX"`, 30*time.Second)
	if err != nil {
		return Landed{}, fmt.Errorf("no room for the work on %s: %w", m.Target.Label(), err)
	}
	pack := strings.TrimSpace(tmp)
	if err := m.Target.PutFile(pack, data, putTimeout); err != nil {
		return Landed{}, fmt.Errorf("the work could not be copied to %s: %w", m.Target.Label(), err)
	}
	say("landing it in a new worktree")
	return Land(m.Target, targetRoot, pack, worktrees, suffix, say)
}

// Slug is a machine's name as it may sit in a branch: "This Mac · MacBook" is "this-mac-macbook".
func Slug(label string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(label) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if b.Len() > 0 && !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	slug := strings.TrimSuffix(b.String(), "-")
	if len(slug) > 24 {
		slug = strings.TrimSuffix(slug[:24], "-")
	}
	if slug == "" {
		return "elsewhere"
	}
	return slug
}

// Size is a byte count as a person reads it: "812 B", "14 KB", "3.2 MB".
func Size(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1<<20:
		return fmt.Sprintf("%d KB", (n+512)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
}

// Preamble is what the agent taking over is told first: where the work came from and how it arrived.
func Preamble(from, sourceDir string, src Source, landed Landed, here string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# This work moved here from %s\n\n", from)
	fmt.Fprintf(&b, "It was in %s on %s, on branch %s. It now sits in this worktree on %s, on branch %s", sourceDir, from, src.Branch, here, landed.Branch)
	var came []string
	if src.Staged > 0 {
		came = append(came, "the staged changes (still staged)")
	}
	if src.Unstaged > 0 {
		came = append(came, "the unstaged changes")
	}
	if src.Untracked > 0 {
		came = append(came, "the new files")
	}
	if len(came) > 0 {
		b.WriteString(", with " + strings.Join(came, ", ") + " applied as they were")
	}
	b.WriteString(". Nothing was committed or pushed on the way, and the work on " + from + " is untouched.\n")
	return b.String()
}
