package review

import (
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
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
	// Incomplete says text was left unread, so finding nothing does not clear it; a binary file has none to read, unless an
	// attribute may have called a text file binary (SkipMarked).
	Incomplete bool      `json:"incomplete,omitempty"`
	Files      int       `json:"files"`             // files of the work read
	Commits    int       `json:"commits,omitempty"` // commits read, for a push
	Push       bool      `json:"push,omitempty"`
	At         time.Time `json:"at"`
}

// Skipped is what the check did not read, or not whole, and why.
type Skipped struct {
	Path  string `json:"path,omitempty"`
	Why   string `json:"why"`
	Count int    `json:"count,omitempty"` // the untracked files past the cap for SkipMany, the commits for SkipOlder
	// InCommit says it is part of the commits a push would send, not of the work beside them.
	InCommit bool `json:"in_commit,omitempty"`
}

// PushUnread says the commits a push would send were not read whole; the work beside them does not go with it.
func (s SecretScan) PushUnread() bool {
	return slices.ContainsFunc(s.Skipped, func(skip Skipped) bool { return skip.InCommit })
}

// Why a file was not read.
const (
	SkipBinary = "binary" // no text to read
	SkipLarge  = "large"  // an untracked file over scanPerFile, not read
	SkipPartly = "partly" // its changes ran past scanPerFile: the first part read
	SkipBudget = "budget" // past scanTotal, for the work or for the commits, not read
	SkipMany   = "many"   // untracked files past scanUntracked
	SkipOlder  = "older"  // commits not pushed yet past scanCommits, for a push
	SkipMarked = "marked" // what git calls binary where an attribute the check cannot set aside may be hiding text
)

// The bounds of one check: what crosses from a remote machine stays a few megabytes, and the person waits seconds.
const (
	scanPerFile   = 1 << 20
	scanTotal     = 8 << 20
	scanUntracked = 2000
	scanCommits   = 200
	countCommits  = 100000 // how far the commits past scanCommits are counted
	scanTimeout   = 45 * time.Second
)

// ScanWork runs the secret check on a folder of the machine, in one round trip; push reads the unpushed commits too.
func ScanWork(host shell.Host, dir string, push bool) (SecretScan, error) {
	output, err := shell.Text(host, secretScript(dir, push), scanTimeout)
	if err != nil && strings.Contains(err.Error(), "timed out") { // ssh's own words quote the whole script
		return SecretScan{}, fmt.Errorf("it took longer than %s", scanTimeout)
	}
	if err != nil {
		return SecretScan{}, err
	}
	return readScan(output, push)
}

// failures are the words of a stream's "\036failed <what>": a check git could not finish is no clean one.
var failures = map[string]string{
	"folder":     "the folder is not there",
	"repository": "the folder is not a git checkout",
	"work":       "git could not read the changes",
	"commits":    "git could not read the commits not pushed yet",
}

// errUnfinished is a stream that stopped before its end: a script cut off, or one that never ran.
var errUnfinished = errors.New("it stopped before it had read everything")

// errUnreadable is a stream with a line no diff holds: what cannot be read has not been checked.
var errUnreadable = errors.New("git's answer could not be read")

