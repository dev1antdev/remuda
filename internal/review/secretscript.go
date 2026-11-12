package review

import (
	"strconv"

	"github.com/dev1antdev/remuda/internal/script"
)

// gitDiff is git's diff as the check reads it: no colour, no external driver or textconv, no renames, no context.
const gitDiff = `git -c core.quotepath=off -c color.ui=never diff --no-ext-diff --no-textconv --no-renames -U0`

// capScript cuts the stream on its way out: each file's lines at scanPerFile (a line "\036partly" says where), and
// every file once scanTotal is spent (each named on a line "\036budget <path>", "\036unread <path>" for an untracked
// one); remuda's own lines pass. Past a line "\036untracked" come git grep's lines of the untracked files, path, line
// number and text joined by \037, or its "Binary file <path> matches": each file's lines go out after a line
// "\036file <path>" as a diff's added lines would, a binary one as "\036binary <path>".
var capScript = `LC_ALL=C awk -v per=` + strconv.Itoa(scanPerFile) + ` -v total=` + strconv.Itoa(scanTotal) + ` '
function untracked(   at, path, text, size, mark) {
	at = index($0, "\037")
	if (at == 0) { if ($0 ~ /^Binary file .* matches$/) print "\036binary " substr($0, 13, length($0) - 20); return }
	path = substr($0, 1, at - 1); text = substr($0, at + 1); text = substr(text, index(text, "\037") + 1)
	if (path != current) { current = path; n = 0; cut = 0; over = (sum > total); mark = over ? "\036unread " : "\036file "; print mark path }
	if (over || cut) return
	size = length(text) + 1; n += size; sum += size
	if (n > per) { cut = 1; print "\036partly"; return }
	print "+" text
}
/^\036untracked$/ { grep = 1; next }
/^\036/ { grep = 0; print; next }
grep { untracked(); next }
/^diff --git / { header = 1; n = 0; cut = 0; over = (sum > total); if (!over) print; next }
/^@@/ { header = 0 }
over { if (header && /^\+\+\+ /) print "\036budget " substr($0, 5); next }
cut { next }
{ size = length($0) + 1; n += size; sum += size; if (n > per) { cut = 1; print "\036partly"; next }; print }'`

// untrackedScript prints the untracked files git does not ignore, the first scanUntracked of them, in one git grep —
// one process for them all, never one per file: two thousand of them took twenty seconds on a Mac — after a line
// "\036untracked"; those past the count are counted ("\036many N"), a file over scanPerFile is named ("\036large
// <path>") and not read, and a link or a folder (a repository inside) is passed over. The names travel NUL-separated
// to find and git, so a blank, a quote or a leading dash in one is no trouble; one holding a line break is lost.
var untrackedScript = `list=$(git ls-files -z --others --exclude-standard 2>/dev/null | tr '\0' '\n'); ` +
	`if [ -n "$list" ]; then ` +
	`u=$(printf '%s\n' "$list" | wc -l | tr -d ' '); ` +
	`if [ "${u:-0}" -gt ` + strconv.Itoa(scanUntracked) + ` ]; then printf '\036many %s\n' "$((u - ` + strconv.Itoa(scanUntracked) + `))"; fi; ` +
	`names=$(printf '%s\n' "$list" | head -n ` + strconv.Itoa(scanUntracked) + ` | sed 's|^|./|'); ` +
	`printf '%s\n' "$names" | tr '\n' '\0' | xargs -0 -r sh -c '[ $# -gt 0 ] && find "$@" -prune -type f -size +` + strconv.Itoa(scanPerFile) + `c' sh 2>/dev/null | ` +
	`while IFS= read -r f; do printf '\036large %s\n' "${f#./}"; done; ` +
	`printf '\036untracked\n'; ` +
	`printf '%s\n' "$names" | tr '\n' '\0' | xargs -0 -r sh -c '[ $# -gt 0 ] && find "$@" -prune -type f ! -size +` + strconv.Itoa(scanPerFile) + `c -print0' sh 2>/dev/null | ` +
	`LC_ALL=C xargs -0 -r git --literal-pathspecs -c grep.fullName=false -c grep.column=false -c color.ui=never ` +
	`grep --untracked --no-color -G -n -z -e '' -- 2>/dev/null | tr '\0' '\037'; ` +
	`fi; `

// secretScript prints, in one stream through capScript, the diff of the work against HEAD (the empty tree before the
// first commit) with only the lines added, every untracked file git does not ignore (untrackedScript), after a line
// "\036staged" the index's diff against HEAD — what git commit carries, though a staged file may have changed or gone
// since — and, for a push, the commits on no remote-tracking branch with their whole patches — a push sends the whole commit, whatever
// folder the work is in — each after a line "\036commit <short sha>", those past scanCommits counted ("\036older N").
// A line "\036failed <what>" says git could not read the folder, the work or the commits, and "\036end" closes a
// stream that ran to its end: without it nothing is known, and the check is no clean one.
func secretScript(dir string, push bool) string {
	commits := ""
	if push {
		commits = `if [ "$base" = HEAD ]; then ` +
			`c=$(git rev-list --no-merges --max-count=` + strconv.Itoa(countCommits) + ` HEAD --not --remotes 2>/dev/null | wc -l | tr -d ' '); ` +
			`if [ "${c:-0}" -gt ` + strconv.Itoa(scanCommits) + ` ]; then printf '\036older %s\n' "$((c - ` + strconv.Itoa(scanCommits) + `))"; fi; ` +
			`git -c core.quotepath=off -c color.ui=never log --no-merges -n ` + strconv.Itoa(scanCommits) +
			` -p -U0 --no-ext-diff --no-textconv --no-renames --format='%x1ecommit %h' HEAD --not --remotes 2>/dev/null || printf '\036failed commits\n'; fi; `
	}
	return `cd ` + script.Quote(dir) + ` 2>/dev/null || { printf '\036failed folder\n'; exit 0; }; ` +
		`git rev-parse --show-toplevel >/dev/null 2>&1 || { printf '\036failed repository\n'; exit 0; }; ` +
		`base=HEAD; git rev-parse -q --verify HEAD >/dev/null 2>&1 || base=$(git hash-object -t tree /dev/null); ` +
		`{ ` + gitDiff + ` --relative "$base" 2>/dev/null || printf '\036failed work\n'; ` +
		untrackedScript +
		`printf '\036staged\n'; ` + gitDiff + ` --cached --relative "$base" 2>/dev/null || printf '\036failed work\n'; ` +
		commits + `printf '\036end\n'; } | ` + capScript
}
