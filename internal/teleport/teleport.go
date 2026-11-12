// Package teleport moves the work an agent is doing to another machine: the checkout's branch with the commits the
// target lacks, its staged and unstaged changes as they were and its new files land in a fresh worktree of the same
// repository there, where another agent carries on with the conversation's gist as its first prompt. Nothing on the
// source changes, nothing goes through a remote (a git bundle travels over remuda's own shells), and ~/.claude is
// never written: the conversation travels as the handover text.
package teleport

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// Host is a machine the work leaves or lands on: its shell, and a file put there whole.
type Host interface {
	shell.Host
	PutFile(path string, data []byte, timeout time.Duration) error
}

// Source is a checkout as the inspection read it on its machine.
type Source struct {
	Root      string `json:"root"`
	Branch    string `json:"branch"`
	Head      string `json:"head"`
	Remote    string `json:"remote"`     // origin's address, else the first remote's; "" for none
	FirstRoot string `json:"first_root"` // a root commit of the history, what another clone of the repository shares
	Ahead     int    `json:"ahead"`      // commits on no remote-tracking branch
	Staged    int    `json:"staged"`
	Unstaged  int    `json:"unstaged"`
	Untracked int    `json:"untracked"`
	state     string // merge, rebase or cherry-pick in progress
	unmerged  int
}

// Checkout is a checkout found on a machine, and whether it holds the same repository as the source.
type Checkout struct {
	Dir    string `json:"dir"`
	Branch string `json:"branch"`
	Remote string `json:"remote"`
	Same   bool   `json:"same"`
}

// MaxPack is the most the work may weigh on its way: the bundle, the patches and the new files together.
const MaxPack = 256 << 20

const inspectTimeout = 60 * time.Second

// RemoteScript prints the checkout's remote address, in the folder the script stands in: origin's, else the first
// remote's, else nothing; the fleet queue reads the checkouts' repositories with it too (internal/placement).
const RemoteScript = remoteScript

// remoteScript is RemoteScript.
const remoteScript = `{ git remote get-url origin 2>/dev/null || { r=$(git remote | head -n 1); [ -n "$r" ] && git remote get-url "$r"; }; } 2>/dev/null | head -n 1 | tr -d '\n'`

// inspectScript reads a checkout in one line of fields; a folder outside git answers nothing.
func inspectScript(dir string) string {
	return script.InRepo(dir) + `cd "$root" || exit 0; g=$(git rev-parse --git-dir); st=; ` +
		`[ -e "$g/MERGE_HEAD" ] && st=merge; [ -e "$g/CHERRY_PICK_HEAD" ] && st=cherry-pick; { [ -d "$g/rebase-merge" ] || [ -d "$g/rebase-apply" ]; } && st=rebase; ` +
		`if [ -n "$(git for-each-ref --count=1 refs/remotes)" ]; then ahead=$(git rev-list --count HEAD --not --remotes 2>/dev/null); else ahead=$(git rev-list --count HEAD 2>/dev/null); fi; ` +
		`printf '%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s' "$root" "$(git symbolic-ref --short -q HEAD)" "$(git rev-parse -q --verify HEAD)" "$(` + remoteScript + `)" ` +
		`"$(git rev-list --max-parents=0 HEAD 2>/dev/null | head -n 1)" "${ahead:-0}" ` +
		`"$(git diff --cached --name-only | wc -l | tr -d ' ')" "$(git diff --name-only | wc -l | tr -d ' ')" "$(git ls-files --others --exclude-standard | wc -l | tr -d ' ')" ` +
		`"$st" "$(git diff --name-only --diff-filter=U | wc -l | tr -d ' ')"`
}

// Inspect reads the checkout the work is in, refusing one that cannot travel: outside git, on no branch, in the middle
// of a merge or a rebase, with conflicts.
func Inspect(host shell.Host, dir string) (Source, error) {
	out, err := shell.Text(host, inspectScript(dir), inspectTimeout)
	if err != nil {
		return Source{}, fmt.Errorf("the checkout could not be read on %s: %w", host.Label(), err)
	}
	fields := script.Fields(out)
	if len(fields) != 11 {
		return Source{}, fmt.Errorf("%s on %s is not a git checkout", dir, host.Label())
	}
	number := func(s string) int { n, _ := strconv.Atoi(strings.TrimSpace(s)); return n }
	src := Source{Root: fields[0], Branch: fields[1], Head: fields[2], Remote: fields[3], FirstRoot: fields[4], Ahead: number(fields[5]),
		Staged: number(fields[6]), Unstaged: number(fields[7]), Untracked: number(fields[8]), state: fields[9], unmerged: number(fields[10])}
	switch {
	case src.Head == "":
		return src, errors.New("the checkout has no commit yet: there is no history to move")
	case src.Branch == "":
		return src, errors.New("the checkout is on no branch (a detached HEAD): check out a branch first")
	case src.state != "":
		return src, fmt.Errorf("a %s is in progress in the checkout: finish or abort it first", src.state)
	case src.unmerged > 0:
		return src, fmt.Errorf("%d file(s) have unresolved conflicts: resolve them first", src.unmerged)
	}
	return src, nil
}

