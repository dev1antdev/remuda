// Package worktrees takes a linked git worktree down with its branch, on the machine, as the person — the one thing
// remuda ever deletes from a repository, and only when asked: once the work's pull request is merged, when the
// workspace on a worktree herdr or remuda made is closed (Tidy, tidy.go), or for an arena's lanes the person did not keep. A look first (Inspect) refuses a worktree with uncommitted changes before anything is
// closed; the removal (Remove) never passes --force to git, and deletes a branch that git does not see as merged only
// when the caller says the request was. Discard is one exception, an arena's losing lane: its work saved as a
// patch by the caller first, after a confirm that says so, then git's --force and -D; the remove dialog's removal is
// the other (RemoveAsShown, removal.go): --force past exactly what the person was shown and confirmed.
// A worktree remuda makes gets the ignored files its repository's .worktreeinclude lists (Include, include.go), and is
// made the way the New worktree dialog asks (create.go), its setup script run in its terminal (setup.go).
package worktrees

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// Timeout is how long one git call may take.
var Timeout = 30 * time.Second

// Info is what a look at a linked worktree says before anything goes.
type Info struct {
	Path   string // the worktree's folder
	Branch string // the branch checked out there, "" when detached
	Repo   string // the repository's main worktree, where the removal runs
	Dirty  string // the first uncommitted entry of git status, "" when clean
}

// InspectScript prints the worktree's branch, its repository's main worktree and the first line of its status.
func InspectScript(path string) string {
	return script.PATH + "\ncd " + script.Quote(path) + " 2>/dev/null || { echo 'the folder is not there'; exit 3; }\n" +
		"git rev-parse --is-inside-work-tree >/dev/null 2>&1 || { echo 'not a git worktree'; exit 3; }\n" +
		"printf '%s\\n' \"$(git rev-parse --abbrev-ref HEAD 2>/dev/null)\"\n" +
		"printf '%s\\n' \"$(git worktree list --porcelain | sed -n '1s/^worktree //p')\"\n" +
		"git status --porcelain --untracked-files=normal 2>/dev/null | head -1\n"
}

// ParseInspect reads the script's three lines.
func ParseInspect(path, out string) (Info, error) {
	lines := strings.SplitN(strings.TrimRight(out, "\n"), "\n", 3)
	if len(lines) < 2 || lines[1] == "" {
		return Info{}, errors.New(lastLine(out, "git answered nothing"))
	}
	info := Info{Path: path, Branch: lines[0], Repo: lines[1]}
	if info.Branch == "HEAD" {
		info.Branch = ""
	}
	if len(lines) == 3 {
		info.Dirty = strings.TrimSpace(lines[2])
	}
	return info, nil
}

// Inspect looks at the worktree on the machine.
func Inspect(host shell.Shell, path string) (Info, error) {
	out, err := shell.Text(host, InspectScript(path), Timeout)
	if err != nil && strings.TrimSpace(out) != "" {
		return Info{}, errors.New(lastLine(out, err.Error()))
	}
	if err != nil {
		return Info{}, err
	}
	return ParseInspect(path, out)
}

// Landed is a merged request's branch and the commit it ended on: what vouches for a branch a squash leaves git unsure of.
type Landed struct {
	Branch, Commit string
}

// landedCommits are the commits merged requests of the branch ended on.
func landedCommits(landed []Landed, branch string) []string {
	var commits []string
	for _, l := range landed {
		if l.Branch == branch && l.Commit != "" {
			commits = append(commits, l.Commit)
		}
	}
	return commits
}

// LandedAt says a merged request of the branch ended on the commit, so nothing on the branch is lost with it.
func LandedAt(landed []Landed, branch, tip string) bool {
	return branch != "" && tip != "" && slices.Contains(landedCommits(landed, branch), tip)
}

// RemoveScript removes the worktree from its repository and deletes its branch: git's own -d, which refuses a branch
// it does not see as merged, or -D when the branch ends, there and then, on a commit a merged request of it ended on.
func RemoveScript(info Info, landed []Landed) string {
	s := script.PATH + "\ncd " + script.Quote(info.Repo) + " 2>/dev/null || { echo 'the repository is not there'; exit 3; }\n" +
		"git worktree remove " + script.Quote(info.Path) + " 2>&1 || exit 4\n"
	if info.Branch == "" {
		return s
	}
	branch := script.Quote(info.Branch)
	flag := "-d"
	if commits := landedCommits(landed, info.Branch); len(commits) > 0 {
		s += "tip=$(git rev-parse --verify -q " + script.Quote("refs/heads/"+info.Branch) + ")\n" +
			"flag=-d; case " + script.Quote(" "+strings.Join(commits, " ")+" ") + " in *\" ${tip:-none} \"*) flag=-D ;; esac\n"
		flag = `"$flag"`
	}
	return s + "git branch " + flag + " " + branch + " 2>&1 || exit 5\n"
}

// Remove takes the worktree down on the machine; the error is git's own line.
func Remove(host shell.Shell, info Info, landed []Landed) error {
	out, err := shell.Text(host, RemoveScript(info, landed), Timeout)
	if err == nil {
		return nil
	}
	var exit *shell.ExitError
	if errors.As(err, &exit) && exit.Code == 5 {
		return errors.New("the worktree is removed; the branch stays: " + lastLine(out, err.Error()))
	}
	return errors.New(lastLine(out, err.Error()))
}

// DiscardScript removes a worktree whose work the caller has saved, uncommitted changes and all, and deletes its branch.
func DiscardScript(info Info) string {
	s := script.PATH + "\ncd " + script.Quote(info.Repo) + " 2>/dev/null || { echo 'the repository is not there'; exit 3; }\n" +
		"git worktree remove --force " + script.Quote(info.Path) + " 2>&1 || exit 4\n"
	if info.Branch != "" {
		s += "git branch -D " + script.Quote(info.Branch) + " 2>&1 || exit 5\n"
	}
	return s
}

// Discard takes down a worktree whose work the caller saved first (an arena's lane the person did not keep); git's line on a refusal.
func Discard(host shell.Shell, info Info) error {
	out, err := shell.Text(host, DiscardScript(info), Timeout)
	if err == nil {
		return nil
	}
	var exit *shell.ExitError
	if errors.As(err, &exit) && exit.Code == 5 {
		return errors.New("the worktree is removed; the branch stays: " + lastLine(out, err.Error()))
	}
	return errors.New(lastLine(out, err.Error()))
}

// lastLine is git's complaint: the last line it printed, else the fallback.
func lastLine(text, fallback string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return fallback
	}
	last := text
	if i := strings.LastIndex(text, "\n"); i >= 0 {
		last = text[i+1:]
	}
	return strings.TrimSpace(last)
}