// readScan reads secretScript's stream: the lines each diff adds, and every line of an untracked file, go to the
// scanner with their numbers in the new file. A stream that says git failed, or that never reached its end, is an
// error, never a scan that found nothing.
func readScan(output string, push bool) (SecretScan, error) {
	scan := SecretScan{Push: push, At: time.Now()}
	collector := secrets.NewCollector()
	read := map[string]bool{}
	var file, commit string
	var number int
	header, ended, attrs := false, false, false
	converted := map[string]bool{} // converted in the working tree: its bytes alone say nothing of what a commit carries
	skipped := map[Skipped]bool{}  // a file the work and the index both carry is said once
	skip := func(path, why string) {
		entry := Skipped{Path: path, Why: why, InCommit: commit != "" && commit != secrets.Staged}
		if path != "" && !skipped[entry] {
			skipped[entry] = true
			scan.Skipped = append(scan.Skipped, entry)
		}
	}
	binary := func(path string) {
		if path == "" {
			return
		}
		collector.Binary(path, commit)
		switch {
		case secrets.KeyFile(path): // a keystore is a finding, not a file left unread
		case attrs:
			skip(path, SkipMarked)
		case commit == "" || commit == secrets.Staged:
			skip(path, SkipBinary)
		}
	}
	for rest := output; rest != ""; {
		var line string
		line, rest, _ = strings.Cut(rest, "\n")
		switch {
		case strings.HasPrefix(line, script.RS):
			word, value, _ := strings.Cut(line[len(script.RS):], " ")
			switch word {
			case "attrs": // an attribute may be hiding text in what git calls binary: those files count as unread
				attrs = true
			case "converts": // check-attr --all -z's triplets, path, attribute and value, in hex, then "ok"
				answer, err := hex.DecodeString(value)
				if err != nil {
					return SecretScan{}, errUnreadable
				}
				triplets, ok := strings.CutSuffix(string(answer), "ok")
				if !ok {
					return SecretScan{}, errors.New(failures["work"])
				}
				// a value is no help: a filter may be named "unset", the word git says for one taken away
				for fields := strings.Split(triplets, "\x00"); len(fields) >= 3; fields = fields[3:] {
					if fields[1] == "filter" || fields[1] == "working-tree-encoding" {
						converted[fields[0]] = true
					}
				}
			case "commit":
				commit, file, header = value, "", false
				scan.Commits++
			case secrets.Staged: // the index's lines follow: what the work already showed is said once
				commit, file, header = secrets.Staged, "", false
			case "file": // an untracked file's lines follow, numbered from the first
				file, header, number = value, false, 1
				read[file] = true
			case "binary":
				binary(value)
			case "binarymerge": // what a merge brought in it cannot be told: a keystore is a finding, the rest stays unread
				binary(pathOf(value))
				if path := pathOf(value); path != "" && !secrets.KeyFile(path) && !attrs {
					skip(path, SkipBinary)
				}
			case "binarydiff": // past the budget, still named: a keystore is a finding
				binary(binaryPath(value))
			case SkipPartly:
				skip(file, SkipPartly)
				file = ""
			case SkipBudget:
				skip(pathOf(value), SkipBudget)
			case "unread":
				skip(value, SkipBudget)
			case SkipLarge:
				skip(value, SkipLarge)
			case SkipMany, SkipOlder:
				count, _ := strconv.Atoi(value)
				scan.Skipped = append(scan.Skipped, Skipped{Why: word, Count: count, InCommit: word == SkipOlder})
			case "failed":
				if why, known := failures[value]; known {
					return SecretScan{}, errors.New(why)
				}
				return SecretScan{}, errUnfinished
			case "end":
				ended = true
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
			binary(binaryPath(line))
		case !header && file != "" && strings.HasPrefix(line, "+"):
			collector.Line(file, number, line[1:], commit)
			number++
		case !header && strings.HasPrefix(line, " "):
			number++
		case strings.HasPrefix(line, "* Unmerged path "): // the index's word on a conflict: the work's copy was read above
		case !header && line != "" && !strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "-") && !strings.HasPrefix(line, `\`):
			return SecretScan{}, errUnreadable
		}
	}
	if !ended {
		return SecretScan{}, errUnfinished
	}
	// the work's second read settles what the first called binary: a converted file stays unread whatever its bytes say,
	// the rest is read as text or binary indeed
	for i, skip := range scan.Skipped {
		if skip.Why == SkipBinary && !skip.InCommit && converted[skip.Path] {
			scan.Skipped[i].Why = SkipMarked
		}
	}
	scan.Skipped = slices.DeleteFunc(scan.Skipped, func(skip Skipped) bool { return skip.Why == SkipBinary && !skip.InCommit && read[skip.Path] })
	scan.Findings, scan.More, scan.Files = collector.Findings(), collector.More(), len(read)
	scan.Incomplete = slices.ContainsFunc(scan.Skipped, func(skip Skipped) bool { return skip.Why != SkipBinary || skip.InCommit })
	return scan, nil
}

// pathOf is the file a diff header names, C-quoted by git when it holds an odd byte and followed by a tab when it
// holds a blank; "" for /dev/null.
func pathOf(named string) string {
	named = strings.TrimSuffix(named, "\t")
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

// binaryPath is the new file git's "Binary files a/x and b/x differ" names: with no renames both sides name the same
// path, so the second starts halfway, whatever " and " the path itself holds; "" for a file deleted.
func binaryPath(line string) string {
	pair := strings.TrimSuffix(strings.TrimPrefix(line, "Binary files "), " differ")
	if added, ok := strings.CutPrefix(pair, "/dev/null and "); ok {
		return pathOf(added)
	}
	if strings.HasSuffix(pair, " and /dev/null") {
		return ""
	}
	return pathOf(pair[(len(pair)+len(" and "))/2:])
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
