// Package gitgraph reads a checkout's history for the Git lens: the commits of its branches, tags and remotes newest
// first in date order, the labels each one carries, where HEAD stands, the uncommitted files, a merge or rebase stopped
// halfway, a fingerprint of the refs that moves when any of them moves, and one commit's files and one file's patch.
// Every read is one script over the machine's shell that writes nothing: no index lock, no fsmonitor, no refresh.
package gitgraph

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

const (
	Page       = 300  // the commits a page of the history holds
	MaxLimit   = 9000 // the most a reread of the pages shown asks for at once
	MaxChanges = 200  // the tracked uncommitted files named, for a hard reset's confirm
	readFor    = 60 * time.Second
)

// Ref is one label on a commit: a local branch (head when it is checked out), a remote branch, a tag, or HEAD itself
// when it is detached.
type Ref struct {
	Kind string `json:"kind"` // branch | remote | tag | head
	Name string `json:"name"`
	Head bool   `json:"head,omitempty"`
}

// Commit is one row of the history, as git log prints it.
type Commit struct {
	Hash    string   `json:"hash"`
	Parents []string `json:"parents"`
	Author  string   `json:"author"`
	Email   string   `json:"email"`
	Time    int64    `json:"time"`
	Refs    []Ref    `json:"refs,omitempty"`
	Subject string   `json:"subject"`
}

// Change is one tracked file the working tree or the index has changed, in git status's two letters.
type Change struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}

