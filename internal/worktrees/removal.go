package worktrees

// A worktree removed at the person's word, after Orca's remove dialog: a look first says what goes and what would be
// lost — the uncommitted changes, the branch's commits on no remote and in no main checkout, a submodule's commits
// pushed nowhere — and the removal runs only with the look the person saw still standing (Stamp). It passes git's
// --force only for what the person was shown: uncommitted work, or submodules, which git never removes without it.

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/repos"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// RemoveTimeout is how long git may take to remove a worktree: a node_modules takes a while to delete.
var RemoveTimeout = 10 * time.Minute

// Look is a linked worktree as its removal would find it.
type Look struct {
	Path          string   `json:"path"`
	Branch        string   `json:"branch,omitempty"` // "" when detached
	Repo          string   `json:"repo"`             // the repository's main checkout, where the removal runs
	Dirty         int      `json:"dirty"`            // uncommitted entries, new files too
	First         string   `json:"first,omitempty"`  // the first of them
	Unpushed      int      `json:"unpushed"`         // the branch's commits on no remote
	Unmerged      int      `json:"unmerged"`         // of those, the ones the main checkout's HEAD lacks too: lost with the branch
	Merged        bool     `json:"merged"`           // the main checkout's HEAD holds the branch
	HasSubmodules bool     `json:"has_submodules,omitempty"`
	Submodules    []string `json:"submodules,omitempty"` // the ones with commits pushed nowhere
}

// LookScript reads the worktree: its branch and repository, its status, the branch's reach, its submodules.
func LookScript(path string) string {
	git := repos.Git
	return script.PATH + "\ncd " + script.Quote(path) + " 2>/dev/null || { echo 'the folder is not there'; exit 3; }\n" +
		git + " rev-parse --is-inside-work-tree >/dev/null 2>&1 || { echo 'not a git worktree'; exit 3; }\n" +
		"branch=$(" + git + " symbolic-ref --short -q HEAD 2>/dev/null)\n" +
		"main=$(" + git + " worktree list --porcelain | sed -n '1s/^worktree //p')\n" +
		"printf '%s\\n%s\\n\\036\\n' \"$branch\" \"$main\"\n" +
		git + " status --porcelain --untracked-files=normal 2>/dev/null\nprintf '\\036\\n'\n" +
		"if [ -n \"$branch\" ]; then\n" +
		"  " + git + " rev-list --count \"refs/heads/$branch\" --not --remotes 2>/dev/null || echo 0\n" +
		"  " + git + " -C \"$main\" rev-list --count \"refs/heads/$branch\" --not --remotes HEAD 2>/dev/null || echo 0\n" +
		"  if " + git + " -C \"$main\" merge-base --is-ancestor \"refs/heads/$branch\" HEAD 2>/dev/null; then echo merged; else echo open; fi\n" +
		"else printf '0\\n0\\nopen\\n'; fi\nprintf '\\036\\n'\n" +
		"[ -f .gitmodules ] && echo submodules\n" +
		git + " submodule foreach --quiet --recursive 'if [ -n \"$(git rev-list -n1 HEAD --not --remotes 2>/dev/null)\" ]; then printf \"lost %s\\n\" \"$displaypath\"; fi' 2>/dev/null\ntrue\n"
}

// ParseLook reads the script's answer.
func ParseLook(path, out string) (Look, error) {
	parts := script.Sections(out)
	if len(parts) < 4 {
		return Look{}, errors.New(lastLine(out, "git answered nothing"))
	}
	head := strings.SplitN(strings.TrimRight(parts[0], "\n"), "\n", 2)
	if len(head) < 2 || strings.TrimSpace(head[1]) == "" {
		return Look{}, errors.New(lastLine(out, "git named no repository"))
	}
	look := Look{Path: path, Branch: strings.TrimSpace(head[0]), Repo: strings.TrimSpace(head[1])}
	for _, line := range script.Lines(parts[1]) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if look.Dirty == 0 {
			look.First = strings.TrimSpace(line)
		}
		look.Dirty++
	}
	reach := script.Lines(parts[2])
	if len(reach) >= 3 {
		look.Unpushed, _ = strconv.Atoi(strings.TrimSpace(reach[0]))
		look.Unmerged, _ = strconv.Atoi(strings.TrimSpace(reach[1]))
		look.Merged = strings.TrimSpace(reach[2]) == "merged"
	}
	for _, line := range script.Lines(parts[3]) {
		switch {
		case strings.TrimSpace(line) == "submodules":
			look.HasSubmodules = true
		case strings.HasPrefix(line, "lost "):
			look.Submodules = append(look.Submodules, strings.TrimSpace(strings.TrimPrefix(line, "lost ")))
		}
	}
	return look, nil
}

// LookAt reads the worktree on its machine.
func LookAt(host shell.Shell, path string) (Look, error) {
	out, err := shell.Text(host, LookScript(path), Timeout)
	if err != nil {
		return Look{}, errors.New(lastLine(out, err.Error()))
	}
	return ParseLook(path, out)
}

// Stamp is what the person was shown of the risk; a removal finding another refuses until it is seen.
func (l Look) Stamp() string {
	return strings.Join([]string{strconv.Itoa(l.Dirty), l.First, strconv.Itoa(l.Unmerged), l.Branch, strings.Join(l.Submodules, ",")}, "|")
}

// Loses says whether removing the folder loses work: uncommitted changes, or a submodule's commits pushed nowhere.
func (l Look) Loses() bool { return l.Dirty > 0 || len(l.Submodules) > 0 }

// ForceScript removes the worktree, --force when it holds what the look showed, and deletes its branch when asked:
// git's -D, since the person was told what of it is on no remote.
func ForceScript(look Look, deleteBranch bool) string {
	force := ""
	if look.Dirty > 0 || look.HasSubmodules {
		force = "--force "
	}
	s := script.PATH + "\ncd " + script.Quote(look.Repo) + " 2>/dev/null || { echo 'the repository is not there'; exit 3; }\n" +
		"git worktree remove " + force + script.Quote(look.Path) + " 2>&1 || exit 4\n"
	if deleteBranch && look.Branch != "" {
		s += "git branch -D " + script.Quote(look.Branch) + " 2>&1 || exit 5\n"
	}
	return s
}

// RemoveAsShown takes the worktree down as the look says, its branch with it when asked; what stays is in the outcome.
func RemoveAsShown(host shell.Shell, look Look, deleteBranch bool) Outcome {
	out := Outcome{Path: look.Path, Branch: look.Branch}
	text, err := shell.Text(host, ForceScript(look, deleteBranch), RemoveTimeout)
	var exit *shell.ExitError
	switch {
	case err == nil:
		out.Removed, out.BranchDeleted = true, deleteBranch && look.Branch != ""
		if !deleteBranch && look.Branch != "" {
			out.BranchKept = "kept as asked"
		}
	case errors.As(err, &exit) && exit.Code == 5:
		out.Removed, out.BranchKept = true, lastLine(text, err.Error())
	default:
		out.Kept = lastLine(text, err.Error())
	}
	return out
}
