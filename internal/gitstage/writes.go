package gitstage

import (
	"regexp"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// writeFor is how long an index write may take; a commit runs the repository's own hooks and gets longer.
const (
	writeFor  = 60 * time.Second
	commitFor = 2 * time.Minute
)

// MaxMessage is the longest commit message taken: it travels inside the script, over ssh for a remote machine.
const MaxMessage = 16 * 1024

// Done is what a write changed: the files it touched, and the ones it passed over, a conflict each.
type Done struct {
	Paths   []string `json:"paths"`
	Skipped []string `json:"skipped,omitempty"`
}

// split is the paths a write takes and the conflicted ones it leaves: a bulk action passes over a conflict, since
// staging one marks it resolved with its markers still in and unstaging one throws away git's record of it.
func split(paths []string, conflicts map[string]bool) (take, skipped []string) {
	for _, path := range paths {
		if conflicts[path] {
			skipped = append(skipped, path)
		} else {
			take = append(take, path)
		}
	}
	return take, skipped
}

// Stage adds the files to the index as the working tree holds them, a deletion as a deletion. A conflicted file is
// staged only on its own (bulk false), as its resolution, and only once no conflict marker is left in it.
func (s *Service) Stage(host shell.Host, dir string, paths []string, bulk bool) (Done, error) {
	if err := checkPaths(paths); err != nil {
		return Done{}, err
	}
	found, release, err := s.prepare(host, dir)
	if err != nil {
		return Done{}, err
	}
	defer release()
	take, skipped := paths, []string(nil)
	if bulk {
		take, skipped = split(paths, found.conflicts)
	} else if found.conflicts[paths[0]] {
		output, _ := shell.Text(host, script.InDir(dir)+`grep -c -E '^(<<<<<<<|>>>>>>>)( |$)' < `+script.Quote(paths[0])+` 2>/dev/null`, writeFor)
		if n := strings.TrimSpace(output); n != "" && n != "0" {
			return Done{}, &Refusal{Why: paths[0] + " still has conflict markers: resolve every block first", Paths: paths[:1]}
		}
	}
	if len(take) == 0 {
		return Done{Paths: []string{}, Skipped: skipped}, &Refusal{Why: "every file named is in conflict", Paths: skipped}
	}
	if _, err := run(host, dir, inChunks(writeGit+" add", take), writeFor); err != nil {
		return Done{}, err
	}
	return Done{Paths: take, Skipped: skipped}, nil
}

// Unstage takes the files out of the index back to HEAD, the working tree left as it is; before the first commit, out
// of the index altogether. A conflicted file is never unstaged.
func (s *Service) Unstage(host shell.Host, dir string, paths []string, bulk bool) (Done, error) {
	if err := checkPaths(paths); err != nil {
		return Done{}, err
	}
	found, release, err := s.prepare(host, dir)
	if err != nil {
		return Done{}, err
	}
	defer release()
	take, skipped := split(paths, found.conflicts)
	if !bulk && len(skipped) > 0 {
		return Done{}, &Refusal{Why: skipped[0] + " is in conflict: resolve it in the file, then stage it", Paths: skipped}
	}
	if len(take) == 0 {
		return Done{Paths: []string{}, Skipped: skipped}, &Refusal{Why: "every file named is in conflict", Paths: skipped}
	}
	command := writeGit + " reset -q HEAD"
	if !found.hasHead {
		command = writeGit + " rm -q --cached -r -f"
	}
	if _, err := run(host, dir, inChunks(command, take), writeFor); err != nil {
		return Done{}, err
	}
	return Done{Paths: take, Skipped: skipped}, nil
}

// What a discard throws away.
const (
	DiscardUnstaged  = "unstaged"  // the working tree's edits, back to what the index holds
	DiscardAll       = "all"       // staged and not, back to HEAD; a file HEAD does not have is deleted
	DiscardUntracked = "untracked" // a file git never tracked, deleted
)

// discardScripts is each kind's git, for one quoted path: the untracked file checked to be untracked and a file before
// git clean takes it, the one picked and nothing else.
var discardScripts = map[string]func(path string) string{
	DiscardUnstaged: func(path string) string {
		return writeGit + ` ls-files --error-unmatch -- ` + path + ` >/dev/null || exit 1; ` + writeGit + ` checkout -q -- ` + path
	},
	DiscardAll: func(path string) string {
		return `if git cat-file -e HEAD:./` + path + ` 2>/dev/null; then ` + writeGit + ` checkout -q HEAD -- ` + path + `; ` +
			`else ` + writeGit + ` ls-files --error-unmatch -- ` + path + ` >/dev/null || exit 1; ` + writeGit + ` rm -q -f -- ` + path + `; fi`
	},
	DiscardUntracked: func(path string) string {
		return `if ` + writeGit + ` ls-files --error-unmatch -- ` + path + ` >/dev/null 2>&1; then echo "git tracks this file"; exit 1; fi; ` +
			`[ -f ` + path + ` ] || [ -L ` + path + ` ] || { echo "not a file"; exit 1; }; ` + writeGit + ` clean -f -q -- ` + path
	},
}

// Discard throws one file's changes away, as kind says; never a conflicted file.
func (s *Service) Discard(host shell.Host, dir, path, kind string) error {
	discard, ok := discardScripts[kind]
	if !ok {
		return &Refusal{Why: "a discard is of the unstaged changes, all of them, or an untracked file"}
	}
	if err := checkPaths([]string{path}); err != nil {
		return err
	}
	found, release, err := s.prepare(host, dir)
	if err != nil {
		return err
	}
	defer release()
	if found.conflicts[path] {
		return &Refusal{Why: path + " is in conflict: resolve it in the file first", Paths: []string{path}}
	}
	_, err = run(host, dir, discard(script.Quote(path)), writeFor)
	return err
}

// CommitRequest is a commit as the person asked for it: the message, the files to stage first (Commit all), and for an
// amend the HEAD it was chosen on.
type CommitRequest struct {
	Message string   `json:"message"`
	Stage   []string `json:"stage,omitempty"`
	Amend   bool     `json:"amend,omitempty"`
	Head    string   `json:"head,omitempty"`
}

// Committed is the commit made.
type Committed struct {
	SHA     string   `json:"sha"`
	Short   string   `json:"short"`
	Subject string   `json:"subject"`
	Skipped []string `json:"skipped,omitempty"` // files Commit all passed over, each in conflict
}

var headRe = regexp.MustCompile(`^[0-9a-f]{40,64}$`)

// The words a commit script says when it stops on its own, never git's.
const (
	movedMark   = "\x1emoved"
	nothingMark = "\x1enothing"
)

// Commit records what is staged — with Stage, those files staged first — as a new commit, or as the last one amended
// when HEAD is still the one the amend was chosen on; never while a file of the folder is in conflict.
func (s *Service) Commit(host shell.Host, dir string, req CommitRequest) (Committed, error) {
	message := strings.TrimRight(req.Message, " \t\r\n")
	switch {
	case strings.TrimSpace(message) == "":
		return Committed{}, &Refusal{Why: "write a message first"}
	case len(message) > MaxMessage:
		return Committed{}, &Refusal{Why: "the message is longer than 16 KB"}
	case req.Amend && !headRe.MatchString(req.Head):
		return Committed{}, &Refusal{Why: "choose Amend again: the commit it rewrites is not known"}
	}
	if len(req.Stage) > 0 {
		if err := checkPaths(req.Stage); err != nil {
			return Committed{}, err
		}
	}
	found, release, err := s.prepare(host, dir)
	if err != nil {
		return Committed{}, err
	}
	defer release()
	if len(found.conflicts) > 0 {
		return Committed{}, &Refusal{Why: "resolve the conflicts and stage the files before committing", Paths: keys(found.conflicts)}
	}
	if req.Amend && !found.hasHead {
		return Committed{}, &Refusal{Why: "there is no commit yet to amend"}
	}
	take, skipped := split(req.Stage, found.conflicts)
	var body strings.Builder
	if req.Amend {
		body.WriteString(`[ "$(git rev-parse -q --verify HEAD)" = ` + script.Quote(req.Head) + ` ] || { printf '` + `\036moved` + `'; exit 3; }; `)
	}
	if len(take) > 0 {
		body.WriteString(inChunks(writeGit+" add", take))
	}
	amend := ""
	if req.Amend {
		amend = " --amend"
	} else {
		body.WriteString(`if git diff --cached --quiet 2>/dev/null; then printf '\036nothing'; exit 4; fi; `)
	}
	body.WriteString(`t=$(mktemp "${TMPDIR:-/tmp}/remuda-commit.XXXXXX") || exit 1; printf '%s\n' ` + script.Quote(message) + ` > "$t"; ` +
		writeGit + ` commit -q` + amend + ` -F "$t"; c=$?; rm -f "$t"; [ "$c" = 0 ] || exit "$c"; ` +
		`printf '\035'; git log -1 --format='%H%x1f%h%x1f%s'`)
	output, err := run(host, dir, body.String(), commitFor)
	switch {
	case strings.Contains(output, movedMark):
		return Committed{}, &Refusal{Why: "the latest commit changed since you chose Amend: choose Amend again before retrying"}
	case strings.Contains(output, nothingMark):
		return Committed{}, &Refusal{Why: "nothing is staged to commit"}
	case err != nil:
		return Committed{}, err
	}
	_, made, _ := strings.Cut(output, script.GS)
	fields := script.Fields(strings.TrimSpace(made))
	if len(fields) < 3 {
		return Committed{}, &GitError{Output: strings.TrimSpace(output)}
	}
	return Committed{SHA: fields[0], Short: fields[1], Subject: fields[2], Skipped: skipped}, nil
}

// LastCommit is HEAD and its whole message, which Amend loads into an empty box.
func LastCommit(host shell.Host, dir string) (head, message string, err error) {
	output, err := shell.Text(host, script.InDir(dir)+`git log -1 --format='%H%x00%B' 2>/dev/null`, writeFor)
	if err != nil {
		return "", "", err
	}
	head, message, ok := strings.Cut(output, "\x00")
	if !ok || !headRe.MatchString(head) {
		return "", "", &Refusal{Why: "there is no commit yet to amend"}
	}
	return head, strings.TrimRight(message, "\n"), nil
}

func keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for path := range set {
		out = append(out, path)
	}
	return out
}
