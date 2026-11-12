package collisions

import (
	"strconv"
	"strings"

	"github.com/dev1antdev/remuda/internal/script"
)

// The bounds of one machine's reading: the hunks and the untracked files one work carries at most, so a work that
// rewrote a tree crosses from a remote as a few hundred kilobytes.
const (
	maxLines     = 6000 // lines of the diff's summary per work
	maxUntracked = 2000
)

// mainScript finds the repository's main branch in $m: the remote's default branch (origin/HEAD), else origin/main,
// origin/master, main or master — and the local branch of that name when it holds the remote's, since a main ahead of
// origin with commits not pushed yet is what every branch off it starts from, and those commits are no collision.
const mainScript = `m=$(git symbolic-ref -q --short refs/remotes/origin/HEAD 2>/dev/null); ` +
	`if [ -z "$m" ]; then for b in origin/main origin/master main master; do if git rev-parse -q --verify "$b^{commit}" >/dev/null 2>&1; then m=$b; break; fi; done; fi; ` +
	`l=${m#origin/}; if [ -n "$m" ] && [ "$l" != "$m" ] && git rev-parse -q --verify "refs/heads/$l" >/dev/null 2>&1 && git merge-base --is-ancestor "$m" "refs/heads/$l" 2>/dev/null; then m=$l; fi`

// diffAwk keeps of git's -U0 diff what the radar compares, one line each: "F <path>\t<blob>" for a file's new side
// (its old side for a deletion), "B <blob>\t<git's Binary line>", "H <old range> <new range>" per hunk; past maxLines
// a line "C" says the rest was not read. A file header's "---"/"+++" lines are told from a removed "-- comment" by
// the state: they come before the file's first hunk.
var diffAwk = `LC_ALL=C awk -v most=` + strconv.Itoa(maxLines) + ` '
function out(line) { if (++n > most) { print "C"; exit } print line }
/^diff --git / { h = 1; blob = ""; old = ""; next }
h && /^index / { split($2, ab, "\\.\\."); blob = ab[2]; next }
h && /^--- / { old = substr($0, 5); next }
h && /^\+\+\+ / { name = substr($0, 5); if (name == "/dev/null") name = old; out("F " name "\t" blob); next }
h && /^Binary files / { out("B " blob "\t" $0); next }
/^@@ / { h = 0; out("H " $2 " " $3); next }'`

// Script reads, in one round trip, every work of the folders given: per folder, the folder, its checkout's top, the
// main branch and the merge base with it, the diff's summary against that base — the branch's commits and the
// uncommitted work alike, paths from the top — and the untracked files git does not ignore. A folder outside git,
// or one whose repository has no main branch, answers its name alone.
func Script(dirs []string) string {
	var out strings.Builder
	for _, dir := range dirs {
		out.WriteString(`( printf '%s\036' ` + script.Quote(dir) + `; cd ` + script.Quote(dir) + ` 2>/dev/null && ` +
			`top=$(git rev-parse --show-toplevel 2>/dev/null) && cd "$top" && { ` + mainScript + `; ` +
			`if [ -n "$m" ] && mb=$(git merge-base HEAD "$m" 2>/dev/null); then ` +
			`printf '%s\037%s\037%s\n' "$top" "$m" "$mb"; ` +
			`git -c core.quotepath=off -c color.ui=never diff -U0 --no-ext-diff --no-textconv --no-renames --full-index "$mb" 2>/dev/null | ` + diffAwk + `; ` +
			`printf '\036\n'; git -c core.quotepath=off ls-files --others --exclude-standard 2>/dev/null | head -n ` + strconv.Itoa(maxUntracked) + `; ` +
			`fi; }; printf '\035\n' ); `)
	}
	return out.String()
}

