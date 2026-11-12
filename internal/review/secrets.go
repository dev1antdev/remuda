package review

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/secrets"
	"github.com/dev1antdev/remuda/internal/shell"
)

// SecretScan is the secret check of a folder's work: what looks like a credential in what a commit of it would carry —
// and, before a pull request, in the commits a push would send — what could not be read, and how much was.
type SecretScan struct {
	Findings []secrets.Finding `json:"findings"`
	More     int               `json:"more,omitempty"` // findings past the caps
	Skipped  []Skipped         `json:"skipped,omitempty"`
	Files    int               `json:"files"`             // files of the work read
	Commits  int               `json:"commits,omitempty"` // commits read, for a push
	Push     bool              `json:"push,omitempty"`
	At       time.Time         `json:"at"`
}

// Skipped is what the check did not read, or not whole, and why.
type Skipped struct {
	Path  string `json:"path,omitempty"`
	Why   string `json:"why"`
	Count int    `json:"count,omitempty"` // the untracked files past the cap, for SkipMany
}

// Why a file was not read.
const (
	SkipBinary = "binary" // no text to read
	SkipLarge  = "large"  // an untracked file over scanPerFile, not read
	SkipPartly = "partly" // its changes ran past scanPerFile: the first part read
	SkipBudget = "budget" // past scanTotal in all, not read
	SkipMany   = "many"   // untracked files past scanUntracked
)

// The bounds of one check: what crosses from a remote machine stays a few megabytes, and the person waits seconds.
const (
	scanPerFile   = 1 << 20
	scanTotal     = 8 << 20
	scanUntracked = 2000
	scanCommits   = 200
	scanTimeout   = 45 * time.Second
)

// gitDiff is git's diff as the check reads it: no colour, no external driver or textconv, no renames, no context.
const gitDiff = `git -c core.quotepath=off -c color.ui=never diff --no-ext-diff --no-textconv --no-renames -U0`

// capScript cuts the stream of diffs on its way out: each file's at scanPerFile (a line "\036partly" says where), and
// every file once scanTotal is spent (each named on a line "\036budget <path>"); remuda's own lines pass.
var capScript = `LC_ALL=C awk -v per=` + strconv.Itoa(scanPerFile) + ` -v total=` + strconv.Itoa(scanTotal) + ` '
/^\036/ { print; next }
/^diff --git / { header = 1; n = 0; cut = 0; over = (sum > total); if (!over) print; next }
/^@@/ { header = 0 }
over { if (header && /^\+\+\+ /) print "\036budget " substr($0, 5); next }
cut { next }
{ size = length($0) + 1; n += size; sum += size; if (n > per) { cut = 1; print "\036partly"; next }; print }'`

// secretScript prints, in one stream through capScript, the diff of the work against HEAD (the empty tree before the
// first commit) with only the lines added, every untracked file git does not ignore as a diff from nothing, and, for a
// push, the commits on no remote-tracking branch with their patches, each after a line "\036commit <short sha>".
func secretScript(dir string, push bool) string {
	commits := ""
	if push {
		commits = `git -c core.quotepath=off -c color.ui=never log --no-merges -n ` + strconv.Itoa(scanCommits) +
			` -p -U0 --no-ext-diff --no-textconv --no-renames --relative --format='%x1ecommit %h' HEAD --not --remotes 2>/dev/null; `
	}
	return script.InRepo(dir) +
		`base=HEAD; git rev-parse -q --verify HEAD >/dev/null 2>&1 || base=$(git hash-object -t tree /dev/null); ` +
		`{ ` + gitDiff + ` --relative "$base" 2>/dev/null; ` +
		`u=$(git ls-files --others --exclude-standard 2>/dev/null | wc -l | tr -d ' '); ` +
		`if [ "${u:-0}" -gt ` + strconv.Itoa(scanUntracked) + ` ]; then printf '\036many %s\n' "$((u - ` + strconv.Itoa(scanUntracked) + `))"; fi; ` +
		`git -c core.quotepath=off ls-files --others --exclude-standard 2>/dev/null | head -n ` + strconv.Itoa(scanUntracked) + ` | ` +
		`while IFS= read -r f; do [ -f "$f" ] && [ ! -L "$f" ] || continue; ` +
		`s=$(wc -c < "$f" 2>/dev/null | tr -d ' '); ` +
		`if [ "${s:-0}" -gt ` + strconv.Itoa(scanPerFile) + ` ]; then printf '\036large %s\n' "$f"; continue; fi; ` +
		gitDiff + ` --no-index -- /dev/null "$f" 2>/dev/null; done; ` +
		commits + `} | ` + capScript
}

