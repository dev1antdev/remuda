package repos

// The open issues and pull requests a new worktree may start from, read with gh on the repository's machine as the
// person, and one number looked up when it is in neither list. Read only: nothing is written to GitHub here.

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// GitHubTimeout is how long gh may take; ListLimit how many of each list are read.
var (
	GitHubTimeout = 45 * time.Second
	ListLimit     = 50
)

// ErrNoGH says the machine has no gh to read GitHub with.
var ErrNoGH = errors.New("no gh")

// Issue is an open issue; PR an open pull request with its head branch and the issues it closes.
type Issue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
}

type PR struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Head      string `json:"head"`
	CrossRepo bool   `json:"cross_repo,omitempty"` // its head is on a fork, fetched through pull/N/head
	Closes    []int  `json:"closes,omitempty"`
}

// Lists are a repository's open issues and pull requests.
type Lists struct {
	Issues []Issue `json:"issues"`
	PRs    []PR    `json:"prs"`
}

// Found is one number looked up: an issue or a pull request, open or not.
type Found struct {
	Kind      string `json:"kind"` // issue | pr
	Number    int    `json:"number"`
	Title     string `json:"title"`
	State     string `json:"state"`
	Head      string `json:"head,omitempty"`
	CrossRepo bool   `json:"cross_repo,omitempty"`
}

var ownerRepo = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// ghPrelude finds gh, quiets its update notice and prompts, and keeps its complaints aside for a failure to say.
const ghPrelude = "command -v gh >/dev/null 2>&1 || { echo 'no gh'; exit 4; }\n" +
	"GH_NO_UPDATE_NOTIFIER=1 GH_PROMPT_DISABLED=1; export GH_NO_UPDATE_NOTIFIER GH_PROMPT_DISABLED\n" +
	"e=$(mktemp 2>/dev/null || echo \"${TMPDIR:-/tmp}/remuda-gh.$$\"); trap 'rm -f \"$e\"' EXIT\n"

// ListScript reads the open issues, then the open pull requests.
func ListScript(repo string) string {
	r := script.Quote(repo)
	n := strconv.Itoa(ListLimit)
	return script.PATH + "\n" + ghPrelude +
		"gh issue list -R " + r + " --state open --limit " + n + " --json number,title 2>\"$e\" || { cat \"$e\"; exit 5; }\nprintf '\\036\\n'\n" +
		"gh pr list -R " + r + " --state open --limit " + n + " --json number,title,headRefName,body,closingIssuesReferences,isCrossRepository 2>\"$e\" || { cat \"$e\"; exit 5; }\n"
}

var closesRef = regexp.MustCompile(`(?i)\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?):?\s+#(\d+)\b`)

// ParseLists reads the script's two JSON lists.
func ParseLists(out string) (Lists, error) {
	parts := script.Sections(out)
	if len(parts) < 2 {
		return Lists{}, errors.New(lastLine(out, "gh answered nothing"))
	}
	lists := Lists{Issues: []Issue{}, PRs: []PR{}}
	if err := json.Unmarshal([]byte(strings.TrimSpace(parts[0])), &lists.Issues); err != nil {
		return Lists{}, fmt.Errorf("gh's issue list: %w", err)
	}
	var prs []struct {
		Number    int    `json:"number"`
		Title     string `json:"title"`
		Head      string `json:"headRefName"`
		Body      string `json:"body"`
		CrossRepo bool   `json:"isCrossRepository"`
		Closing   []struct {
			Number int `json:"number"`
		} `json:"closingIssuesReferences"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(parts[1])), &prs); err != nil {
		return Lists{}, fmt.Errorf("gh's pull request list: %w", err)
	}
	for _, p := range prs {
		pr := PR{Number: p.Number, Title: p.Title, Head: p.Head, CrossRepo: p.CrossRepo}
		seen := map[int]bool{}
		for _, c := range p.Closing {
			if !seen[c.Number] {
				seen[c.Number] = true
				pr.Closes = append(pr.Closes, c.Number)
			}
		}
		for _, m := range closesRef.FindAllStringSubmatch(p.Body, -1) {
			if n, _ := strconv.Atoi(m[1]); n > 0 && !seen[n] {
				seen[n] = true
				pr.Closes = append(pr.Closes, n)
			}
		}
		lists.PRs = append(lists.PRs, pr)
	}
	return lists, nil
}

// ListOpen reads the repository's open issues and pull requests on the machine.
func ListOpen(host shell.Shell, repo string) (Lists, error) {
	if !ownerRepo.MatchString(repo) {
		return Lists{}, fmt.Errorf("%q is no GitHub repository", repo)
	}
	out, err := shell.Text(host, ListScript(repo), GitHubTimeout)
	if err != nil {
		return Lists{}, ghError(out, err)
	}
	return ParseLists(out)
}

// LookupScript asks GitHub what a number is, then a pull request's head.
func LookupScript(repo string, number int) string {
	r, n := script.Quote(repo), strconv.Itoa(number)
	return script.PATH + "\n" + ghPrelude +
		"info=$(gh api " + script.Quote("repos/"+repo+"/issues/"+n) + " --jq '[(.pull_request != null), .state, .title] | @tsv' 2>\"$e\") || { cat \"$e\"; exit 5; }\n" +
		"printf '%s\\n' \"$info\"\nprintf '\\036\\n'\n" +
		"case \"$info\" in true*) gh pr view " + n + " -R " + r + " --json headRefName,isCrossRepository 2>\"$e\" || { cat \"$e\"; exit 5; } ;; esac\n"
}

// ParseLookup reads the script's answer.
func ParseLookup(number int, out string) (Found, error) {
	parts := script.Sections(out)
	fields := strings.SplitN(strings.TrimSpace(parts[0]), "\t", 3)
	if len(fields) < 3 {
		return Found{}, errors.New(lastLine(out, "gh answered nothing"))
	}
	found := Found{Kind: "issue", Number: number, State: strings.ToLower(fields[1]), Title: fields[2]}
	if fields[0] != "true" {
		return found, nil
	}
	found.Kind = "pr"
	if len(parts) > 1 {
		var pr struct {
			Head      string `json:"headRefName"`
			CrossRepo bool   `json:"isCrossRepository"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(parts[1])), &pr); err != nil {
			return Found{}, fmt.Errorf("gh's pull request: %w", err)
		}
		found.Head, found.CrossRepo = pr.Head, pr.CrossRepo
	}
	return found, nil
}

// Lookup is one issue's or pull request's number asked of GitHub on the machine.
func Lookup(host shell.Shell, repo string, number int) (Found, error) {
	if !ownerRepo.MatchString(repo) || number <= 0 {
		return Found{}, fmt.Errorf("nothing to look up as %s#%d", repo, number)
	}
	out, err := shell.Text(host, LookupScript(repo, number), GitHubTimeout)
	if err != nil {
		return Found{}, ghError(out, err)
	}
	return ParseLookup(number, out)
}

// ghError is ErrNoGH for a machine without gh, else gh's own last line.
func ghError(out string, err error) error {
	var exit *shell.ExitError
	if errors.As(err, &exit) && exit.Code == 4 {
		return ErrNoGH
	}
	return errors.New(lastLine(out, err.Error()))
}