// Parse reads Script's answer into the works of a machine; a folder that answered its name alone has none.
func Parse(machineID, output string) []Work {
	var works []Work
	for _, record := range strings.Split(output, script.GS+"\n") {
		dir, rest, ok := strings.Cut(record, script.RS)
		if !ok || rest == "" {
			continue
		}
		head, rest, _ := strings.Cut(rest, "\n")
		fields := script.Fields(head)
		if len(fields) != 3 || fields[0] == "" {
			continue
		}
		work := Work{Machine: machineID, Dir: strings.TrimLeft(dir, "\n"), Top: fields[0], Main: fields[1], Base: fields[2], Files: map[string]Change{}}
		diff, untracked, _ := strings.Cut(rest, script.RS+"\n")
		readDiff(&work, diff)
		for _, line := range script.Lines(untracked) {
			if path := unquote(line); path != "" {
				work.Files[path] = Change{Whole: true}
			}
		}
		works = append(works, work)
	}
	return works
}

// readDiff reads the awk's summary of one work's diff.
func readDiff(work *Work, diff string) {
	path := ""
	for _, line := range script.Lines(diff) {
		kind, value, _ := strings.Cut(line, " ")
		switch kind {
		case "F":
			named, blob := lastField(value)
			path = sidePath(named)
			if path != "" {
				work.Files[path] = Change{Blob: blobOf(blob)}
			}
		case "B":
			blob, gitLine := firstField(value)
			path = binaryPath(gitLine)
			if path != "" {
				work.Files[path] = Change{Blob: blobOf(blob), Whole: true}
			}
		case "H":
			if path == "" {
				continue
			}
			old, added, _ := strings.Cut(value, " ")
			start, count := rangeOf(strings.TrimPrefix(old, "-"))
			_, size := rangeOf(strings.TrimPrefix(added, "+"))
			change := work.Files[path]
			change.Hunks = append(change.Hunks, Hunk{Start: start, Count: count, Size: count + size})
			work.Files[path] = change
			work.Lines += count + size
		case "C":
			work.Cut = true
		}
	}
}

// rangeOf reads one side of a hunk header, "12,3" or "12" (one line).
func rangeOf(spec string) (int, int) {
	startText, countText, hasCount := strings.Cut(spec, ",")
	start, _ := strconv.Atoi(startText)
	count := 1
	if hasCount {
		count, _ = strconv.Atoi(countText)
	}
	return start, count
}

// Gone is the blob of a file deleted: two works deleting one file merge cleanly, as two leaving it the same blob do.
const Gone = "gone"

// blobOf is a blob git names, Gone for the null one a deletion leaves, "" for none.
func blobOf(blob string) string {
	switch {
	case blob == "":
		return ""
	case strings.Trim(blob, "0") == "":
		return Gone
	}
	return blob
}

func lastField(value string) (string, string) {
	if at := strings.LastIndex(value, "\t"); at >= 0 {
		return value[:at], value[at+1:]
	}
	return value, ""
}

func firstField(value string) (string, string) {
	if before, after, ok := strings.Cut(value, "\t"); ok {
		return before, after
	}
	return "", value
}

// sidePath is the file a "---" or "+++" line names: C-quoted by git when it holds an odd byte, followed by a tab when
// it holds a blank, its a/ or b/ taken off; "" for /dev/null.
func sidePath(named string) string {
	named = unquote(strings.TrimSuffix(named, "\t"))
	if named == "/dev/null" {
		return ""
	}
	if strings.HasPrefix(named, "a/") || strings.HasPrefix(named, "b/") {
		return named[2:]
	}
	return named
}

func unquote(named string) string {
	if strings.HasPrefix(named, `"`) {
		if plain, err := strconv.Unquote(named); err == nil {
			return plain
		}
	}
	return named
}

// binaryPath is the file git's "Binary files a/x and b/x differ" names: with no renames both sides name the same
// path, so the second starts halfway whatever " and " the path itself holds; a deletion's is its old side.
func binaryPath(line string) string {
	pair := strings.TrimSuffix(strings.TrimPrefix(line, "Binary files "), " differ")
	if added, ok := strings.CutPrefix(pair, "/dev/null and "); ok {
		return sidePath(added)
	}
	if removed, ok := strings.CutSuffix(pair, " and /dev/null"); ok {
		return sidePath(removed)
	}
	return sidePath(pair[(len(pair)+len(" and "))/2:])
}
