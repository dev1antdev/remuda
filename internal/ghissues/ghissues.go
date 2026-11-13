// Package ghissues is a GitHub repository's issues inside remuda: the list a search finds, one issue with its
// timeline, and the writes the person presses — a comment, a close, a reopen, assigning themselves — all through gh on
// the machine of one of the repository's checkouts, as the person. An answer is kept a while so the panel spends little
// of GitHub's rate limit: under Fresh it is served as it is, to ProbeFor a conditional request decides (a 304 costs
// nothing), past that it is read again whole.
package ghissues

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

// Person is a GitHub account as an issue names it.
type Person struct {
	Login string `json:"login"`
	Bot   bool   `json:"bot,omitempty"`
}

// Label is a label with GitHub's colour for it, six hex digits.
type Label struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description,omitempty"`
}

// Item is one issue as the list shows it.
type Item struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Author    Person    `json:"author"`
	State     string    `json:"state"`            // open, closed
	Reason    string    `json:"reason,omitempty"` // completed, not_planned, for a closed one
	Labels    []Label   `json:"labels"`
	Assignees []Person  `json:"assignees"`
	Updated   time.Time `json:"updated"`
	URL       string    `json:"url"`
}

// List is what a search of the repository's issues found, at most Limit of them.
type List struct {
	Items  []Item    `json:"items"`
	More   bool      `json:"more"` // the search found Limit or more: the newest Limit are listed
	Me     string    `json:"me"`   // the login gh is signed in with on the machine
	At     time.Time `json:"at"`   // when GitHub last confirmed it
	Source string    `json:"source"`
}

// How an answer came: kept and served as it was, kept and confirmed unchanged by a 304, or read afresh.
const (
	FromCache   = "cache"
	FromProbe   = "unchanged"
	FromGitHub  = "read"
	Limit       = 200
	MaxQuery    = 256
	MaxBody     = 65536
	DefaultList = "is:issue is:open"
)

var (
	// Fresh is how long an answer is served with no request at all.
	Fresh = 60 * time.Second
	// ProbeFor is how long an answer is confirmed by a conditional request rather than read again whole.
	ProbeFor = 15 * time.Minute
	// Timeout is how long one script of gh calls may take.
	Timeout = 45 * time.Second
	// LoginFor is how long the signed-in login of a machine's gh is kept.
	LoginFor = time.Hour
	// OriginsFor is how long the origins of a machine's checkouts are kept.
	OriginsFor = time.Minute
)

// ErrInvalid is a request that names no repository, issue or text gh could take.
var ErrInvalid = errors.New("invalid request")

// Trouble is why gh cannot answer, said for the person: what is wrong and what to run, on which machine.
type Trouble struct {
	Kind string `json:"kind"` // no-gh, signed-out, not-found, failed
	Text string `json:"text"`
	Run  string `json:"run,omitempty"`
}

func (t *Trouble) Error() string { return t.Text }

var repoRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*/[A-Za-z0-9_.][A-Za-z0-9_.-]*$`)

// RepoOK says whether a name reads as owner/repository, the only shape the routes pass to gh.
func RepoOK(repo string) bool {
	return len(repo) <= 200 && repoRe.MatchString(repo) && !strings.Contains(repo, "..")
}

// QueryOK says whether a search can be handed to gh: some text on one line, at most MaxQuery long.
func QueryOK(query string) bool {
	if strings.TrimSpace(query) == "" || len(query) > MaxQuery {
		return false
	}
	for _, r := range query {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// Reason is a close's reason as gh takes it: "completed" or "not planned"; false for anything else.
func Reason(word string) (string, bool) {
	switch strings.ReplaceAll(strings.ToLower(strings.TrimSpace(word)), "_", " ") {
	case "", "completed":
		return "completed", true
	case "not planned":
		return "not planned", true
	}
	return "", false
}

// troubleOf reads gh's complaint on a machine: gh missing, not signed in, a repository it cannot see, else its last line.
func troubleOf(machine, repo, out string) *Trouble {
	text := strings.TrimSpace(out)
	low := strings.ToLower(text)
	switch {
	case strings.Contains(low, "gh: command not found"):
		return &Trouble{Kind: "no-gh", Text: "gh, GitHub's command line, is not installed on " + machine + ". Install it there (cli.github.com), then sign in.", Run: "gh auth login"}
	case strings.Contains(low, "gh auth login"), strings.Contains(low, "not logged in"), strings.Contains(low, "http 401"), strings.Contains(low, "bad credentials"), strings.Contains(low, "authentication required"):
		return &Trouble{Kind: "signed-out", Text: "gh on " + machine + " is not signed in to GitHub.", Run: "gh auth login"}
	case strings.Contains(low, "could not resolve to a repository"), strings.Contains(low, "http 404"), strings.Contains(low, "not found"):
		return &Trouble{Kind: "not-found", Text: "gh on " + machine + " cannot see " + repo + ": it is gone, or private to another account.", Run: "gh auth status"}
	case text == "":
		return &Trouble{Kind: "failed", Text: "gh on " + machine + " answered nothing."}
	}
	lines := strings.Split(text, "\n")
	return &Trouble{Kind: "failed", Text: "gh on " + machine + ": " + strings.TrimSpace(lines[len(lines)-1])}
}
