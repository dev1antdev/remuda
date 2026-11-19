package review

import (
	"strconv"

	"github.com/dev1antdev/remuda/internal/script"
)

// plainPatch is a patch in the one form readScan reads: no renames, no context.
const plainPatch = script.PatchForm + ` --no-renames -U0`

// noAttrs is what makes git judge a file by its bytes alone, whatever an attribute (*.env -diff) says of it; $A is set by
// attrsScript.
const noAttrs = `$A ` + ownAttrs

// ownAttrs keeps the person's own attributes file and a configured default diff driver from calling text binary.
const ownAttrs = `-c core.attributesFile=/dev/null -c diff.default.binary=auto`

// attrsMark is what, in an attributes file, can make git call a text file binary.
const attrsMark = `(^|[[:space:]])(binary|-diff|!diff)([[:space:]]|$)`

// attrsScript points git's attributes at the empty tree where it can (--attr-source, git 2.41). What that leaves, the
// repository's info/attributes and on an older git every .gitattributes, tracked or not, in the index too, is said with a line "\036attrs"
// when it marks anything, a diff driver the configuration calls binary included: the files git then calls binary may be
// text the check cannot read.
var attrsScript = `A=; e=$(git hash-object -t tree /dev/null); i=$(git rev-parse --git-path info/attributes 2>/dev/null); ` +
	`if git --attr-source="$e" rev-parse --git-dir >/dev/null 2>&1; then A="--attr-source=$e"; fi; ` +
	`m='` + attrsMark + `'; if [ -n "$(git config --get-regexp '^diff\..*\.binary$' 2>/dev/null)" ]; then m="$m|(^|[[:space:]])diff="; fi; ` +
	`attrs=; grep -E "$m" "$i" 2>/dev/null | grep -qvE '^[[:space:]]*#' && attrs=1; ` +
	`if [ -z "$A" ] && { ` + readGit + ` grep --untracked --no-exclude-standard -hE "$m" -- ':(top,glob)**/.gitattributes' 2>/dev/null; ` +
	readGit + ` grep --cached -hE "$m" -- ':(top,glob)**/.gitattributes' 2>/dev/null; } | grep -qvE '^[[:space:]]*#'; then attrs=1; fi; `

// gitDiff is git's diff as the check reads it, writing nothing; each call says itself whether it is relative to the folder.
const gitDiff = readGit + ` ` + noAttrs + ` -c diff.relative=false diff ` + plainPatch

// attrsDiff is the diff with the repository's attributes, which say how a working-tree-encoding file decodes.
const attrsDiff = readGit + ` ` + ownAttrs + ` -c diff.relative=false diff ` + plainPatch

// workDiff is the work against HEAD, read with the repository's attributes, and what they leave binary once more without
// them (retryScript).
var workDiff = attrsDiff + ` --relative "$base" 2>/dev/null || printf '\036failed work\n'; ` +
	`if [ -n "$A" ]; then export A base; { ` + attrsDiff + ` --numstat -z --relative "$base" || printf 'failed\000'; } 2>/dev/null | ` +
	`xargs -0 -r sh -c ` + script.Quote(retryScript) + ` sh; fi; `

// retryScript is given, in xargs's batches, the records of --numstat -z. The files git called binary ("-\t-\t<path>")
// it reads again without the repository's attributes, first saying which of them an attribute converts in the working
// tree (working-tree-encoding, filter): read by their bytes alone they may be binary to git, or other text, than what a
// commit carries. git check-attr -z's answer comes coded in hex, so that no name breaks the stream ("\036converts <hex>").
var retryScript = `t=$(printf '\t'); for r; do case $r in failed) printf '\036failed work\n';; "-$t-$t"*) set -- "$@" "${r#-$t-$t}";; esac; shift; done; ` +
	`[ $# -gt 0 ] || exit 0; ` +
	`h=$(printf '%s\000' "$@" | ` + readGit + ` check-attr -z --stdin working-tree-encoding filter 2>/dev/null | od -An -v -tx1 | tr -d ' \t\n'); ` +
	`if [ -n "$h" ]; then printf '\036converts %s\n' "$h"; else printf '\036failed work\n'; fi; ` +
	`for p; do shift; set -- "$@" ":(literal)$p"; done; ` +
	gitDiff + ` --relative "$base" -- "$@" 2>/dev/null || printf '\036failed work\n'`