// Words is what the source holds, for the log and the dialog: "feature-x · 3 commits on no remote · 2 staged · 5 changed · 1 new file".
func (s Source) Words() string {
	parts := []string{s.Branch}
	add := func(n int, one, many string) {
		if n == 1 {
			parts = append(parts, "1 "+one)
		} else if n > 1 {
			parts = append(parts, fmt.Sprintf("%d %s", n, many))
		}
	}
	add(s.Ahead, "commit on no remote", "commits on no remote")
	add(s.Staged, "staged file", "staged files")
	add(s.Unstaged, "changed file", "changed files")
	add(s.Untracked, "new file", "new files")
	return strings.Join(parts, " · ")
}

// Checkouts lists the git checkouts among a machine's folders (its workspaces'), each once, the ones holding the
// source's repository — by its remote — first.
func Checkouts(host shell.Host, dirs []string, src Source) ([]Checkout, error) {
	if len(dirs) == 0 {
		return nil, nil
	}
	var sweep strings.Builder
	for _, dir := range dirs {
		sweep.WriteString(`( ` + script.InRepo(dir) + `printf '%s\037%s\037%s\036' "$root" "$(git symbolic-ref --short -q HEAD)" "$(` + remoteScript + `)" ); `)
	}
	out, err := shell.Text(host, sweep.String(), inspectTimeout)
	if err != nil {
		return nil, fmt.Errorf("the checkouts on %s could not be read: %w", host.Label(), err)
	}
	seen := map[string]bool{}
	var same, other []Checkout
	for _, record := range strings.Split(out, script.RS) {
		fields := script.Fields(record)
		if len(fields) != 3 || fields[0] == "" || seen[fields[0]] {
			continue
		}
		seen[fields[0]] = true
		c := Checkout{Dir: fields[0], Branch: fields[1], Remote: fields[2], Same: SameRepository(src.Remote, fields[2])}
		if c.Same {
			same = append(same, c)
		} else {
			other = append(other, c)
		}
	}
	return append(same, other...), nil
}

var scpLike = regexp.MustCompile(`^(?:[^@/]+@)?([^:/]+):(.+)$`)

// RemoteKey is a remote's address reduced to host and path, so the ssh and https spellings of one repository agree:
// git@gitlab.com:team/app.git and https://gitlab.com/team/app are both "gitlab.com/team/app".
func RemoteKey(remote string) string {
	r := strings.TrimSpace(remote)
	if r == "" {
		return ""
	}
	host, rest := "", r
	if strings.Contains(r, "://") {
		u, err := url.Parse(r)
		if err != nil {
			return strings.ToLower(r)
		}
		host, rest = u.Hostname(), u.Path
	} else if m := scpLike.FindStringSubmatch(r); m != nil {
		host, rest = m[1], m[2]
	}
	rest = strings.TrimSuffix(strings.Trim(rest, "/"), ".git")
	if host == "" {
		return path.Clean(rest)
	}
	return strings.ToLower(host) + "/" + rest
}

// SameRepository says whether two remotes name one repository; an empty one names none.
func SameRepository(a, b string) bool {
	ka, kb := RemoteKey(a), RemoteKey(b)
	return ka != "" && ka == kb
}

// Fits checks that the folder on the target holds the source's repository: the same remote, or, where either has
// none, the source's first commit in its history.
func Fits(target shell.Host, dir string, src Source) (string, error) {
	check := script.InRepo(dir) + `cd "$root" || exit 0; printf '%s\037%s\037' "$root" "$(` + remoteScript + `)"; ` +
		`if git cat-file -e ` + script.Quote(src.FirstRoot+"^{commit}") + ` 2>/dev/null; then printf yes; fi`
	out, err := shell.Text(target, check, inspectTimeout)
	if err != nil {
		return "", fmt.Errorf("%s could not be read on %s: %w", dir, target.Label(), err)
	}
	fields := script.Fields(out)
	if len(fields) != 3 || fields[0] == "" {
		return "", fmt.Errorf("%s on %s is not a git checkout", dir, target.Label())
	}
	root, remote, shared := fields[0], fields[1], fields[2] == "yes"
	if remote != "" && src.Remote != "" && !SameRepository(remote, src.Remote) {
		return "", fmt.Errorf("%s on %s holds another repository (%s, not %s)", root, target.Label(), remote, src.Remote)
	}
	if (remote == "" || src.Remote == "") && !shared {
		return "", fmt.Errorf("%s on %s does not share this repository's history", root, target.Label())
	}
	return root, nil
}