// ScanWork runs the secret check on a folder of the machine, in one round trip; push reads the unpushed commits too.
func ScanWork(host shell.Host, dir string, push bool) (SecretScan, error) {
	output, err := shell.Text(host, secretScript(dir, push), scanTimeout)
	if err != nil && strings.Contains(err.Error(), "timed out") { // ssh's own words quote the whole script
		return SecretScan{}, fmt.Errorf("it took longer than %s", scanTimeout)
	}
	if err != nil {
		return SecretScan{}, err
	}
	return readScan(output, push), nil
}

// readScan reads secretScript's stream: the lines each diff adds go to the scanner with their numbers in the new file.
func readScan(output string, push bool) SecretScan {
	scan := SecretScan{Push: push, At: time.Now()}
	collector := secrets.NewCollector()
	read := map[string]bool{}
	var file, commit string
	var number int
	header := false
	skip := func(path, why string) {
		if path != "" {
			scan.Skipped = append(scan.Skipped, Skipped{Path: path, Why: why})
		}
	}
	for rest := output; rest != ""; {
		var line string
		line, rest, _ = strings.Cut(rest, "\n")
		switch {
		case strings.HasPrefix(line, script.RS):
			word, value, _ := strings.Cut(line[len(script.RS):], " ")
			switch word {
			case "commit":
				commit, file, header = value, "", false
				scan.Commits++
			case SkipPartly:
				skip(file, SkipPartly)
				file = ""
			case SkipBudget:
				skip(pathOf(value), SkipBudget)
			case SkipLarge:
				skip(value, SkipLarge)
			case SkipMany:
				count, _ := strconv.Atoi(value)
				scan.Skipped = append(scan.Skipped, Skipped{Why: SkipMany, Count: count})
			}
		case strings.HasPrefix(line, "diff --git "):
			file, header = "", true
		case strings.HasPrefix(line, "@@ "):
			header, number = false, newStart(line)
		case header && strings.HasPrefix(line, "+++ "):
			file = pathOf(line[len("+++ "):])
			if file != "" && commit == "" {
				read[file] = true
			}
		case header && strings.HasPrefix(line, "Binary files ") && strings.HasSuffix(line, " differ"):
			path := pathOf(line[strings.LastIndex(line, " and ")+len(" and ") : len(line)-len(" differ")])
			if path != "" {
				collector.Binary(path, commit)
				if commit == "" && !secrets.KeyFile(path) { // a keystore is a finding, not a file left unread
					skip(path, SkipBinary)
				}
			}
		case !header && file != "" && strings.HasPrefix(line, "+"):
			collector.Line(file, number, line[1:], commit)
			number++
		case !header && strings.HasPrefix(line, " "):
			number++
		}
	}
	scan.Findings, scan.More, scan.Files = collector.Findings(), collector.More(), len(read)
	return scan
}

// pathOf is the file a diff header names, C-quoted by git when it holds an odd byte; "" for /dev/null.
func pathOf(named string) string {
	if strings.HasPrefix(named, `"`) {
		if unquoted, err := strconv.Unquote(named); err == nil {
			named = unquoted
		}
	}
	if named == "/dev/null" {
		return ""
	}
	return strings.TrimPrefix(named, "b/")
}

// newStart is the first line of the new file a hunk header names: "@@ -12,3 +14,5 @@" is 14.
func newStart(header string) int {
	_, after, ok := strings.Cut(header, " +")
	if !ok {
		return 0
	}
	end := strings.IndexAny(after, ", ")
	if end < 0 {
		end = len(after)
	}
	start, _ := strconv.Atoi(after[:end])
	return start
}