// State is where the checkout stands now: cheap enough to read every few seconds, its fingerprint moving whenever HEAD,
// a branch, a tag, a remote branch or a stopped operation moves, so the history is read again only then.
type State struct {
	Repo        bool     `json:"repo"` // false: the folder is not inside a git repository (or is gone)
	Root        string   `json:"root,omitempty"`
	Head        string   `json:"head,omitempty"`   // "" before the first commit
	Branch      string   `json:"branch,omitempty"` // "" while HEAD is detached
	Operation   string   `json:"operation,omitempty"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	Dirty       int      `json:"dirty"`             // the files git status lists, untracked ones too
	Conflicts   int      `json:"conflicts"`         // the files still unmerged
	Changes     []Change `json:"changes,omitempty"` // the tracked ones, at most MaxChanges
}

// Log is one page of the history with the state it was read at.
type Log struct {
	State
	Commits  []Commit `json:"commits"`
	More     bool     `json:"more"` // a commit past the page exists
	Skip     int      `json:"skip"`
	Branches []string `json:"branches"` // every local branch, for a remote branch's checkout: is there a local one already
	Remotes  []string `json:"remotes"`
	Error    string   `json:"error,omitempty"` // git's own words when the history could not be read
}

// Query is what a page of the history covers: every branch or HEAD's alone, the remote branches too, from where.
type Query struct {
	All     bool
	Remotes bool
	Skip    int
	Limit   int
}

// ErrInput is a request this package refuses before any script runs.
var ErrInput = errors.New("a commit is named by its hash, a file by its path inside the folder")

var hashRe = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

// HashOK says the text names a commit by its hash, the only way a commit reaches a script here.
func HashOK(hash string) bool { return hashRe.MatchString(hash) }

// PathOK says the path is a file's inside the folder: relative, no climbing out, never read as an option.
func PathOK(path string) bool {
	return path != "" && !rpath.Absolute(path) && !strings.HasPrefix(path, "-") && !strings.Contains(path, "..") && !strings.ContainsAny(path, "\x00\n")
}

// ReadState reads where the checkout stands.
func ReadState(host shell.Host, dir string) (State, error) {
	out, err := shell.Text(host, stateScript(dir)+`printf '\036end\n'`, readFor)
	if err != nil {
		return State{}, err
	}
	return parseState(sectionsOf(out)), nil
}

// ReadLog reads one page of the history and the state it stands at.
func ReadLog(host shell.Host, dir string, q Query) (Log, error) {
	if q.Limit <= 0 || q.Limit > MaxLimit {
		q.Limit = Page
	}
	if q.Skip < 0 {
		q.Skip = 0
	}
	out, err := shell.Text(host, logScript(dir, q), readFor)
	if err != nil {
		return Log{}, err
	}
	parts := sectionsOf(out)
	log := Log{State: parseState(parts), Commits: []Commit{}, Branches: []string{}, Remotes: []string{}, Skip: q.Skip}
	if !log.Repo {
		return log, nil
	}
	if end, ok := parts.get("logend"); ok {
		if code := end.args; code != "0" {
			log.Error = LastLines(end.body, 6)
			if log.Error == "" {
				log.Error = "git log ended with " + code
			}
		}
	}
	if body, ok := parts.get("log"); ok {
		commits, err := parseLog(body.body)
		if err != nil && log.Error == "" {
			log.Error = err.Error()
		}
		if len(commits) > q.Limit {
			commits, log.More = commits[:q.Limit], true
		}
		if !q.Remotes {
			for i := range commits {
				commits[i].Refs = withoutRemotes(commits[i].Refs)
			}
		}
		log.Commits = commits
	}
	if body, ok := parts.get("branches"); ok {
		for _, line := range script.Lines(body.body) {
			if name, ok := strings.CutPrefix(line, "refs/heads/"); ok {
				log.Branches = append(log.Branches, name)
			}
		}
	}
	if body, ok := parts.get("remotes"); ok {
		log.Remotes = append(log.Remotes, script.Lines(body.body)...)
	}
	return log, nil
}

// parseState reads the header the state script prints and its status section.
func parseState(parts sections) State {
	header := strings.Split(strings.TrimRight(parts.head, "\n"), "\n")
	if len(header) < 6 || header[0] != "repo" {
		return State{}
	}
	st := State{Repo: true, Root: header[1], Head: header[2], Branch: header[3], Operation: header[4], Fingerprint: header[5]}
	if body, ok := parts.get("status"); ok {
		for _, line := range script.Lines(body.body) {
			if len(line) < 4 {
				continue
			}
			st.Changes = append(st.Changes, Change{Status: line[:2], Path: strings.ReplaceAll(line[3:], "\x01", "\n")})
		}
	}
	if count, ok := parts.get("count"); ok {
		fields := strings.Fields(count.args)
		if len(fields) == 2 {
			st.Dirty, _ = strconv.Atoi(fields[0])
			st.Conflicts, _ = strconv.Atoi(fields[1])
		}
	}
	return st
}

// parseLog reads git log -z with seven NUL-ended fields a commit.
func parseLog(text string) ([]Commit, error) {
	text = strings.TrimPrefix(text, "\n")
	commits := []Commit{}
	if text == "" {
		return commits, nil
	}
	fields := strings.Split(text, "\x00")
	if last := fields[len(fields)-1]; strings.TrimSpace(last) == "" {
		fields = fields[:len(fields)-1]
	}
	if len(fields)%7 != 0 {
		return commits, errors.New("the history came back cut short")
	}
	for i := 0; i < len(fields); i += 7 {
		hash := strings.TrimLeft(fields[i], "\n")
		when, _ := strconv.ParseInt(fields[i+4], 10, 64)
		commit := Commit{Hash: hash, Parents: strings.Fields(fields[i+1]), Author: fields[i+2], Email: fields[i+3], Time: when, Refs: parseRefs(fields[i+5]), Subject: fields[i+6]}
		if !HashOK(commit.Hash) {
			return commits, errors.New("the history came back unreadable")
		}
		commits = append(commits, commit)
	}
	return commits, nil
}

// parseRefs reads git's %D with full ref names: "HEAD -> refs/heads/main, tag: refs/tags/v1, refs/remotes/origin/main".
func parseRefs(text string) []Ref {
	var refs []Ref
	if text == "" {
		return refs
	}
	for _, name := range strings.Split(text, ", ") {
		head := false
		if rest, ok := strings.CutPrefix(name, "HEAD -> "); ok {
			name, head = rest, true
		}
		switch {
		case name == "HEAD":
			refs = append(refs, Ref{Kind: "head", Name: "HEAD"})
		case strings.HasPrefix(name, "refs/heads/"):
			refs = append(refs, Ref{Kind: "branch", Name: strings.TrimPrefix(name, "refs/heads/"), Head: head})
		case strings.HasPrefix(name, "tag: refs/tags/"):
			refs = append(refs, Ref{Kind: "tag", Name: strings.TrimPrefix(name, "tag: refs/tags/")})
		case strings.HasPrefix(name, "refs/remotes/") && !strings.HasSuffix(name, "/HEAD"):
			refs = append(refs, Ref{Kind: "remote", Name: strings.TrimPrefix(name, "refs/remotes/")})
		}
	}
	return refs
}

// withoutRemotes drops the remote branches' labels: git decorates every ref it knows, walked or not.
func withoutRemotes(refs []Ref) []Ref {
	var kept []Ref
	for _, ref := range refs {
		if ref.Kind != "remote" {
			kept = append(kept, ref)
		}
	}
	return kept
}

// LastLines is git's own words for a failure: the last n lines of its output without the hints.
func LastLines(text string, n int) string {
	var kept []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, " \r\t")
		if line == "" || strings.HasPrefix(line, "hint:") {
			continue
		}
		kept = append(kept, line)
	}
	if len(kept) > n {
		kept = kept[len(kept)-n:]
	}
	return strings.Join(kept, "\n")
}
