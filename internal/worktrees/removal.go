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
	"github.com/dev1antdev/remuda/internal/text"
)

// RemoveTimeout is how long git may take to remove a worktree: a node_modules takes a while to delete.
var RemoveTimeout = 10 * time.Minute

// Look is a linked worktree as its removal would find it.
type Look struct {
	Path          string   `json:"path"`
	Branch        string   `json:"branch,omitempty"`         // "" when detached
	Repo          string   `json:"repo"`                     // the repository's main checkout, where the removal runs
	Dirty         int      `json:"dirty"`                    // uncommitted entries, new files too
	First         string   `json:"first,omitempty"`          // the first of them
	Unpushed      int      `json:"unpushed"`                 // the branch's commits on no remote
	Unmerged      int      `json:"unmerged"`                 // of those, the ones the main checkout's HEAD lacks too: lost with the branch
	Merged        bool     `json:"merged"`                   // the main checkout's HEAD holds the branch
	Tip           string   `json:"-"`                        // the commit the branch ends on
	Changes       string   `json:"-"`                        // a checksum of the uncommitted work's contents
	HasSubmodules bool     `json:"has_submodules,omitempty"` // named by .gitmodules, or kept in the worktree's git folder
	Submodules    []string `json:"submodules,omitempty"`     // the ones with commits pushed nowhere
	Ignored       []string `json:"ignored,omitempty"`        // ignored files and folders that go with it, build and dependency folders left out
	IgnoredMore   int      `json:"ignored_more,omitempty"`   // further ignored entries, beyond the ones named
}

// LookScript reads the worktree: its branch and repository, its status, the branch's reach, its submodules.
func LookScript(path string) string {
	git := repos.Git
	return script.PATH + "\ncd " + script.Quote(path) + " 2>/dev/null || { echo 'the folder is not there'; exit 3; }\n" +
		git + " rev-parse --is-inside-work-tree >/dev/null 2>&1 || { echo 'not a git worktree'; exit 3; }\n" +
		"branch=$(" + git + " symbolic-ref --short -q HEAD 2>/dev/null)\n" +
		"main=$(" + git + " worktree list --porcelain | sed -n '1s/^worktree //p')\n" +
		"printf '%s\\n%s\\n\\036\\n' \"$branch\" \"$main\"\n" +
		git + " status --porcelain --untracked-files=normal --ignore-submodules=none 2>/dev/null\nprintf '\\036\\n'\n" +
		"if [ -n \"$branch\" ]; then\n" +
		"  " + git + " rev-list --count \"refs/heads/$branch\" --not --remotes 2>/dev/null || echo 0\n" +
		"  " + git + " -C \"$main\" rev-list --count \"refs/heads/$branch\" --not --remotes HEAD 2>/dev/null || echo 0\n" +
		"  if " + git + " -C \"$main\" merge-base --is-ancestor \"refs/heads/$branch\" HEAD 2>/dev/null; then echo merged; else echo open; fi\n" +
		"  " + git + " rev-parse --verify -q \"refs/heads/$branch\" 2>/dev/null\n" +
		"else printf '0\\n0\\nopen\\n'; fi\nprintf '\\036\\n'\n" +
		"{ [ -f .gitmodules ] || [ -n \"$(ls -A \"$(" + git + " rev-parse --git-path modules)\" 2>/dev/null)\" ]; } && echo submodules\n" +
		git + " submodule foreach --quiet --recursive 'if [ -n \"$(git rev-list -n1 HEAD --branches $(git rev-parse -q --verify refs/stash) --not --remotes 2>/dev/null)\" ]; then printf \"lost %s\\n\" \"$displaypath\"; fi' 2>/dev/null\n" +
		"printf '\\036\\n'\n" + changesScript() + "\nprintf '\\036\\n'\n" +
		git + " ls-files -z --others --ignored --exclude-standard --directory 2>/dev/null | tr '\\0' '\\n'\n"
}

