package gitsync

import (
	"strconv"
	"strings"
	"time"
)

// Repo is one checkout as its row shows it.
type Repo struct {
	Path      string    `json:"path"`
	Name      string    `json:"name"`
	Parent    string    `json:"parent"`              // the folder it sits in, the home folder as ~
	Workspace bool      `json:"workspace,omitempty"` // a workspace of the machine stands on it, else the scan found it
	Branch    string    `json:"branch,omitempty"`    // "" for a detached HEAD
	Detached  bool      `json:"detached,omitempty"`
	Upstream  string    `json:"upstream,omitempty"` // the upstream as git names it, origin/main
	Gone      bool      `json:"gone,omitempty"`     // an upstream is set but no longer there
	Ahead     int       `json:"ahead"`
	Behind    int       `json:"behind"`
	Dirty     int       `json:"dirty"` // uncommitted entries: changed, staged, new, in conflict
	Staged    int       `json:"staged,omitempty"`
	Conflicts int       `json:"conflicts,omitempty"`
	Remote    bool      `json:"remote"`            // the repository has a remote
	Fetched   time.Time `json:"fetched,omitzero"`  // FETCH_HEAD's time: the last fetch, pull or pull's fetch
	Error     string    `json:"error,omitempty"`   // git status's own last line when it failed
	Initial   bool      `json:"initial,omitempty"` // no commit yet
}

// Status reads `git status --porcelain=v2 --branch -z` into the row's counts.
func (repo *Repo) Status(output string) {
	counted := false
	records := strings.Split(output, "\x00")
	for i := 0; i < len(records); i++ {
		record := records[i]
		if record == "" {
			continue
		}
		if !strings.HasPrefix(record, "# ") {
			switch record[0] {
			case '1':
				repo.Dirty++
				if staged(record) {
					repo.Staged++
				}
			case '2':
				repo.Dirty++
				if staged(record) {
					repo.Staged++
				}
				i++ // a rename's original path follows it as a record of its own
			case 'u':
				repo.Dirty++
				repo.Conflicts++
			case '?':
				repo.Dirty++
			}
			continue
		}
		key, value, _ := strings.Cut(record[2:], " ")
		switch key {
		case "branch.oid":
			repo.Initial = value == "(initial)"
		case "branch.head":
			repo.Detached = value == "(detached)"
			if !repo.Detached {
				repo.Branch = value
			}
		case "branch.upstream":
			repo.Upstream = value
		case "branch.ab":
			ahead, behind, ok := aheadBehind(value)
			if ok {
				repo.Ahead, repo.Behind, counted = ahead, behind, true
			}
		}
	}
	repo.Gone = repo.Upstream != "" && !counted
}

// staged says whether a changed entry's index side moved: its XY's X is not ".".
func staged(record string) bool {
	return len(record) > 2 && record[2] != '.'
}

// aheadBehind reads git's "+2 -3".
func aheadBehind(value string) (int, int, bool) {
	plus, minus, ok := strings.Cut(value, " ")
	if !ok || !strings.HasPrefix(plus, "+") || !strings.HasPrefix(minus, "-") {
		return 0, 0, false
	}
	ahead, errAhead := strconv.Atoi(plus[1:])
	behind, errBehind := strconv.Atoi(minus[1:])
	return ahead, behind, errAhead == nil && errBehind == nil
}