// capScript cuts the stream on its way out: a diff's removed lines go, nothing to find in them, and each file's lines at scanPerFile (a line "\036partly" says where), and
// every file once scanTotal is spent, the commits having a scanTotal of their own so the work beside them cannot use it up (each named on a line "\036budget <path>", "\036unread <path>" for an untracked
// one, and a binary one passes as "\036binarydiff <git's line>"); remuda's own lines pass. Past a line "\036untracked" come git grep's lines of the untracked files, path, line
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
/^\036commit / { if (!commits) { commits = 1; sum = 0 } }
/^\036/ { grep = 0; print; next }
grep { untracked(); next }
/^diff --git / { header = 1; n = 0; cut = 0; over = (sum > total); if (!over) print; next }
/^@@/ { header = 0 }
!header && /^-/ { next }
over { if (header && /^\+\+\+ /) print "\036budget " substr($0, 5); else if (header && /^Binary files /) print "\036binarydiff " $0; next }
cut { next }
{ size = length($0) + 1; n += size; sum += size; if (n > per) { cut = 1; print "\036partly"; next }; print }'`

// untrackedScript prints the untracked files git does not ignore, the first scanUntracked of them, in one git grep —
// one process for them all, never one per file: two thousand of them took twenty seconds on a Mac — after a line
// "\036untracked"; those past the count are counted ("\036many N"), a file over scanPerFile is named and not read
// ("\036large <path>", or "\036binary <path>" when its first bytes hold a NUL, as git tells), and a link or a folder (a repository inside) is passed over. The names travel NUL-separated
// to find and git, so a blank, a quote or a leading dash in one is no trouble; one holding a line break is lost.
var untrackedScript = `list=$(` + readGit + ` ls-files -z --others --exclude-standard 2>/dev/null | tr '\0' '\n'); ` +
	`if [ -n "$list" ]; then ` +
	`u=$(printf '%s\n' "$list" | wc -l | tr -d ' '); ` +
	`if [ "${u:-0}" -gt ` + strconv.Itoa(scanUntracked) + ` ]; then printf '\036many %s\n' "$((u - ` + strconv.Itoa(scanUntracked) + `))"; fi; ` +
	`names=$(printf '%s\n' "$list" | head -n ` + strconv.Itoa(scanUntracked) + ` | sed 's|^|./|'); ` +
	`printf '%s\n' "$names" | tr '\n' '\0' | xargs -0 -r sh -c '[ $# -gt 0 ] && find "$@" -prune -type f -size +` + strconv.Itoa(scanPerFile) + `c' sh 2>/dev/null | ` +
	`while IFS= read -r f; do if [ "$(head -c 8000 "$f" 2>/dev/null | LC_ALL=C tr -dc '\000' | wc -c | tr -d ' ')" -gt 0 ]; then printf '\036binary %s\n' "${f#./}"; else printf '\036large %s\n' "${f#./}"; fi; done; ` +
	`printf '\036untracked\n'; ` +
	`printf '%s\n' "$names" | tr '\n' '\0' | xargs -0 -r sh -c '[ $# -gt 0 ] && find "$@" -prune -type f ! -size +` + strconv.Itoa(scanPerFile) + `c -print0' sh 2>/dev/null | ` +
	`LC_ALL=C xargs -0 -r git --literal-pathspecs ` + noAttrs + ` -c grep.fullName=false -c grep.column=false -c color.ui=never ` +
	`grep --untracked --no-color -G -n -z -e '' -- 2>/dev/null | tr '\0' '\037'; ` +
	`fi; `

// unpushedLog is git's log of the commits a push would send, in the form given.
func unpushedLog(form string) string {
	return readGit + ` ` + noAttrs + ` -c diff.relative=false -c log.showRoot=true -c log.showSignature=false log -n ` + strconv.Itoa(scanCommits) + ` ` + form +
		` HEAD --not --remotes 2>/dev/null`
}

// mergeLines turns git's combined diff of the merges into the plain diff the rest of the stream reads. A line marked
// added against every parent is what the merge brought itself, a conflict's resolution or anything else, and stays
// added; a line one parent already had is context, since that parent's commit is read or already out; a line the result
// lacks goes. A file a parent holds as binary has no lines to tell, and is named ("\036binarymerge <path>") unless the
// merge deleted it.
const mergeLines = `LC_ALL=C awk '
/^\036/ { n = 0; print; next }
/^diff --cc / { n = 0; gone = 0; path = substr($0, 11); print "diff --git " path " " path; next }
/^deleted file mode / { gone = 1 }
/^Binary files differ$/ { if (!gone) print "\036binarymerge " path; next }
/^@@@+ / { match($0, /^@+/); n = RLENGTH - 1; start = $(n + 2); sub(/^\+/, "", start); sub(/,.*/, "", start); print "@@ -0,0 +" start " @@"; next }
n == 0 || length($0) < n || /^\\/ { if (n == 0) print; next }
{ mark = substr($0, 1, n); if (index(mark, "-")) next
  text = substr($0, n + 1); gsub(/\+/, "", mark); print (mark == "" ? "+" : " ") text }'`