// changesScript prints a checksum of the uncommitted work in the folder it runs in and in its submodules: staged,
// unstaged, the contents of new files and where new links point, all of a repository git lists as one new folder, and
// what a submodule's own branches and stash hold, which go with its folder. What it cannot read adds the shell's process id, so no two looks agree and nothing goes past --force unseen.
func changesScript() string {
	work := func(git string) string {
		return git + " diff " + script.PatchForm + " --binary || echo $$; " + git + " diff --cached " + script.PatchForm + " --binary || echo $$; " +
			git + " ls-files -o --exclude-standard >/dev/null || echo $$; " +
			git + ` ls-files -z -o --exclude-standard | xargs -0 sh -c "for p; do find \"./\$p\" -type f -exec cksum {} + && find \"./\$p\" -type l -exec ls -ld {} + || echo \$\$; done" sh`
	}
	return "{ " + work(repos.Git) + "; " + repos.Git + " submodule foreach --quiet --recursive '" + work(script.ReadGit) + "; git rev-parse HEAD; git for-each-ref refs/heads refs/stash; true' || echo $$; } 2>/dev/null | cksum"
}

// ParseLook reads the script's answer.
func ParseLook(path, out string) (Look, error) {
	parts := script.Sections(out)
	if len(parts) < 4 {
		return Look{}, errors.New(text.LastLine(out, "git answered nothing"))
	}
	head := strings.SplitN(strings.TrimRight(parts[0], "\n"), "\n", 2)
	if len(head) < 2 || strings.TrimSpace(head[1]) == "" {
		return Look{}, errors.New(text.LastLine(out, "git named no repository"))
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
	if len(reach) >= 4 {
		look.Tip = strings.TrimSpace(reach[3])
	}
	for _, line := range script.Lines(parts[3]) {
		switch {
		case strings.TrimSpace(line) == "submodules":
			look.HasSubmodules = true
		case strings.HasPrefix(line, "lost "):
			look.Submodules = append(look.Submodules, strings.TrimSpace(strings.TrimPrefix(line, "lost ")))
		}
	}
	if len(parts) > 4 {
		look.Changes = strings.TrimSpace(parts[4])
	}
	if len(parts) > 5 {
		look.Ignored, look.IgnoredMore = shownIgnored(script.Lines(parts[5]))
	}
	return look, nil
}

// maxIgnoredShown bounds the ignored entries the dialog names: the rest is a count, so a worktree full of local files
// still reads as one line.
const maxIgnoredShown = 20

// buildFolders are the dependency and build output folders a worktree is expected to lose: their ignored contents are
// not worth a line in the dialog, which is about the files a person may miss.
var buildFolders = map[string]bool{
	"node_modules": true, "bower_components": true, "jspm_packages": true, "vendor": true,
	"target": true, "build": true, "dist": true, "out": true, "bin": true, "obj": true,
	"coverage": true, ".nyc_output": true, ".cache": true, ".parcel-cache": true, ".turbo": true,
	".next": true, ".nuxt": true, ".svelte-kit": true, ".output": true, ".vercel": true, ".netlify": true,
	".expo": true, ".dart_tool": true, ".gradle": true, ".idea": true, ".vscode": true,
	"__pycache__": true, ".pytest_cache": true, ".mypy_cache": true, ".ruff_cache": true, ".tox": true,
	".venv": true, "venv": true, "virtualenv": true, "pods": true, "deriveddata": true, ".swiftpm": true,
	".terraform": true, ".serverless": true, ".aws-sam": true, ".yarn": true, ".pnpm-store": true,
	".bun": true, "cmake-build-debug": true, "cmake-build-release": true,
}

// buildOutput says whether an ignored entry is dependency or build output, which a worktree is expected to lose whole.
func buildOutput(entry string) bool {
	for _, segment := range strings.Split(strings.TrimSuffix(entry, "/"), "/") {
		if buildFolders[strings.ToLower(segment)] {
			return true
		}
	}
	return false
}

// shownIgnored names the ignored entries a person may not want to lose, up to maxIgnoredShown, and counts the rest.
func shownIgnored(entries []string) ([]string, int) {
	var shown []string
	more := 0
	for _, line := range entries {
		entry := strings.TrimSpace(line)
		if entry == "" || buildOutput(entry) {
			continue
		}
		if len(shown) >= maxIgnoredShown {
			more++
			continue
		}
		shown = append(shown, entry)
	}
	return shown, more
}

// LookAt reads the worktree on its machine.
func LookAt(host shell.Shell, path string) (Look, error) {
	out, err := shell.Text(host, LookScript(path), Timeout)
	if err != nil {
		return Look{}, errors.New(text.LastLine(out, err.Error()))
	}
	return ParseLook(path, out)
}

// Stamp is what the person was shown of the risk; a removal finding another refuses until it is seen.
func (l Look) Stamp() string {
	return strings.Join([]string{strconv.Itoa(l.Dirty), l.First, strconv.Itoa(l.Unmerged), l.Branch, l.Tip, l.Changes, strconv.FormatBool(l.HasSubmodules), strings.Join(l.Submodules, ","), strings.Join(l.Ignored, ","), strconv.Itoa(l.IgnoredMore)}, "|")
}

// Loses says whether removing the folder loses work: uncommitted changes, a submodule's commits pushed nowhere, or
// ignored files a person may miss (dependency and build output left out).
func (l Look) Loses() bool {
	return l.Dirty > 0 || len(l.Submodules) > 0 || len(l.Ignored) > 0 || l.IgnoredMore > 0
}

// ForceScript removes the worktree, --force when it holds what the look showed, and deletes its branch when asked, as it
// stood when looked at, since the person was told what of it is on no remote.
func ForceScript(look Look, deleteBranch bool) string {
	force := ""
	if look.Dirty > 0 || look.HasSubmodules {
		force = "--force "
	}
	s := script.PATH + "\ncd " + script.Quote(look.Repo) + " 2>/dev/null || { echo 'the repository is not there'; exit 3; }\n"
	if force != "" { // the look was before the workspace closed: work saved since is not what the person agreed to lose
		s += "now=$(cd " + script.Quote(look.Path) + " && " + changesScript() + ")\n" +
			"[ \"$now\" = " + script.Quote(look.Changes) + " ] || { echo 'its uncommitted work changed since you confirmed'; exit 4; }\n"
	}
	s += "git worktree remove " + force + script.Quote(look.Path) + " 2>&1 || exit 4\n"
	switch {
	case !deleteBranch || look.Branch == "":
	case look.Tip != "":
		s += deleteBranchAt(look.Branch, script.Quote(look.Tip))
	default:
		s += "echo 'the branch had no commit when it was looked at'; exit 5\n"
	}
	return s
}

// RemoveAsShown takes the worktree down as the look says, its branch with it when asked; what stays is in the outcome.
func RemoveAsShown(host shell.Shell, look Look, deleteBranch bool) Outcome {
	out := Outcome{Path: look.Path, Branch: look.Branch}
	// Asked here, at the removal: the workspace's close and the roster's refresh took a moment, and herdr may have
	// started in it, its agents there not the ones this remuda sees.
	if why := Hold(host); why != "" {
		out.Kept = why
		return out
	}
	said, err := shell.Text(host, ForceScript(look, deleteBranch), RemoveTimeout)
	var exit *shell.ExitError
	switch {
	case err == nil:
		out.Removed, out.BranchDeleted = true, deleteBranch && look.Branch != ""
		if !deleteBranch && look.Branch != "" {
			out.BranchKept = "kept as asked"
		}
	case errors.As(err, &exit) && exit.Code == 5:
		out.Removed, out.BranchKept = true, text.LastLine(said, err.Error())
	default:
		out.Kept = text.LastLine(said, err.Error())
	}
	return out
}
