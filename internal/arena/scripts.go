package arena

import (
	"strconv"
	"strings"

	"github.com/dev1antdev/remuda/internal/script"
)

// The lanes' work is read against the commit each started from, through a throwaway index: the worktree's whole
// state — its commits, what is staged, what is not and its new files, .gitignore kept — and never the lane's own index.

// HeadScript prints the commit a fresh worktree stands on.
func HeadScript(dir string) string {
	return script.PATH + "\ncd " + script.Quote(dir) + " 2>/dev/null || exit 3\ngit rev-parse HEAD\n"
}

// throwaway stages the worktree in an index of its own, over the base's tree: $t is the index inside the temporary
// folder $d, which the caller removes through cleanup. Without a folder nothing else runs.
func throwaway(base string) string {
	return `d=$(mktemp -d "${TMPDIR:-/tmp}/remuda-arena.XXXXXX") && t="$d/index" && ` +
		`GIT_INDEX_FILE="$t" git read-tree ` + script.Quote(base) + ` 2>/dev/null && GIT_INDEX_FILE="$t" git add -A 2>/dev/null`
}

// cleanup removes throwaway's folder, and nothing at all when mktemp made none.
const cleanup = `if [ -n "$d" ]; then rm -rf "$d"; fi`

// Stat is a lane's work against its base: lines added and removed, and files touched.
type Stat struct {
	Added   int `json:"added"`
	Removed int `json:"removed"`
	Files   int `json:"files"`
}

// StatsScript prints, for every lane, its id and then git's numstat of its work, a section each.
func StatsScript(lanes []Lane) string {
	var b strings.Builder
	b.WriteString(script.PATH + "\n")
	for _, l := range lanes {
		if l.Dir == "" || l.Base == "" {
			continue
		}
		b.WriteString(`( printf '%s\n' ` + strconv.Itoa(l.ID) + `; cd ` + script.Quote(l.Dir) + ` 2>/dev/null && { ` + throwaway(l.Base) +
			` && GIT_INDEX_FILE="$t" git diff --cached --numstat ` + script.Quote(l.Base) + ` 2>/dev/null; ` + cleanup + `; }; printf '\036\n' ); `)
	}
	return b.String()
}

// ParseStats reads StatsScript's sections into each lane's numbers; a binary file counts as a file with no lines.
func ParseStats(out string) map[int]Stat {
	stats := map[int]Stat{}
	for _, section := range strings.Split(out, script.RS+"\n") {
		lines := strings.Split(strings.TrimSpace(section), "\n")
		id, err := strconv.Atoi(strings.TrimSpace(lines[0]))
		if err != nil {
			continue
		}
		var s Stat
		for _, line := range lines[1:] {
			fields := strings.SplitN(line, "\t", 3)
			if len(fields) != 3 {
				continue
			}
			s.Files++
			added, _ := strconv.Atoi(fields[0])
			removed, _ := strconv.Atoi(fields[1])
			s.Added += added
			s.Removed += removed
		}
		stats[id] = s
	}
	return stats
}

// PatchScript prints the lane's whole work against its base as a binary patch git apply takes back: through the
// plumbing diff-index, which no external diff tool, textconv, colour or prefix setting of the person's changes.
func PatchScript(dir, base string) string {
	return script.PATH + "\ncd " + script.Quote(dir) + " 2>/dev/null || { echo 'the worktree is not there'; exit 3; }\n" +
		throwaway(base) + " || { " + cleanup + "; echo 'git could not read the work'; exit 4; }\n" +
		`GIT_INDEX_FILE="$t" git diff-index --cached -p --binary ` + script.Quote(base) + " --; status=$?; " + cleanup + "; exit $status\n"
}