// unpushedScript prints the commits a push would send, those with one parent as patches and the merges by what they
// brought themselves (mergeLines): a merge that only merged shows nothing. A combined diff merges nothing again, so no
// driver runs. It reads the parents' files, which a partial clone may lack and a git before 2.45 would fetch whatever it
// is told: there the merges are left unread, counted with the commits past the limit.
var unpushedScript = `o=0; m=0; for kind in --no-merges --merges; do ` +
	`c=$(git rev-list --count "$kind" --max-count=` + strconv.Itoa(countCommits) + ` HEAD --not --remotes 2>/dev/null) || printf '\036failed commits\n'; ` +
	`if [ "$kind" = --merges ]; then m=${c:-0}; fi; ` +
	`if [ "${c:-0}" -gt ` + strconv.Itoa(scanCommits) + ` ]; then o=$((o + c - ` + strconv.Itoa(scanCommits) + `)); fi; done; ` +
	`skip=; if [ "$m" -gt 0 ] && [ -n "$(git config --get-regexp '^(remote\..*\.(promisor|partialclonefilter)|extensions\.partialclone)$' 2>/dev/null)" ] && ! git --no-lazy-fetch version >/dev/null 2>&1; then ` +
	`skip=1; if [ "$m" -gt ` + strconv.Itoa(scanCommits) + ` ]; then m=` + strconv.Itoa(scanCommits) + `; fi; o=$((o + m)); fi; ` +
	`if [ "$o" -gt 0 ]; then printf '\036older %s\n' "$o"; fi; ` +
	unpushedLog(`--no-merges -p `+plainPatch+` --format='%x1ecommit %h'`) + ` || printf '\036failed commits\n'; ` +
	`if [ -z "$skip" ]; then { ` + unpushedLog(`--merges --cc `+plainPatch+` --format='%x1ecommit %h'`) + ` || printf '\036failed commits\n'; } | ` + mergeLines + ` || printf '\036failed commits\n'; fi; `

// secretScript prints, in one stream through capScript, the diff of the work against HEAD (the empty tree before the
// first commit) with only the lines added, every untracked file git does not ignore (untrackedScript), after a line
// "\036staged" the index's diff against HEAD — what git commit carries, though a staged file may have changed or gone
// since — and, for a push, the commits on no remote-tracking branch with their whole patches (unpushedScript) — a push
// sends the whole commit, whatever folder the work is in — each after a line "\036commit <short sha>", those past
// scanCommits counted ("\036older N").
// A line "\036failed <what>" says git could not read the folder, the work or the commits, and "\036end" closes a
// stream that ran to its end: without it nothing is known, and the check is no clean one.
func secretScript(dir string, push bool) string {
	commits := ""
	if push {
		commits = `if [ "$base" = HEAD ]; then ` + unpushedScript + `fi; `
	}
	// a partial clone is not sent for what it lacks (git 2.45 on): nothing is written, and git's failure says so
	// nor are the machine's own attributes read (GIT_ATTR_NOSYSTEM), which nothing here could set aside
	return `GIT_NO_LAZY_FETCH=1; GIT_ATTR_NOSYSTEM=1; export GIT_NO_LAZY_FETCH GIT_ATTR_NOSYSTEM; cd ` + script.Quote(dir) + ` 2>/dev/null || { printf '\036failed folder\n'; exit 0; }; ` +
		`git rev-parse --show-toplevel >/dev/null 2>&1 || { printf '\036failed repository\n'; exit 0; }; ` +
		`base=HEAD; git rev-parse -q --verify HEAD >/dev/null 2>&1 || base=$(git hash-object -t tree /dev/null); ` +
		attrsScript +
		`{ if [ -n "$attrs" ]; then printf '\036attrs\n'; fi; ` + workDiff +
		untrackedScript +
		`printf '\036staged\n'; ` + gitDiff + ` --cached --relative "$base" 2>/dev/null || printf '\036failed work\n'; ` +
		// a folder inside the repository commits the whole index: what is staged outside it is read too, named from the top
		`p=$(git rev-parse --show-prefix 2>/dev/null); if [ -n "$p" ]; then ` + gitDiff + ` --cached "$base" -- ':(top)' ":(top,literal,exclude)$p" 2>/dev/null || printf '\036failed work\n'; fi; ` +
		commits + `printf '\036end\n'; } | ` + capScript
}
